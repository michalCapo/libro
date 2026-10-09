const {test} = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const {JSDOM} = require('jsdom')

const source = fs.readFileSync(path.join(__dirname,'../internal/components/voice_input.js'),'utf8')

function harness(markup) {
  const dom = new JSDOM(markup,{runScripts:'outside-only',pretendToBeVisual:true})
  dom.window.HTMLElement.prototype.getClientRects = () => [1]
  dom.window.document.execCommand = () => false
  dom.window.eval(source)
  return {window:dom.window,document:dom.window.document,input:dom.window.libroVoiceInput}
}

for (const tag of ['input','textarea']) {
  test(tag+' replaces the captured selection, emits input, and does not submit', () => {
    const h = harness(`<form><${tag} id="field"></${tag}><input id="other"><button type="submit">Send</button></form>`)
    const field = h.document.getElementById('field')
    field.value='Before old after'; field.focus(); field.setSelectionRange(7,10)
    const token = h.input.capture()
    let events=0,submitted=false
    field.addEventListener('input', event => { events++; assert.equal(event.data,'new text') })
    h.document.querySelector('form').addEventListener('submit',() => { submitted=true })
    h.document.getElementById('other').focus()
    assert.equal(h.input.insert(token,'new text'),true)
    assert.equal(field.value,'Before new text after')
    assert.equal(field.selectionStart,15)
    assert.equal(events,1)
    assert.equal(submitted,false)
    h.window.close()
  })
}

test('captures a textarea inside an open shadow root', () => {
  const h = harness('<div id="prompt"></div>')
  const root=h.document.getElementById('prompt').attachShadow({mode:'open'})
  const field=h.document.createElement('textarea'); root.append(field)
  field.focus()
  const token=h.input.capture()
  assert.ok(token)
  assert.equal(h.input.insert(token,'Describe this element'),true)
  assert.equal(field.value,'Describe this element')
  h.window.close()
})

test('keyboard focus on the shared microphone retains the previous input', () => {
  const h=harness('<input id="field"><div data-voice-control><button id="mic">Mic</button></div>')
  const field=h.document.getElementById('field'); field.focus()
  h.document.getElementById('mic').focus()
  assert.equal(h.input.insert(h.input.capture(),'Dictation'),true)
  assert.equal(field.value,'Dictation')
  h.window.close()
})

test('email inputs update through their native setter', () => {
  const h=harness('<input type="email">')
  const field=h.document.querySelector('input'); field.value='user'; field.focus()
  assert.equal(h.input.insert(h.input.capture(),'@example.test'),true)
  assert.equal(field.value,'user@example.test')
  h.window.close()
})

test('read-only fields and xterm helper textareas are not direct dictation targets', () => {
  const h=harness('<textarea readonly></textarea><div data-terminal><textarea></textarea></div>')
  for(const field of h.document.querySelectorAll('textarea')) {
    field.focus(); assert.equal(h.input.capture(),0)
  }
  h.window.close()
})

test('removed fields and superseded captures reject stale insertions', () => {
  const h=harness('<input id="one"><input id="two">')
  const one=h.document.getElementById('one'),two=h.document.getElementById('two')
  one.focus(); const first=h.input.capture()
  two.focus(); const second=h.input.capture()
  assert.equal(h.input.insert(first,'Stale'),false)
  two.remove()
  assert.equal(h.input.insert(second,'Stale'),false)
  assert.equal(one.value,'')
  h.window.close()
})

for(const electron of [false,true]) {
  test((electron?'Electron guest':'same-origin iframe')+' inserts into its captured shadow input', async () => {
    const h=harness('<div id="prompt"></div>')
    const root=h.document.getElementById('prompt').attachShadow({mode:'open'})
    const field=h.document.createElement('textarea'); root.append(field)
    field.value='Before old after'; field.focus(); field.setSelectionRange(7,10)
    const guest={isConnected:true,contentWindow:h.window}
    if(electron) guest.executeJavaScript=async script => h.window.eval(script)
    const browser=fs.readFileSync(path.join(__dirname,'../internal/components/browser.go'),'utf8')
    const start=browser.indexOf('window.__libroCaptureBrowserVoiceInput =')
    const end=browser.indexOf('\nfunction executePageToolMode',start)
    const window={__libroVoiceInputScript:source}
    vm.runInNewContext(browser.slice(start,end),{window,pageToolWebview:() => guest})
    const target=await window.__libroCaptureBrowserVoiceInput('browser')
    assert.ok(target.available())
    assert.equal(await target.insert('quoted "text"'),true)
    assert.equal(field.value,'Before quoted "text" after')
    field.remove()
    assert.equal(await target.insert('Stale'),false)
    h.window.close()
  })
}
