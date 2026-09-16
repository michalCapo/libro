const { test } = require('node:test')
const assert = require('node:assert/strict')
const { EventEmitter } = require('node:events')
const { PassThrough } = require('node:stream')
const { readFileSync } = require('node:fs')
const vm = require('node:vm')

function fixture() {
  const app = new EventEmitter()
  const nativeTheme = { themeSource: 'system' }
  const monitor = new EventEmitter()
  monitor.stdout = new PassThrough()
  monitor.kill = () => { monitor.killed = true }
  let read
  const module = { exports: {} }
  vm.runInNewContext(readFileSync(require.resolve('./linux-theme'), 'utf8'), {
    module, process,
    require: name => name === 'node:child_process' ? {
      spawn: () => monitor,
      execFile: (_file, _args, _options, callback) => { read = callback },
    } : require(name),
  })
  return { app, nativeTheme, monitor, start: platform => module.exports.followLinuxTheme(app, nativeTheme, platform), read: (...args) => read(...args) }
}

test('reads initial preference, follows complete monitor lines, and cleans up', async () => {
  const f = fixture()
  const ready = f.start('linux')
  f.read(null, "'prefer-dark'\n")
  await ready
  assert.equal(f.nativeTheme.themeSource, 'dark')
  f.monitor.stdout.write("color-scheme: 'prefer-")
  assert.equal(f.nativeTheme.themeSource, 'dark')
  f.monitor.stdout.write("light'\n")
  assert.equal(f.nativeTheme.themeSource, 'light')
  f.monitor.stdout.write("color-scheme: 'default'\n")
  assert.equal(f.nativeTheme.themeSource, 'system')
  f.app.emit('will-quit')
  assert.equal(f.monitor.killed, true)
})

test('a stale initial read cannot override a live change', async () => {
  const f = fixture()
  const ready = f.start('linux')
  f.monitor.stdout.write("color-scheme: 'prefer-dark'\n")
  f.read(null, "'prefer-light'\n")
  await ready
  assert.equal(f.nativeTheme.themeSource, 'dark')
  f.app.emit('will-quit')
})

test('missing GSettings leaves native detection intact', async () => {
  const f = fixture()
  const ready = f.start('linux')
  f.monitor.emit('error', new Error('ENOENT'))
  f.read(new Error('ENOENT'))
  await ready
  assert.equal(f.nativeTheme.themeSource, 'system')
  f.app.emit('will-quit')
})

test('other platforms use native detection', async () => {
  const f = fixture()
  await f.start('darwin')
  assert.equal(f.nativeTheme.themeSource, 'system')
  assert.equal(f.app.listenerCount('will-quit'), 0)
})
