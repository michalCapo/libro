const {test} = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

const source = fs.readFileSync(path.join(__dirname, '../internal/voice.js'), 'utf8')
const tick = () => new Promise(resolve => setImmediate(resolve))

async function harness(options = {}) {
  const events = {}
  const frames = {agent: {isConnected:true, closest:() => null, getClientRects:() => [1]}}
  const calls = {stops:0, recordings:0, pastes:[], requests:[]}
  let mutation
  const track = {stop() { calls.stops++ }}
  const stream = {getTracks:() => [track], getAudioTracks:() => [track]}
  const window = {
    __libroWorkspaceSID:'session-test',
    __libroSelectedApp:options.selected,
    addEventListener(name, callback) { (events[name] ||= []).push(callback) },
    MediaRecorder:true,
    __libroSendPageToolPrompt(...args) { calls.pastes.push(args); return true },
  }
  class Recorder {
    constructor() { this.state = 'inactive'; this.mimeType = 'audio/webm' }
    start() { this.state = 'recording'; calls.recordings++ }
    stop() {
      this.state = 'inactive'
      queueMicrotask(() => { this.ondataavailable({data:new Blob(['audio'])}); this.onstop() })
    }
  }
  vm.runInNewContext(source, {
    window, document:{querySelectorAll:() => [], addEventListener() {}, getElementById:id => id === 'libro-workspace' ? {} : frames[id.replace('frame-', '')]},
    navigator:{mediaDevices:{getUserMedia:options.getUserMedia || (() => Promise.resolve(stream))}},
    MediaRecorder:Recorder,
    AudioContext:class {
      async decodeAudioData() { return {getChannelData:() => new Float32Array(16000).fill(options.silent ? 0 : 0.2)} }
      async close() {}
    },
    MutationObserver:class { constructor(callback) { mutation = callback } observe() {} },
    async fetch(url, init) {
      calls.requests.push({url, ...init})
      if (url === '/voice/status') return {ok:true, json:async () => ({state:'ready'})}
      if (url === '/voice/transcribe') return options.transcribe ? options.transcribe(init) : {ok:true, json:async () => ({text:'Hello Libro'})}
      return {ok:true}
    },
    setTimeout, clearTimeout, AbortController, Blob, ArrayBuffer, DataView,
  })
  await tick()
  return {
    voice:window.libroVoice, calls, frames, stream, window,
    mutate:() => mutation(),
    event(name, data = {}) {
      const event = {preventDefault() {}, stopImmediatePropagation() {}, ...data}
      for (const callback of events[name] || []) callback(event)
    },
  }
}

test('second press transcribes mono PCM and pastes into the captured agent without Enter', async () => {
  const h = await harness()
  await h.voice.toggle('agent')
  assert.equal(h.calls.recordings, 1)
  h.event('keyup', {code:'CapsLock', key:'CapsLock'})
  await tick()
  assert.equal(h.calls.stops, 0, 'releasing Caps Lock keeps recording')
  await h.voice.toggle('agent')
  await tick()
  assert.deepEqual(h.calls.pastes, [['Hello Libro', false, 'agent']])
  const request = h.calls.requests.find(r => r.url === '/voice/transcribe')
  assert.equal(request.headers['X-Libro-Session'], 'session-test')
  const wav = new DataView(request.body)
  assert.equal(wav.getUint32(24, true), 16000)
  assert.equal(wav.getUint16(22, true), 1)
  assert.equal(wav.getUint32(40, true), 32000)
  assert.ok(h.calls.stops > 0)
})

test('pressing again before microphone permission resolves does not start recording', async () => {
  let grant
  const h = await harness({getUserMedia:() => new Promise(resolve => { grant = resolve })})
  const pending = h.voice.toggle('agent')
  await h.voice.toggle('agent')
  grant(h.stream)
  await pending
  assert.equal(h.calls.recordings, 0)
  assert.equal(h.calls.stops, 1)
})

for (const reason of ['Escape', 'blur', 'switch-thread', 'close-agent']) {
  test(reason + ' cancels capture and stops the microphone', async () => {
    const h = await harness()
    await h.voice.toggle('agent')
    if (reason === 'Escape') h.event('keydown', {key:'Escape'})
    else if (reason === 'switch-thread') { h.frames.agent.closest = () => ({}); h.mutate() }
    else if (reason === 'close-agent') { h.frames.agent.isConnected = false; h.mutate() }
    else h.event(reason, {pointerId:7})
    await tick()
    assert.equal(h.calls.pastes.length, 0)
    assert.ok(h.calls.stops > 0)
    assert.equal(h.calls.requests.filter(r => r.url === '/voice/transcribe').length, 0)
  })
}

test('switching threads aborts transcription and ignores a late result', async () => {
  let finish
  let signal
  const h = await harness({transcribe:init => {
    signal = init.signal
    return new Promise(resolve => { finish = resolve })
  }})
  await h.voice.toggle('agent')
  await h.voice.toggle('agent')
  await tick()
  h.frames.agent.closest = () => ({})
  h.mutate()
  assert.equal(signal.aborted, true)
  finish({ok:true, json:async () => ({text:'Must not be pasted'})})
  await tick()
  assert.equal(h.calls.pastes.length, 0)
})

test('silence does not invoke transcription', async () => {
  const h = await harness({silent:true})
  await h.voice.toggle('agent')
  await h.voice.toggle('agent')
  await tick()
  assert.equal(h.calls.requests.filter(r => r.url === '/voice/transcribe').length, 0)
})

test('microphone denial permits another attempt', async () => {
  let attempts = 0
  const h = await harness({getUserMedia:async () => { attempts++; throw Object.assign(new Error('denied'), {name:'NotAllowedError'}) }})
  await h.voice.toggle('agent')
  await h.voice.toggle('agent')
  assert.equal(attempts, 2)
})

test('guest key releases leave recording active and Escape cancels', async () => {
  const main = fs.readFileSync(path.join(__dirname, 'main.js'), 'utf8')
  const start = main.indexOf("  contents.on('before-input-event', (e, input) => {")
  const end = main.indexOf('    // Native terminals live', start)
  const handler = main.slice(start, end) + '\n  })'
  const h = await harness()
  let callback
  const host = {id:1, executeJavaScript:async script => {
    if (script.includes('libroVoice?.cancel')) h.voice.cancel()
    else vm.runInNewContext(script, {
      window:{dispatchEvent:event => h.event(event.type, event)},
      KeyboardEvent:class { constructor(type, data) { Object.assign(this, {type}, data) } },
    })
  }}
  vm.runInNewContext(handler, {
    contents:{id:2, getType:() => 'webview', on(_name, fn) { callback = fn }},
    mainWindow:{webContents:host, isDestroyed:() => false},
  })
  await h.voice.toggle('agent')
  callback({}, {type:'keyUp', key:'CapsLock', code:'CapsLock'})
  await tick()
  assert.equal(h.calls.pastes.length, 0)
  assert.equal(h.calls.stops, 0)
  callback({}, {type:'keyDown', key:'Escape', code:'Escape'})
  await tick()
  assert.equal(h.calls.pastes.length, 0, 'Escape must cancel without a paste')
  assert.ok(h.calls.stops > 0)
})

test('dictation inserts into the terminal selected at recording start', async () => {
  const h = await harness({selected:'git'})
  h.frames.git = {isConnected:true, closest:() => null, getClientRects:() => [1], querySelector:() => ({})}
  await h.voice.toggle('agent')
  h.window.__libroSelectedApp = 'agent'
  await h.voice.toggle('agent')
  await tick()
  assert.deepEqual(h.calls.pastes, [['Hello Libro', false, 'git']])
})

test('a selected browser keeps dictation targeted at the agent', async () => {
  const h = await harness({selected:'browser'})
  h.frames.browser = {isConnected:true, closest:() => null, getClientRects:() => [1], querySelector:() => null}
  await h.voice.toggle('agent')
  await h.voice.toggle('agent')
  await tick()
  assert.deepEqual(h.calls.pastes, [['Hello Libro', false, 'agent']])
})
