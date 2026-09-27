const { execFile } = require('node:child_process')
const fs = require('node:fs/promises')
const path = require('node:path')
const os = require('node:os')
const { promisify } = require('node:util')

const execute = promisify(execFile)
const agents = [
  { command: 'codex', name: 'Codex', package: '@openai/codex' },
  { command: 'claude', name: 'Claude', package: '@anthropic-ai/claude-code' },
  { command: 'pi', name: 'Pi', package: '@earendil-works/pi-coding-agent' },
  { command: 'opencode', name: 'OpenCode', package: 'opencode-ai' },
]

async function run(file, args, timeout = 15000) {
  const cwd = await fs.mkdtemp(path.join(os.tmpdir(), 'libro-agent-update-'))
  try {
    const result = execute(file, args, { cwd, timeout, maxBuffer: 1024 * 1024, windowsHide: true })
    result.child.stdin.end()
    return (await result).stdout.trim()
  } finally { await fs.rm(cwd, { recursive: true, force: true }) }
}

async function findExecutable(command) {
  for (const directory of (process.env.PATH || '').split(path.delimiter).filter(directory => path.isAbsolute(directory))) {
    for (const suffix of process.platform === 'win32' ? ['.exe', '.cmd', ''] : ['']) {
      const file = path.resolve(directory, command + suffix)
      try {
        await fs.access(file, fs.constants.X_OK)
        if ((await fs.stat(file)).isFile()) return file
      } catch (_) {}
    }
  }
  return null
}

function version(text) {
  return String(text).match(/(?:^|\s)v?(\d+\.\d+\.\d+(?:-[\w.-]+)?)(?:\+[\w.-]+)?(?=\s|$)/)?.[1] || null
}

function newer(latest, current) {
  if (!latest || !current || latest.includes('-') || current.includes('-')) return false
  const left = latest.split('.').map(Number)
  const right = current.split('.').map(Number)
  for (let i = 0; i < 3; i++) {
    if (left[i] !== right[i]) return left[i] > right[i]
  }
  return false
}

async function latestVersion(agent) {
  // Public metadata only; no npm credentials or project configuration are sent.
  const response = await fetch(`https://registry.npmjs.org/${encodeURIComponent(agent.package)}/latest`, {
    signal: AbortSignal.timeout(10000),
  })
  if (!response.ok) throw new Error('Version lookup failed')
  return version((await response.json()).version)
}

async function updateCommand(agent, executable, npmRoot, npm, runCommand = run) {
  const resolved = await fs.realpath(executable)
  if (npmRoot && npm) {
    const packageRoot = path.join(npmRoot, agent.package)
    const relative = path.relative(packageRoot, resolved)
    if (relative && !relative.startsWith('..') && !path.isAbsolute(relative)) {
      return { file: npm, args: ['install', '--global', `${agent.package}@${agent.latest}`] }
    }
  }
  if (agent.command === 'claude' && resolved.startsWith(path.join(os.homedir(), '.local', 'share', 'claude', 'versions') + path.sep)) {
    return { file: executable, args: ['update'] }
  }
  if (agent.command === 'opencode' && resolved === path.join(os.homedir(), '.opencode', 'bin', 'opencode')) {
    return { file: executable, args: ['upgrade', agent.latest] }
  }
  // Only standalone Codex builds with a self-updater can update themselves.
  if (agent.command === 'codex' && !/[/\\](?:Cellar|Caskroom|node_modules)[/\\]/.test(resolved)) {
    const help = await runCommand(executable, ['--help'])
    if (/^\s+update\s/m.test(help)) return { file: executable, args: ['update'] }
  }
  return null
}

async function checkUpdates({ runCommand = run, find = findExecutable, latest = latestVersion, plan = updateCommand } = {}) {
  const npm = await find('npm')
  let npmRoot = null
  if (npm) {
    try { npmRoot = await fs.realpath(await runCommand(npm, ['root', '--global'])) } catch (_) {}
  }
  const results = await Promise.allSettled(agents.map(async definition => {
    const agent = { ...definition }
    const executable = await find(agent.command)
    if (!executable) return null
    if (agent.command === 'pi') {
      const resolved = await fs.realpath(executable).catch(() => '')
      if (resolved.includes('@mariozechner')) agent.package = '@mariozechner/pi-coding-agent'
    }
    const current = version(await runCommand(executable, ['--version']))
    if (!current || current.includes('-')) return null
    const available = await latest(agent)
    if (!newer(available, current)) return null
    const update = { ...agent, executable, current, latest: available }
    update.install = await plan(update, executable, npmRoot, npm, runCommand)
    return update
  }))
  return results.filter(result => result.status === 'fulfilled' && result.value).map(result => result.value)
}

async function updateAgents(notify, { check = checkUpdates, runCommand = run, find = findExecutable, active = () => true } = {}) {
  const updates = await check()
  const tasks = updates.map(async update => {
    if (!active()) return
    if (!update.install) {
      await notify(`${update.name} ${update.latest} is available`, 'Update it with the package manager used to install it.', 'info')
      return
    }
    await notify(`Updating ${update.name}…`, `${update.current} → ${update.latest}`, 'info')
    try {
      await runCommand(update.install.file, update.install.args, 300000)
      const installed = version(await runCommand(update.executable, ['--version']))
      if (!installed || !newer(installed, update.current)) throw new Error('Version did not change')
      await notify(`${update.name} updated to ${installed}`, 'New agent sessions will use this version.', 'success')
    } catch (_) {
      await notify(`${update.name} could not be updated`, 'Try updating it in a terminal with its package manager.', 'error')
    }
  })
  const updateExtensions = async () => {
    await tasks[updates.findIndex(update => update.command === 'pi')]
    const pi = await find('pi')
    if (!pi || !active()) return
    try {
      const help = await runCommand(pi, ['update', '--help'])
      // Older Pi releases used bare `update` for packages, before adding a self-updater.
      const args = help.includes('--extensions') ? ['update', '--extensions', '--no-approve'] : ['update']
      await runCommand(pi, args, 300000)
      // Successful package maintenance is silent: Pi also succeeds when nothing changed.
    } catch (_) {
      await notify('Pi extensions could not be updated', 'Run Pi’s package update command in a terminal to retry.', 'error')
    }
  }
  await Promise.all([...tasks, updateExtensions()])
}

async function startAgentUpdates(window, options = {}) {
  if (window.isDestroyed()) return
  const enabled = await window.webContents.executeJavaScript(`(JSON.parse(localStorage.getItem('libro.workspace') || '{}') || {}).agentAutoUpdate !== false`).catch(() => false)
  if (!enabled) return
  const active = () => !window.isDestroyed()
  const notify = async (title, subtitle, variant) => {
    if (!active()) return
    await window.webContents.executeJavaScript(`window.__libroShowToast?.(${JSON.stringify(title)}, ${JSON.stringify(subtitle)}, ${JSON.stringify(variant)})`).catch(() => {})
  }
  return updateAgents(notify, { ...options, active })
}

module.exports = { checkUpdates, updateAgents, startAgentUpdates, updateCommand, version, newer }
