const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { JSDOM } = require('jsdom')

const source = fs.readFileSync(path.join(__dirname, '../internal/components/browser.go'), 'utf8')
const start = source.indexOf('function initAll(')
const code = source.slice(start, source.indexOf("window.addEventListener('resize'", start))
const tick = () => new Promise(resolve => setImmediate(resolve))

function harness(t, electron = true) {
  const dom = new JSDOM('<body><webview data-webview-app="existing"></webview><div data-terminal="agent"></div></body>', { runScripts: 'outside-only' })
  t.after(() => dom.window.close())
  const initialized = []
  const context = dom.getInternalVMContext()
  context.libroElectron = electron
  context.initWebview = node => initialized.push(node.getAttribute('data-webview-app'))
  vm.runInContext(code, context)
  return { document: dom.window.document, initialized }
}

test('browser discovery handles initial, direct and nested webview additions', async t => {
  const { document, initialized } = harness(t)
  assert.deepEqual(initialized, ['existing'])
  document.body.insertAdjacentHTML('beforeend', '<webview data-webview-app="direct"></webview><section><webview data-webview-app="nested"></webview></section>')
  await tick()
  assert.deepEqual(initialized, ['existing', 'direct', 'nested'])
})

test('terminal redraws and removals do not rescan the document or output', async t => {
  const { document, initialized } = harness(t)
  let scans = 0
  document.querySelectorAll = () => { scans++; return [] }
  const terminal = document.querySelector('[data-terminal]')
  for (let i = 0; i < 100; i++) {
    const row = document.createElement('span')
    row.textContent = 'terminal output'
    row.querySelectorAll = () => { scans++; return [] }
    terminal.append(row)
  }
  await tick()
  terminal.replaceChildren()
  document.querySelector('webview').remove()
  await tick()
  assert.equal(scans, 0)
  assert.deepEqual(initialized, ['existing'])
})

test('plain browser keeps webviews inert', async t => {
  const { document, initialized } = harness(t, false)
  document.body.insertAdjacentHTML('beforeend', '<webview data-webview-app="new"></webview>')
  await tick()
  assert.deepEqual(initialized, [])
})
