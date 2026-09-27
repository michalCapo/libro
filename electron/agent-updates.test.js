const { test } = require('node:test')
const vm = require('node:vm')
const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const os = require('node:os')
const path = require('node:path')
const { checkUpdates, updateAgents, updateCommand, startAgentUpdates, version, newer } = require('./agent-updates')

test('compares numeric releases without downgrading or replacing prereleases', () => {
  assert.equal(version('codex-cli 0.100.0'), '0.100.0')
  assert.equal(version('2.1.4 (Claude Code)'), '2.1.4')
  assert.equal(version('v1.2.3-beta.1'), '1.2.3-beta.1')
  assert.equal(version('unexpected output'), null)
  assert.equal(newer('0.100.0', '0.99.0'), true)
  for (const current of ['0.100.0', '0.101.0', '0.100.0-beta.1']) assert.equal(newer('0.100.0', current), false)
})

test('missing, current and failed agents do not prevent other update checks', async () => {
  const results = await checkUpdates({
    find: async name => ['codex', 'claude', 'pi'].includes(name) ? name : null,
    runCommand: async file => { if (file === 'claude') throw Error('timeout'); return '1.0.0' },
    latest: async agent => agent.command === 'pi' ? '1.0.0' : '1.1.0',
    plan: async () => ({ file: 'npm', args: ['install'] }),
  })
  assert.deepEqual(results.map(result => result.name), ['Codex'])
})

test('npm updates target only the package that owns the executable', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'libro-updates-test-'))
  t.after(() => fs.rm(root, { recursive: true, force: true }))
  const packageRoot = path.join(root, '@earendil-works/pi-coding-agent')
  await fs.mkdir(packageRoot, { recursive: true })
  const executable = path.join(packageRoot, 'cli.js')
  await fs.writeFile(executable, '')
  const agent = { command: 'pi', package: '@earendil-works/pi-coding-agent', latest: '1.2.3' }
  assert.deepEqual(await updateCommand(agent, executable, root, '/bin/npm'), {
    file: '/bin/npm', args: ['install', '--global', '@earendil-works/pi-coding-agent@1.2.3'],
  })
  assert.equal(await updateCommand(agent, executable, path.join(root, 'other'), '/bin/npm'), null)
})

function fixture(updates, options = {}) {
  const notices = []
  const calls = []
  return {
    notices, calls,
    run: () => updateAgents(async (...args) => notices.push(args), {
      check: async () => updates, find: async () => null,
      runCommand: async (file, args) => { calls.push([file, args]); return '1.1.0' }, ...options,
    }),
  }
}
const codex = { name: 'Codex', executable: '/bin/codex', current: '1.0.0', latest: '1.1.0', install: { file: 'npm', args: ['install'] } }

test('automatically updates and verifies before showing success', async () => {
  const f = fixture([codex])
  await f.run()
  assert.deepEqual(f.calls, [['npm', ['install']], ['/bin/codex', ['--version']]])
  assert.equal(f.notices[0][0], 'Updating Codex…')
  assert.equal(f.notices[1][2], 'success')
})

test('reports failure and continues with other agents', async () => {
  const f = fixture([codex, { ...codex, name: 'Pi', install: null }], { runCommand: async () => { throw Error('permission denied') } })
  await f.run()
  assert.ok(f.notices.some(([title, , variant]) => title.includes('Codex') && variant === 'error'))
  assert.ok(f.notices.some(([title]) => /Pi.*available/.test(title)))
})

test('does not report success if updater leaves the old version installed', async () => {
  const f = fixture([codex], { runCommand: async () => '1.0.0' })
  await f.run()
  assert.equal(f.notices.at(-1)[2], 'error')
})

test('updates Pi extensions silently when Pi itself is current', async () => {
  const calls = []
  const f = fixture([], {
    find: async name => name === 'pi' ? '/bin/pi' : null,
    runCommand: async (_file, args) => { calls.push(args); return '--extensions --no-approve' },
  })
  await f.run()
  assert.deepEqual(calls, [['update', '--help'], ['update', '--extensions', '--no-approve']])
  assert.deepEqual(f.notices, [])
})

test('supports legacy Pi package updates and reports errors', async () => {
  const calls = []
  const f = fixture([], {
    find: async () => '/bin/pi',
    runCommand: async (_file, args) => {
      calls.push(args)
      if (!args.includes('--help')) throw Error('offline')
      return 'Update installed packages'
    },
  })
  await f.run()
  assert.deepEqual(calls.at(-1), ['update'])
  assert.equal(f.notices.at(-1)[2], 'error')
})

test('does not begin updates after the window closes', async () => {
  const f = fixture([codex], { active: () => false })
  await f.run()
  assert.equal(f.calls.length, 0)
  assert.equal(f.notices.length, 0)
})


test('agents update concurrently; Pi extensions wait only for Pi', async () => {
  const codexGate = Promise.withResolvers()
  const piGate = Promise.withResolvers()
  const started = []
  const f = fixture([
    { ...codex, command: 'codex', install: { file: 'codex-update', args: [] } },
    { ...codex, command: 'pi', name: 'Pi', executable: '/bin/pi', install: { file: 'pi-update', args: [] } },
  ], {
    find: async () => '/bin/pi',
    runCommand: async (file, args) => {
      if (file.endsWith('-update')) {
        started.push(file)
        await (file === 'codex-update' ? codexGate.promise : piGate.promise)
      }
      if (args.includes('--help')) return '--extensions'
      if (args.includes('--extensions')) started.push('extensions')
      return '1.1.0'
    },
  })
  const running = f.run()
  try {
    await new Promise(resolve => setImmediate(resolve))
    assert.deepEqual(started, ['codex-update', 'pi-update'])
    piGate.resolve()
    await new Promise(resolve => setImmediate(resolve))
    assert.deepEqual(started, ['codex-update', 'pi-update', 'extensions'])
    assert.ok(!f.notices.some(([title]) => title.includes('extensions')))
    assert.ok(!f.notices.some(([title]) => title.startsWith('Codex updated')))
  } finally {
    piGate.resolve()
    codexGate.resolve()
    await running
  }
})


test('startup skips all CLI and extension work when automatic updates are off', async () => {
  for (const [stored, expected] of [[null, 1], ['{"agentAutoUpdate":false}', 0], ['{"agentAutoUpdate":true}', 1], ['invalid', 0]]) {
    let checks = 0
    let finds = 0
    const window = {
      isDestroyed: () => false,
      webContents: { executeJavaScript: async source => vm.runInNewContext(source, { localStorage: { getItem: () => stored } }) },
    }
    await startAgentUpdates(window, {
      check: async () => { checks++; return [] },
      find: async () => { finds++; return null },
    })
    assert.equal(checks, expected)
    assert.equal(finds, expected)
  }
})
