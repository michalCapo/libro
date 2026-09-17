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
    goProcess: child, goProcessForceKillTimer: null, isQuitting: false, quitReady: false,
    setTimeout, clearTimeout, console,
    flushLibroSessionData: async () => events.push('flush'),
    mainWindow: { isDestroyed: () => false, destroy: () => events.push('destroy') },
  })
  vm.runInContext(stop + quit, context)
  const pending = context.quitApp()
  assert.deepEqual(events, ['SIGTERM'])
  assert.equal(context.quitReady, false)
  await context.quitApp()
  assert.deepEqual(events, ['SIGTERM'])
  clearTimeout(context.goProcessForceKillTimer)
  child.emit('exit', 0)
  await pending
  assert.deepEqual(events, ['SIGTERM', 'flush', 'destroy'])
  assert.equal(context.quitReady, true)
})

const request = source.slice(source.indexOf('function requestQuit('), source.indexOf('\nfunction moveSelectedApp'))

test('native close requests show confirmation and can be retried after cancel', () => {
  const scripts = []
  let prevented = 0
  const context = vm.createContext({
    isQuitting: false, quitReady: false,
    mainWindow: { isDestroyed: () => false },
    dispatchToMainWindow: (script) => scripts.push(script),
  })
  vm.runInContext(request, context)
  const event = { preventDefault: () => prevented++ }
  context.requestQuit(event)
  context.requestQuit(event)
  assert.equal(prevented, 2)
  assert.equal(scripts.length, 2)
  assert.match(scripts[0], /window\.__libroShowCloseDialog\(\)/)

  context.isQuitting = true
  context.requestQuit(event)
  assert.equal(prevented, 3)
  assert.equal(scripts.length, 2)

  context.quitReady = true
  context.requestQuit(event)
  assert.equal(prevented, 3)
})

test('quit without a window cleans up instead of waiting for a renderer', async () => {
  let stopped = false
  const context = vm.createContext({
    isQuitting: false, quitReady: false, mainWindow: null, console,
    quitApp: async () => { stopped = true },
  })
  vm.runInContext(request, context)
  let prevented = false
  context.requestQuit({ preventDefault: () => { prevented = true } })
  assert.equal(prevented, true)
  assert.equal(stopped, true)
})

test('window close and application quit use the same confirmation hook', () => {
  assert.match(source, /mainWindow\.on\('close', requestQuit\)/)
  assert.match(source, /app\.on\('before-quit', requestQuit\)/)
})
