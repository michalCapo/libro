const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const main = fs.readFileSync(path.join(__dirname, 'main.js'), 'utf8')
const forwarding = main.slice(main.indexOf('    // Keep workspace navigation'), main.indexOf('    const isZoomInKey'))
const matching = main.slice(main.indexOf('let workspaceShortcuts'), main.indexOf('// Find the Go binary'))
const defaults = [...fs.readFileSync(path.join(__dirname, '../internal/keybindings.go'), 'utf8')
  .matchAll(/\{"[^"\n]+", "[^"\n]+", "([^"]+)"\}/g)].map(match => match[1])

function inputFor(binding, repeat = false) {
  const key = binding.split('+').at(-1).toLowerCase()
  return { key, code: 'Key' + key.toUpperCase(), control: binding.includes('Ctrl+'),
    alt: binding.includes('Alt+'), meta: binding.includes('Meta+'), shift: binding.includes('Shift+'), isAutoRepeat: repeat }
}

test('browser forwards every saved shortcut before browser actions, with modifiers and repeat state', () => {
  const bindings = [...defaults, 'Alt+R', 'Ctrl+Alt+Shift+Meta+F', 'Meta+=']
  for (const binding of [...bindings, 'Ctrl+A', 'Ctrl+1', 'Ctrl+9', 'Ctrl+`']) {
    for (const repeat of [false, true]) {
      let prevented = false
      let forwarded
      const input = inputFor(binding, repeat)
      vm.runInNewContext(matching + '\nworkspaceShortcuts = new Set(bindings); (function () {' + forwarding + '})()', {
        bindings, input, isMainWindowContents: false, shouldSkipDuplicateShortcut: () => false,
        e: { preventDefault() { prevented = true } },
        mainWindow: { webContents: { executeJavaScript(script) {
          vm.runInNewContext(script, {
            window: { dispatchEvent(event) { forwarded = event } },
            KeyboardEvent: function (type, options) { return { type, ...options } },
          })
          return Promise.resolve()
        } } },
      })
      assert.equal(prevented, true, binding)
      assert.equal(forwarded.key, input.key)
      assert.equal(forwarded.ctrlKey, input.control)
      assert.equal(forwarded.altKey, input.alt)
      assert.equal(forwarded.metaKey, input.meta)
      assert.equal(forwarded.shiftKey, input.shift)
      assert.equal(forwarded.repeat, repeat)
    }
  }
})

test('updated and disabled bindings release browser shortcuts and normalize plus', () => {
  const context = vm.createContext({})
  vm.runInContext(matching, context)
  const matches = input => { context.input = input; return vm.runInContext('isWorkspaceShortcut(input)', context) }
  vm.runInContext("workspaceShortcuts = new Set(['Ctrl+F', 'Ctrl+='])", context)
  assert.equal(matches(inputFor('Ctrl+F')), true)
  assert.equal(matches({ key: '+', control: true, shift: true }), true)
  assert.equal(matches(inputFor('Ctrl+Shift+F')), false)
  vm.runInContext("workspaceShortcuts = new Set(['Alt+F'])", context)
  assert.equal(matches(inputFor('Ctrl+F')), false)
  assert.equal(matches(inputFor('Alt+F')), true)
  assert.equal(matches({ key: 'f' }), false)
})

test('browser forwards the physical bottom terminal shortcut across keyboard layouts', () => {
  for (const key of ['`', 'Dead', '§', ';']) {
    let toggled = 0
    let prevented = 0
    const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
    const bottomHandler = workspace.slice(workspace.indexOf("  window.addEventListener('keydown', event => {\n    if (!event.ctrlKey"), workspace.indexOf('  function terminalExited'))
    const renderer = vm.createContext({
      window: {
        addEventListener(type, handler) { this.handler = handler },
        dispatchEvent(event) { this.handler(event) },
      },
      KeyboardEvent: function (type, options) {
        return { type, ...options, preventDefault() {}, stopImmediatePropagation() {} }
      },
      bottom() { toggled++ },
    })
    vm.runInContext(bottomHandler, renderer)
    for (const repeat of [false, true]) {
      vm.runInNewContext(matching + '\n(function () {' + forwarding + '})()', {
        input: { key, code: 'Backquote', control: true, isAutoRepeat: repeat },
        isMainWindowContents: false, shouldSkipDuplicateShortcut: () => false,
        e: { preventDefault() { prevented++ } },
        mainWindow: { webContents: { executeJavaScript(script) {
          vm.runInContext(script, renderer)
          return Promise.resolve()
        } } },
      })
    }
    assert.equal(prevented, 2, key)
    assert.equal(toggled, 1, key)
  }
  const context = vm.createContext({})
  vm.runInContext(matching, context)
  for (const modifier of ['alt', 'meta', 'shift']) {
    context.input = { key: 'Dead', code: 'Backquote', control: true, [modifier]: true }
    assert.equal(vm.runInContext('isWorkspaceShortcut(input)', context), false, modifier)
  }
})

test('workspace syncs initial bindings and saved settings to Electron', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const init = source.slice(source.indexOf('  let toolKeys'), source.indexOf('  function shortcut'))
  const hints = source.slice(source.indexOf('  function updateToolHints'), source.indexOf('  function saveToolKeys'))
  const saved = source.slice(source.indexOf('  function toolKeysSaved'), source.indexOf("  window.addEventListener('keydown'"))
  const calls = []
  const context = vm.createContext({
    window: { __libroToolKeys: { files: 'Ctrl+F' }, libroElectron: { setWorkspaceShortcuts: bindings => calls.push(Array.from(bindings)) } },
    document: { querySelectorAll: () => [], querySelector: () => ({}), getElementById: () => ({}) },
  })
  vm.runInContext(init + hints + saved, context)
  vm.runInContext("toolKeysSaved({files: 'Alt+F'}, 'Saved')", context)
  vm.runInContext("toolKeysSaved({files: ''}, 'Saved')", context)
  assert.deepEqual(calls, [['Ctrl+F'], ['Alt+F'], ['']])
})

for (const overlay of [false, true]) test('Ctrl+A selects the remembered agent with overlay=' + overlay, () => {
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
    binding: 'Ctrl+A', activeGrid: () => ({ querySelector: () => overlay }), frames: () => panels,
    dockState: () => state, select: id => { selected = id },
    event: { preventDefault() {}, stopImmediatePropagation() {} },
  })
  assert.equal(state.hidden.has('browser'), overlay)
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
  for (const [binding, delta] of [['Ctrl+,', 1], ['Ctrl+.', -1], ['Alt+S', -1]]) {
    for (const repeat of [false, true]) {
      const calls = []
      vm.runInNewContext('(function () {' + handler + '})()', {
        binding, sid: 'session',
        toolKeys: { 'panel-size-down': binding === 'Alt+S' ? binding : 'Ctrl+.', 'panel-size-up': 'Ctrl+,' },
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

test('project numbers skip empty projects and update when the last panel closes', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const render = source.slice(source.indexOf('  function renderProjectShortcuts()'), source.indexOf('  let notificationAudio;'))
  const rows = Array.from({ length: 12 }, (_, i) => ({
    dataset: { projectKey: String(i) }, attributes: {}, badge: null,
    querySelector() { return this.badge },
    append(badge) { this.badge = badge; badge.remove = () => { this.badge = null } },
    setAttribute(key, value) { this.attributes[key] = value },
    removeAttribute(key) { delete this.attributes[key] },
  }))
  const grids = rows.map((row, i) => ({ dataset: { workspaceProject: row.dataset.projectKey }, panels: i ? [{}] : [] }))
  const context = vm.createContext({
    document: { querySelectorAll: selector => selector === '.ws-project-row' ? rows : grids },
    frames: grid => grid.panels,
    node: () => ({ setAttribute() {} }),
  })
  vm.runInContext(render + '\nrenderProjectShortcuts();', context)
  assert.deepEqual(rows.map(row => row.dataset.projectShortcut), ['', '1', '2', '3', '4', '5', '6', '7', '8', '9', '', ''])
  assert.equal(rows[1].badge.textContent, '1')
  assert.equal(rows[1].attributes['aria-keyshortcuts'], 'Control+1')
  grids[1].panels = []
  vm.runInContext('renderProjectShortcuts()', context)
  assert.equal(rows[1].badge, null)
  assert.equal(rows[1].attributes['aria-keyshortcuts'], undefined)
  assert.equal(rows[2].dataset.projectShortcut, '1')
  assert.equal(rows[10].dataset.projectShortcut, '9')
})

test('Ctrl+1–9 selects the numbered project once', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('    if (event.ctrlKey &&'), source.indexOf("    if (binding && (binding === toolKeys['panel-size-down']"))
  for (const key of ['1', '9']) for (const repeat of [false, true]) {
    let clicked = 0
    vm.runInNewContext('(function () {' + handler + '})()', {
      event: { key, ctrlKey: true, repeat, preventDefault() {}, stopImmediatePropagation() {} },
      renderProjectShortcuts() {},
      document: { querySelector(selector) {
        assert.equal(selector, '.ws-project-row[data-project-shortcut="' + key + '"]')
        return { click() { clicked++ } }
      } },
    })
    assert.equal(clicked, repeat ? 0 : 1)
  }
})

for (const overlay of [false, true]) test('clicking an agent preserves side-by-side tools with overlay=' + overlay, () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("  root.addEventListener('pointerdown', event => {"), workspace.indexOf('  updateToolHints();', workspace.indexOf("  root.addEventListener('pointerdown', event => {")))
  const grid = { querySelector: () => overlay }
  const agent = { dataset: { appId: 'agent', dock: 'center' }, parentElement: grid }
  const browser = { dataset: { appId: 'browser', dock: 'right' } }
  const state = { hidden: new Set() }
  const window = { __libroSelectedApp: 'browser' }
  const calls = []
  vm.runInNewContext(handler, {
    root: { addEventListener: (type, callback) => callback({ target: { closest: () => agent } }) },
    window, frames: () => [agent, browser], dockState: () => state,
    refresh() {}, call: (action, data) => calls.push([action, data.index, data.focus]),
  })
  assert.equal(state.hidden.has('browser'), overlay)
  assert.equal(window.__libroSelectedApp, 'agent')
  assert.deepEqual(calls, [['app.select', 0, false]])
})
