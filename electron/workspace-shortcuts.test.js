const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const main = fs.readFileSync(path.join(__dirname, 'main.js'), 'utf8')
const forwarding = main.slice(main.indexOf('    // Keep workspace navigation'), main.indexOf('    // Forward Ctrl+;'))

test('browser forwards workspace shortcuts, including repeat state', () => {
  for (const key of ['a', 'b', 'h', 'l', 'p', 'q', 'r', 't', ',', '.']) {
    for (const repeat of [false, true]) {
      let prevented = false
      let forwarded
      const input = { key, code: 'Key' + key.toUpperCase(), control: true, shift: ['p', 'q', 'r', 't'].includes(key), isAutoRepeat: repeat }
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

test('project command shortcuts dispatch once and support custom bindings', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("    if (binding && (binding === toolKeys['run-project']"), workspace.indexOf("    if (binding && binding === toolKeys['close-project'])"))
  for (const [binding, action] of [['Ctrl+Shift+R', 'project.command.run'], ['Ctrl+Shift+T', 'project.command.stop'], ['Alt+R', 'project.command.run']]) {
    for (const repeat of [false, true]) {
      const calls = []
      vm.runInNewContext('(function () {' + handler + '})()', {
        binding, toolKeys: { 'run-project': binding === 'Alt+R' ? binding : 'Ctrl+Shift+R', 'stop-project': 'Ctrl+Shift+T' },
        call: action => calls.push(action), event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
      })
      assert.deepEqual(calls, repeat ? [] : [action])
    }
  }
})

test('a hidden bottom shell exiting keeps the visible project command open', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf('  function terminalExited(id)'), workspace.indexOf('  function bottom()'))
  for (const id of ['shell', 'project']) {
    const state = { bottom: true, bottomID: 'project' }
    const calls = []
    vm.runInNewContext(handler + ';terminalExited(id)', {
      id, document: { getElementById: () => ({ dataset: { dock: 'bottom' }, parentElement: {} }) },
      dockState: () => state, call: (action, data) => calls.push([action, data.id]), refresh() {},
    })
    assert.equal(state.bottom, id === 'shell')
    assert.deepEqual(calls, [['app.close', id]])
  }
})

test('panel size shortcuts use saved bindings and ignore key repeat', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("    if (binding && (binding === toolKeys['panel-size-down']"), workspace.indexOf("    if (binding && binding === toolKeys['toggle-projects'])"))
  for (const [binding, delta] of [['Ctrl+,', -1], ['Ctrl+.', 1], ['Alt+S', -1]]) {
    for (const repeat of [false, true]) {
      const calls = []
      vm.runInNewContext('(function () {' + handler + '})()', {
        binding, sid: 'session',
        toolKeys: { 'panel-size-down': binding === 'Alt+S' ? binding : 'Ctrl+,', 'panel-size-up': 'Ctrl+.' },
        window: { __libroResizeSelectedAppStep: (...args) => calls.push(args) },
        event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
      })
      assert.deepEqual(calls, repeat ? [] : [[delta, 'session']])
    }
  }
})

test('panel navigation includes the side-by-side tool in visual order and wraps', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("    if (binding && (binding === toolKeys['previous-agent']"), workspace.indexOf("    if (binding && binding === toolKeys['new-agent'])"))
  const panel = (appId, dock, visible = true, overlay = false) => ({ dataset: {
    appId, dock, dockVisible: String(visible), toolOverlay: String(overlay),
  } })
  const agent = panel('agent', 'center')
  const tool = panel('tool', 'right')
  const panels = [tool, panel('hidden', 'right', false), agent, panel('bottom', 'bottom')]
  function navigate(panels, from, binding, repeat = false) {
    const state = { agent: 'agent', hidden: new Set() }
    let selected
    vm.runInNewContext('(function () {' + handler + '})()', {
      binding, toolKeys: { 'previous-agent': 'Ctrl+H', 'next-agent': 'Ctrl+L' },
      activeGrid: () => ({ querySelector: () => panels.some(p => p.dataset.toolOverlay === 'true') }),
      frames: () => panels, dockState: () => state,
      window: { __libroSelectedApp: from }, select: id => { selected = id },
      event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
    })
    return { selected, state }
  }
  for (const key of ['Ctrl+H', 'Ctrl+L']) {
    assert.equal(navigate(panels, 'agent', key).selected, 'tool')
    assert.equal(navigate(panels, 'tool', key).selected, 'agent')
    assert.equal(navigate(panels, 'agent', key, true).selected, undefined)
  }
  const twoAgents = [...panels, panel('agent-2', 'center')]
  assert.equal(navigate(twoAgents, 'agent', 'Ctrl+L').selected, 'agent-2')
  assert.equal(navigate(twoAgents, 'agent-2', 'Ctrl+L').selected, 'tool')
  assert.equal(navigate(twoAgents, 'tool', 'Ctrl+H').selected, 'agent-2')
  const overlay = [agent, panel('agent-2', 'center'), panel('tool', 'right', true, true)]
  const result = navigate(overlay, 'tool', 'Ctrl+L')
  assert.equal(result.selected, 'agent-2')
  assert.equal(result.state.hidden.has('tool'), true)
  assert.equal(navigate([agent], 'agent', 'Ctrl+L').selected, undefined)
})

test('close project uses saved binding and ignores repeat', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("    if (binding && binding === toolKeys['close-project'])"), workspace.indexOf("    if (binding && binding === toolKeys['close-panel'])"))
  for (const binding of ['Ctrl+Shift+Q', 'Alt+Q']) {
    for (const repeat of [false, true]) {
      const calls = []
      vm.runInNewContext('(function () {' + handler + '})()', {
        binding, toolKeys: { 'close-project': binding },
        event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
        call: action => calls.push(action),
      })
      assert.deepEqual(calls, repeat ? [] : ['project.close'])
    }
  }
})
