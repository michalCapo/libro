const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const main = fs.readFileSync(path.join(__dirname, 'main.js'), 'utf8')
const forwarding = main.slice(main.indexOf('    // Keep workspace navigation'), main.indexOf('    // Forward Ctrl+;'))

test('browser forwards workspace shortcuts, including repeat state', () => {
  for (const key of ['a', 'b', 'h', 'l', 'p']) {
    for (const repeat of [false, true]) {
      let prevented = false
      let forwarded
      const input = { key, code: 'Key' + key.toUpperCase(), control: true, shift: key === 'p', isAutoRepeat: repeat }
      vm.runInNewContext('(function () {' + forwarding + '})()', {
        key, input, shouldSkipDuplicateShortcut: () => false,
        e: { preventDefault() { prevented = true } },
        mainWindow: { webContents: { executeJavaScript(script) {
          vm.runInNewContext(script, {
            window: { dispatchEvent(event) { forwarded = event } },
            KeyboardEvent: function (type, options) { return { type, ...options } },
          })
          return Promise.resolve()
        } } },
      })
      assert.equal(prevented, true)
      assert.equal(forwarded.key, key)
      assert.equal(forwarded.ctrlKey, true)
      assert.equal(forwarded.repeat, repeat)
    }
  }
})

test('Ctrl+A hides tools and selects the remembered agent without requiring an overlay', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("    if (binding === 'Ctrl+A')"), workspace.indexOf("    if (binding === 'Ctrl+D'"))
  const panels = [
    { dataset: { appId: 'agent-1', dock: 'center' } },
    { dataset: { appId: 'agent-2', dock: 'center' } },
    { dataset: { appId: 'browser', dock: 'right' } },
  ]
  const state = { agent: 'agent-2', hidden: new Set() }
  let selected
  vm.runInNewContext('(function () {' + handler + '})()', {
    binding: 'Ctrl+A', activeGrid: () => ({}), frames: () => panels,
    dockState: () => state, select: id => { selected = id },
    event: { preventDefault() {}, stopImmediatePropagation() {} },
  })
  assert.equal(state.hidden.has('browser'), true)
  assert.equal(selected, 'agent-2')
})
