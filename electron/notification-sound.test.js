const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
const source = workspace.slice(workspace.indexOf('  // Each sound gets a fresh context.'), workspace.indexOf('  const acknowledgedAgents ='))
function setup(prefs = {}) {
  const elements = { 'notification-sound': {}, 'notification-sound-status': {} }
  const listeners = {}
  const tones = []
  let closed = 0
  let stored
  const context = vm.createContext({
    prefs, window: { addEventListener(name, handler) { listeners[name] = handler } },
    document: { addEventListener(name, handler) { listeners[name] = handler }, getElementById(id) { return elements[id] } },
    localStorage: { setItem(key, value) { stored = JSON.parse(value) } },
    AudioContext: class {
      state = 'running'; currentTime = 0; destination = {}
      close() { closed++; return Promise.resolve() }
      createOscillator() {
        const tone = { type: 'sine', frequency: {}, connect() {}, disconnect() {}, start(at) { tones.push({ at: Math.round(at * 1000) / 1000, type: tone.type, frequency: tone.frequency.value, tone }) }, stop() {} }
        return tone
      }
      createGain() { return { gain: { setValueAtTime() {}, linearRampToValueAtTime() {}, exponentialRampToValueAtTime() {} }, connect() {}, disconnect() {} } }
    },
  })
  vm.runInContext(source, context) // No interaction: completions must sound right away.
  return { context, tones, elements, closed: () => closed, stored: () => stored, update(statuses) {
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
  assert.equal(app.tones.length, 4)
  assert.deepEqual(app.tones.map(tone => tone.type), ['triangle', 'triangle', 'triangle', 'triangle'])
  assert.deepEqual(app.tones.map(tone => tone.frequency), [392, 493.88, 587.33, 739.99])
  assert.deepEqual(app.tones.map(tone => tone.at), [0, 0.035, 0.07, 0.105])
  assert.deepEqual(app.tones.map(entry => typeof entry.tone.onended), ['undefined', 'undefined', 'undefined', 'function'])
  assert.equal(app.closed(), 0)
  app.tones[3].tone.onended()
  assert.equal(app.closed(), 1)
  app.update({ a: 'done', b: 'working' })
  app.update({ a: 'done', b: 'done' })
  assert.equal(app.tones.length, 8)
  app.update({}) // Disconnect and reconnect must not replay completion.
  app.update({ a: 'done', b: 'done' })
  assert.equal(app.tones.length, 8)
  app.update({ a: 'working' })
  app.update({ a: 'error' })
  app.update({ a: 'done' })
  assert.equal(app.tones.length, 8)
  app.update({ a: 'working' })
  app.update({ a: 'done' })
  assert.equal(app.tones.length, 12)
})

test('sound preference persists and muted completions are not replayed', () => {
  const app = setup({ sidebarWidth: 300 })
  vm.runInContext("saveNotificationSound('off')", app.context)
  assert.deepEqual(app.stored(), { sidebarWidth: 300, notificationSound: false })
  app.update({ a: 'working' }); app.update({ a: 'done' })
  assert.equal(app.tones.length, 0)
  vm.runInContext("saveNotificationSound('on')", app.context)
  app.update({ a: 'done' })
  assert.equal(app.tones.length, 0)
  app.update({ a: 'working' }); app.update({ a: 'done' })
  assert.equal(app.tones.length, 4)
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
  assert.equal(app.tones.length, 4)
})
