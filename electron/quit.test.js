const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const { EventEmitter } = require('node:events')

const source = fs.readFileSync(require.resolve('./main.js'), 'utf8')
const stop = source.slice(source.indexOf('async function stopGoServer()'), source.indexOf('\nfunction refreshTerminalFramesAfterResume'))
const quit = source.slice(source.indexOf('async function quitApp()'), source.indexOf('\nfunction createWindow()'))

test('quit waits for backend exit before destroying the window', async () => {
  const events = []
  const child = new EventEmitter()
  child.kill = (signal) => events.push(signal)
  const context = vm.createContext({
    goProcess: child, goProcessForceKillTimer: null, isQuitting: false,
    setTimeout, clearTimeout, console,
    flushLibroSessionData: async () => events.push('flush'),
    mainWindow: { isDestroyed: () => false, destroy: () => events.push('destroy') },
  })
  vm.runInContext(stop + quit, context)
  const pending = context.quitApp()
  assert.deepEqual(events, ['SIGTERM'])
  clearTimeout(context.goProcessForceKillTimer)
  child.emit('exit', 0)
  await pending
  assert.deepEqual(events, ['SIGTERM', 'flush', 'destroy'])
})
