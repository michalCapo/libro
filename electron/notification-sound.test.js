const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
const source = workspace.slice(workspace.indexOf('  let notificationAudio;'), workspace.indexOf('  function renderProjectActivity()'))
function setup(prefs = {}) {
  const elements = { 'notification-sound': {}, 'notification-sound-status': {} }
  const listeners = {}
  const tones = []
  let stored
  const context = vm.createContext({
    prefs, window: { addEventListener(name, handler) { listeners[name] = handler } },
    document: { addEventListener(name, handler) { listeners[name] = handler }, getElementById(id) { return elements[id] } },
    localStorage: { setItem(key, value) { stored = JSON.parse(value) } },
    AudioContext: class {
      state = 'running'; currentTime = 0; destination = {}
      createOscillator() { return { frequency: {}, connect() {}, disconnect() {}, start(at) { tones.push(at) }, stop() {} } }
      createGain() { return { gain: { setValueAtTime() {}, linearRampToValueAtTime() {}, exponentialRampToValueAtTime() {} }, connect() {}, disconnect() {} } }
    },
  })
  vm.runInContext(source, context)
  listeners.pointerdown()
  return { context, tones, elements, stored: () => stored, update(statuses) {
    context.window.__libroAgentStatuses = statuses
    listeners['libro-agent-status']()
  } }
}

test('each agent completion sounds once even while another agent works', () => {
  const app = setup()
  app.update({ a: 'done' }) // Initial snapshot must be silent.
  assert.equal(app.tones.length, 0)
  app.update({ a: 'working', b: 'working' })
  app.update({ a: 'done', b: 'working' })
  assert.equal(app.tones.length, 2)
  app.update({ a: 'done', b: 'working' })
  app.update({ a: 'done', b: 'done' })
  assert.equal(app.tones.length, 4)
  app.update({}) // Disconnect and reconnect must not replay completion.
  app.update({ a: 'done', b: 'done' })
  assert.equal(app.tones.length, 4)
  app.update({ a: 'working' })
  app.update({ a: 'error' })
  app.update({ a: 'done' })
  assert.equal(app.tones.length, 4)
  app.update({ a: 'working' })
  app.update({ a: 'done' })
  assert.equal(app.tones.length, 6)
})

test('sound preference persists and muted completions are not replayed', () => {
  const app = setup({ projects: true })
  vm.runInContext("saveNotificationSound('off')", app.context)
  assert.deepEqual(app.stored(), { projects: true, notificationSound: false })
  app.update({ a: 'working' }); app.update({ a: 'done' })
  assert.equal(app.tones.length, 0)
  vm.runInContext("saveNotificationSound('on')", app.context)
  app.update({ a: 'done' })
  assert.equal(app.tones.length, 0)
  app.update({ a: 'working' }); app.update({ a: 'done' })
  assert.equal(app.tones.length, 2)
  const restarted = setup({ notificationSound: false })
  restarted.update({ a: 'working' }); restarted.update({ a: 'done' })
  assert.equal(restarted.tones.length, 0)
})

test('failed save keeps the current preference and explains the failure', () => {
  const app = setup()
  app.context.localStorage.setItem = () => { throw new Error('storage unavailable') }
  vm.runInContext("saveNotificationSound('off')", app.context)
  assert.equal(app.elements['notification-sound'].value, 'on')
  assert.match(app.elements['notification-sound-status'].textContent, /Could not save/)
  app.update({ a: 'working' }); app.update({ a: 'done' })
  assert.equal(app.tones.length, 2)
})
