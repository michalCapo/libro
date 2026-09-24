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

const toolIDs = [...fs.readFileSync(path.join(__dirname, '../internal/plugins.go'), 'utf8')
  .matchAll(/ID: "([^"]+)"[^\n]+Dock: "right"/g)].map(match => match[1])

for (const plugin of [...toolIDs, 'custom-tool']) test(plugin + ' focuses an open panel before toggling it closed', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function tool(id)'), source.indexOf('  let toolKeys'))
  for (const visible of [false, true]) for (const selected of ['agent', 'git']) {
    const state = { hidden: new Set(visible ? [] : ['git']) }
    const calls = []
    vm.runInNewContext(handler + ';tool(plugin)', {
      plugin,
      activeGrid: () => ({}), dockState: () => state,
      frames: () => [{ dataset: { dock: 'right', plugin, appId: 'git', dockVisible: String(visible) } }],
      window: { __libroSelectedApp: selected },
      select(id) { state.hidden.delete(id); calls.push(['select', id]) },
      refresh() { calls.push(['refresh']) },
      restoreAgentFocus() { calls.push(['restoreAgentFocus']) },
    })
    const hide = visible && selected === 'git'
    assert.equal(state.hidden.has('git'), hide)
    assert.deepEqual(calls, hide ? [['restoreAgentFocus']] : [['select', 'git']])
  }
})

test('hiding a tool restores actual focus to the last agent in the active project', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const select = source.slice(source.indexOf('  function select('), source.indexOf('  function launcher('))
  const handlers = source.slice(source.indexOf('  function restoreAgentFocus('), source.indexOf('  function newBrowser('))
  for (const remembered of ['agent-2', 'closed-agent', undefined]) for (const hasAgents of [true, false]) {
    const grid = {}
    const state = { agent: remembered, right: 'git', hidden: new Set() }
    const panels = (hasAgents ? ['agent-1', 'agent-2'] : []).map(appId => ({
      dataset: { appId, dock: 'center' }, parentElement: grid, querySelector: () => null,
    }))
    panels.push({ dataset: { appId: 'git', dock: 'right', plugin: 'lazyrepo', dockVisible: 'true' }, parentElement: grid })
    const focused = [], calls = [], pending = []
    const window = { __libroSelectedApp: 'git', __libroScrollToApp() {}, __libroFocusAppByID(id) { focused.push(id) } }
    vm.runInNewContext(select + handlers + ';tool("lazyrepo")', {
      window, keepBottomHidden: false, maximized: '', activeGrid: () => grid,
      frames: () => panels, dockState: () => state,
      document: { getElementById(id) { return id === 'workspace-settings' ? { hidden: true } : panels.find(p => 'frame-' + p.dataset.appId === id) } },
      refresh() {}, call(action, data) { calls.push([action, data.index]) },
      requestAnimationFrame(fn) { pending.push(fn) },
    })
    pending.forEach(fn => fn())
    const expected = remembered === 'agent-2' ? 'agent-2' : 'agent-1'
    assert.equal(state.hidden.has('git'), true)
    assert.deepEqual(focused, hasAgents ? [expected] : [])
    assert.equal(window.__libroSelectedApp, hasAgents ? expected : 'git')
    assert.deepEqual(calls, hasAgents ? [['app.select', expected === 'agent-2' ? 1 : 0]] : [])
  }
})

test('project switches preserve bottom terminal visibility when restoring selection', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const layout = source.slice(source.indexOf('  function layoutDocks('), source.indexOf('    let tabs = grid.parentElement')) + '\n  }'
  for (const visible of [false, true]) {
    const state = { bottom: visible, hidden: new Set() }
    const grid = { dataset: {}, style: {}, clientWidth: 1000, querySelector: () => ({}) }
    const shell = {
      dataset: { appId: 'shell', dock: 'bottom', dockSeen: 'true' },
      style: {}, querySelector: () => ({}),
    }
    const window = { __libroSelectedApp: 'shell' }
    const context = { grid, shell, window, prefs: {}, resizeHandle() {}, dockState: () => state, keepBottomHidden: false }
    // The inactive project's layout clears lastSelected before switching back.
    vm.runInNewContext(layout + ';layoutDocks(grid, [shell]);', context)
    window.__libroSelectedApp = 'other-project-agent'
    vm.runInNewContext(layout + ';layoutDocks(grid, [shell]);', context)
    window.__libroSelectedApp = 'shell'
    vm.runInNewContext(layout + ';layoutDocks(grid, [shell]);', context)
    assert.equal(state.bottom, visible)
    assert.equal(shell.dataset.dockVisible, String(visible))
    assert.equal(shell.inert, !visible)
  }
})

test('bottom terminal focuses before toggling closed', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function bottom()'), source.indexOf('  function layoutDocks'))
  for (const visible of [false, true]) for (const selected of ['agent', 'shell']) {
    const state = { bottom: visible, bottomID: 'shell' }
    const calls = []
    vm.runInNewContext(handler + ';bottom()', {
      activeGrid: () => ({}), dockState: () => state,
      frames: () => [{ dataset: { dock: 'bottom', appId: 'shell' } }],
      window: { __libroSelectedApp: selected },
      select(id) { calls.push(['select', id]) },
      refresh() { calls.push(['refresh']) },
      restoreAgentFocus() { calls.push(['restoreAgentFocus']) },
    })
    const hide = visible && selected === 'shell'
    assert.equal(state.bottom, !hide)
    assert.deepEqual(calls, hide ? [['restoreAgentFocus']] : [['select', 'shell']])
  }
})

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

test('settings shortcut opens and closes settings and ignores key repeat', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const settingsHandlerStart = workspace.indexOf("    if (binding && binding === toolKeys['settings'])")
  const settingsHandler = workspace.slice(settingsHandlerStart, workspace.indexOf("    if (!document.getElementById('workspace-settings').hidden", settingsHandlerStart))
  const settingsFunction = workspace.slice(workspace.indexOf('  let settingsFocus;'), workspace.indexOf('  function themePreference()'))
  for (const hidden of [true, false]) for (const repeat of [false, true]) {
    const calls = []
    const page = { hidden }
    const document = {
      activeElement: {},
      getElementById: () => page,
      querySelector: () => null,
    }
    vm.runInNewContext(settingsFunction + '\n(function () {' + settingsHandler + '})()', {
      binding: 'Ctrl+Shift+S', toolKeys: { settings: 'Ctrl+Shift+S' }, document,
      event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
      closeSettings: () => calls.push('close'), call: action => calls.push(action),
    })
    assert.deepEqual(calls, repeat ? [] : [hidden ? 'settings.open' : 'close'])
  }
})

test('panel navigation includes the side-by-side tool in visual order and stops at the edges', () => {
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
    assert.equal(navigate(panels, 'agent', key).selected, key === 'Ctrl+H' ? 'agent' : 'tool')
    assert.equal(navigate(panels, 'tool', key).selected, key === 'Ctrl+L' ? 'tool' : 'agent')
    assert.equal(navigate(panels, 'agent', key, true).selected, undefined)
  }
  const twoAgents = [...panels, panel('agent-2', 'center')]
  assert.equal(navigate(twoAgents, 'agent', 'Ctrl+H').selected, 'agent')
  assert.equal(navigate(twoAgents, 'tool', 'Ctrl+L').selected, 'tool')
  assert.equal(navigate(twoAgents, 'agent', 'Ctrl+L').selected, 'agent-2')
  assert.equal(navigate(twoAgents, 'agent-2', 'Ctrl+L').selected, 'tool')
  assert.equal(navigate(twoAgents, 'tool', 'Ctrl+H').selected, 'agent-2')
  const overlay = [agent, panel('agent-2', 'center'), panel('tool', 'right', true, true)]
  const result = navigate(overlay, 'tool', 'Ctrl+L')
  assert.equal(result.selected, 'agent-2')
  assert.equal(result.state.hidden.has('tool'), true)
  assert.equal(navigate([agent], 'agent', 'Ctrl+L').selected, undefined)
})

test('close project asks for confirmation, uses saved binding and ignores repeat', () => {
  const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = workspace.slice(workspace.indexOf("    if (binding && binding === toolKeys['close-project'])"), workspace.indexOf("    if (binding && binding === toolKeys['close-panel'])"))
  for (const binding of ['Ctrl+Shift+Q', 'Alt+Q']) {
    for (const repeat of [false, true]) {
      const calls = []
      vm.runInNewContext('(function () {' + handler + '})()', {
        binding, toolKeys: { 'close-project': binding },
        event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
        call: action => calls.push(action),
        activeGrid: () => ({ dataset: { projectLabel: 'Libro' } }),
      })
      assert.deepEqual(calls, repeat ? [] : ['project.close.check'])
    }
  }
})

test('base and worktree rows are numbered without requiring agents', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const render = source.slice(source.indexOf('  function renderThreadShortcuts()'), source.indexOf('  let notificationAudio;'))
  const rows = Array.from({ length: 11 }, (_, i) => ({
    dataset: { agentId: String(i) }, attributes: {}, badge: null,
    querySelector() { return this.badge },
    append(badge) { this.badge = badge; badge.remove = () => { this.badge = null } },
    setAttribute(key, value) { this.attributes[key] = value },
    removeAttribute(key) { delete this.attributes[key] },
  }))
  const context = vm.createContext({
    document: { querySelectorAll: selector => {
      assert.equal(selector, '.ws-project-row:not([data-kind=project])')
      return rows
    } },
    node: () => ({ setAttribute() {} }),
  })
  vm.runInContext(render + '\nrenderThreadShortcuts();', context)
  assert.deepEqual(rows.map(row => row.dataset.projectShortcut), ['1', '2', '3', '4', '5', '6', '7', '8', '9', '', ''])
  assert.equal(rows[0].badge.textContent, '1')
  assert.equal(rows[0].attributes['aria-keyshortcuts'], 'Control+1')
  assert.equal(rows[9].badge, null)
})

test('Ctrl+1–9 selects the numbered thread once', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('    if (event.ctrlKey &&'), source.indexOf("    if (binding && (binding === toolKeys['panel-size-down']"))
  for (const key of ['1', '9']) for (const repeat of [false, true]) {
    let clicked = 0
    vm.runInNewContext('(function () {' + handler + '})()', {
      event: { key, ctrlKey: true, repeat, preventDefault() {}, stopImmediatePropagation() {} },
      renderThreadShortcuts() {},
      document: { querySelector(selector) {
        assert.equal(selector, '[data-project-shortcut="' + key + '"]')
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
  const agent = { dataset: { appId: 'agent', dock: 'center' }, parentElement: grid, querySelector: () => null }
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

for (const thread of [false, true]) test('new browser creates separate blank panels with thread=' + thread, () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const launch = source.slice(source.indexOf('  function openPlugin('), source.indexOf('  function select('))
  const browser = source.slice(source.indexOf('  function newBrowser('), source.indexOf('  let toolKeys'))
  const calls = []
  vm.runInNewContext(launch + browser + ';newBrowser();newBrowser();', {
    isThread: () => thread,
    window: { __libroPlugins: [{ id: 'browser', type: 'url', name: 'Browser', dock: 'right' }] },
    call: (action, data) => calls.push([action, JSON.parse(JSON.stringify(data))]),
  })
  assert.equal(calls.length, 2)
  for (const [action, data] of calls) {
    assert.equal(action, 'app.start')
    assert.equal(data.plugin, 'browser')
    assert.equal(data.url, '')
    assert.equal(data.dock, 'right')
  }
})

for (const thread of [false, true]) for (const occupied of [false, true]) test(`agent launch routing: thread=${thread}, occupied=${occupied}`, () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const launch = source.slice(source.indexOf('  function openPlugin('), source.indexOf('  function select('))
  const calls = []
  vm.runInNewContext(launch + ';openPlugin("codex", "center")', {
    window: {
      __libroActiveProject: 'project',
      __libroPlugins: [{ id: 'codex', type: 'terminal', name: 'Codex', dock: 'center' }],
    },
    activeGrid: () => ({ dataset: { thread: 'false' } }),
    isThread: () => thread,
    frames: () => occupied ? [{ dataset: { dock: 'center' } }] : [],
    call: (action, data) => calls.push([action, JSON.parse(JSON.stringify(data))]),
  })
  assert.equal(calls.length, 1)
  assert.equal(calls[0][0], thread && occupied ? 'thread.create' : 'app.start')
  if (thread && occupied) assert.equal(calls[0][1].agent, 'codex')
  else {
    assert.equal(calls[0][1].plugin, 'codex')
    assert.equal(calls[0][1].dock, 'center')
  }
})

test('browser navigation wraps, includes hidden browsers, and skips other tools', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function navigateBrowser('), source.indexOf('  let toolKeys'))
  const panels = [
    { appId: 'agent', appType: 'terminal' },
    { appId: 'one', appType: 'url', plugin: 'browser', dockVisible: 'false' },
    { appId: 'files', appType: 'url', plugin: 'files' },
    { appId: 'git', appType: 'terminal' },
    { appId: 'two', appType: 'url', plugin: 'browser' },
    { appId: 'legacy', appType: 'url' },
  ].map(dataset => ({ dataset }))
  for (const [selected, right, delta, expected] of [
    ['one', 'one', 1, 'two'], ['one', 'one', -1, 'legacy'],
    ['legacy', 'legacy', 1, 'one'], ['two', 'two', -1, 'one'],
    ['agent', 'two', 1, 'legacy'], ['git', 'git', 1, 'one'], ['git', 'git', -1, 'legacy'],
  ]) {
    const calls = []
    vm.runInNewContext(handler + ';navigateBrowser(delta)', {
      delta, activeGrid: () => ({}), frames: () => panels, dockState: () => ({ right }),
      window: { __libroSelectedApp: selected }, select: id => calls.push(id),
    })
    assert.deepEqual(calls, [expected])
    assert.equal(panels.length, 6)
  }
  for (const available of [[], [panels[1]]]) {
    const calls = []
    vm.runInNewContext(handler + ';navigateBrowser(1)', {
      activeGrid: () => ({}), frames: () => available, dockState: () => ({}),
      window: { __libroSelectedApp: 'one' }, select: id => calls.push(id),
    })
    assert.deepEqual(calls, available.length ? ['one'] : [])
  }
})

test('Ctrl+B toggles the current browser when multiple browsers exist', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function restoreAgentFocus('), source.indexOf('  function newBrowser'))
  const calls = []
  const state = { right: 'two', hidden: new Set() }
  vm.runInNewContext(handler + ";tool('browser')", {
    activeGrid: () => ({}), dockState: () => state,
    frames: () => ['one', 'two'].map(appId => ({ dataset: { appId, plugin: 'browser', dock: 'right', dockVisible: String(appId === 'two') } })),
    window: { __libroSelectedApp: 'two' }, select: id => calls.push(id), refresh() {},
  })
  assert.equal(state.hidden.has('two'), true)
  assert.equal(state.hidden.has('one'), false)
  assert.deepEqual(calls, [])
})

test('browser shortcuts accept brackets, custom bindings, and ignore repeat', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const normalize = source.slice(source.indexOf('  function shortcut('), source.indexOf('  function zoom('))
  const handler = source.slice(source.indexOf("    if (binding && binding === toolKeys['new-browser'])"), source.indexOf("    if (binding && binding === toolKeys['new-agent'])"))
  for (const [binding, expected] of [['Ctrl+Shift+B', 'new'], ['Ctrl+[', -1], ['Ctrl+]', 1], ['Alt+B', 'new']]) {
    for (const repeat of [false, true]) {
      const input = inputFor(binding)
      const calls = []
      vm.runInNewContext(normalize + '\nconst binding = shortcut(event); (function(){' + handler + '})()', {
        toolKeys: { 'new-browser': binding === 'Alt+B' ? binding : 'Ctrl+Shift+B', 'previous-browser': 'Ctrl+[', 'next-browser': 'Ctrl+]' },
        event: { key: input.key, ctrlKey: input.control, shiftKey: input.shift, altKey: input.alt, repeat, preventDefault() {}, stopImmediatePropagation() {} },
        newBrowser: () => calls.push('new'), navigateBrowser: delta => calls.push(delta),
      })
      assert.deepEqual(calls, repeat ? [] : [expected])
    }
  }
})

test('browser cycling runs at capture phase from terminals and other focused panels', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const navigate = source.slice(source.indexOf('  function navigateBrowser('), source.indexOf('  let toolKeys'))
  const normalize = source.slice(source.indexOf('  function shortcut('), source.indexOf('  function zoom('))
  const start = source.indexOf("  window.addEventListener('keydown', event => {\n    const input")
  const end = source.indexOf("  window.addEventListener('keydown', event => {\n    if (!event.ctrlKey", start)
  for (const focused of ['agent', 'terminal', 'git', 'files']) {
    for (const [key, expected] of [['[', 'one'], [']', 'three']]) {
      let keydown
      let prevented = false
      let stopped = false
      const calls = []
      const state = { browser: 'two', right: focused }
      vm.runInNewContext(navigate + normalize + source.slice(start, end), {
        window: {
          __libroSelectedApp: focused,
          addEventListener(type, handler, capture) {
            assert.equal(type, 'keydown')
            assert.equal(capture, true)
            keydown = handler
          },
        },
        document: { getElementById: () => ({ hidden: true }), querySelector: () => null },
        toolKeys: { 'previous-browser': 'Ctrl+[', 'next-browser': 'Ctrl+]' },
        activeGrid: () => ({}), dockState: () => state,
        frames: () => ['one', 'two', 'three'].map(appId => ({ dataset: { appId, appType: 'url', plugin: 'browser' } })),
        select: id => calls.push(id),
      })
      keydown({ key, ctrlKey: true, target: { closest: () => null },
        preventDefault() { prevented = true }, stopImmediatePropagation() { stopped = true } })
      assert.deepEqual(calls, [expected], focused + ': Ctrl+' + key)
      assert.equal(prevented, true)
      assert.equal(stopped, true)
    }
  }
})

test('double Ctrl+A hides all tools and the bottom terminal without closing them', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  let lastCtrlA'), source.indexOf("  window.addEventListener('keydown', event => {"))
  const detection = source.slice(source.indexOf("    if (event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'a')"), source.indexOf("    const number = /^[1-9]$/"))
  const state = {hidden:new Set(), bottom:true}
  let now = 100, focused = 0
  const context = vm.createContext({
    performance:{now:() => now}, activeGrid:() => ({}), dockState:() => state,
    frames:() => ['center', 'right', 'right', 'bottom'].map((dock, i) => ({dataset:{dock, appId:String(i)}})),
    restoreAgentFocus:() => focused++, maximized:'tool',
    event:{ctrlKey:true, key:'a', preventDefault(){}, stopImmediatePropagation(){}},
  })
  vm.runInContext(handler + '\nfunction press(){' + detection + '}', context)
  vm.runInContext('press()', context)
  assert.equal(focused, 0)
  now = 300
  vm.runInContext('press()', context)
  assert.equal(focused, 1)
  assert.deepEqual([...state.hidden], ['1', '2'])
  assert.equal(state.bottom, false)
  assert.equal(context.maximized, '')
  now = 1000
  vm.runInContext('press()', context)
  now = 1500
  vm.runInContext('press()', context)
  assert.equal(focused, 1)
})

test('address popup reclaims native focus only for the workspace sender', () => {
  const source = main.slice(main.indexOf("ipcMain.handle('libro-focus-workspace'"), main.indexOf('async function quitApp()'))
  let handler, focused = 0, destroyed = false
  const host = { focus() { focused++ } }
  const context = {
    ipcMain: { handle(channel, fn) { assert.equal(channel, 'libro-focus-workspace'); handler = fn } },
    mainWindow: { isDestroyed: () => destroyed, webContents: host },
  }
  vm.runInNewContext(source, context)
  handler({ sender: {} })
  assert.equal(focused, 0)
  handler({ sender: host })
  assert.equal(focused, 1)
  destroyed = true
  handler({ sender: host })
  assert.equal(focused, 1)
})


test('new thread shortcut uses the current project and ignores repeats', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const create = source.slice(source.indexOf('  function newThread('), source.indexOf('  function threadArchived()'))
  const handler = source.slice(source.indexOf("    if (binding && binding === toolKeys['new-thread'])"), source.indexOf("    if (binding && binding === toolKeys['new-agent'])"))
  for (const project of ['', 'project', 'thread:project']) for (const binding of ['Ctrl+N', 'Alt+N']) for (const repeat of [false, true]) {
    const calls = []
    vm.runInNewContext(create + '(function(){' + handler + '})()', {
      window: { __libroActiveProject: project },
      binding, toolKeys: { 'new-thread': binding },
      event: { repeat, preventDefault() {}, stopImmediatePropagation() {} },
      call(action, data) { calls.push([action, data.project]) },
    })
    assert.deepEqual(calls, repeat ? [] : [['thread.create', project]])
  }
})

test('thread title updates use the terminal session and do not rename from tools', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/components.go'), 'utf8')
  const handler = source.slice(source.indexOf('term.onTitleChange(function(title)'), source.indexOf('term.onData(function(data)', source.indexOf('term.onTitleChange(function(title)')))
  for (const dock of ['center', 'right', 'bottom']) {
    let onTitle
    const calls = []
    const frame = { dataset: { dock }, closest: () => ({ dataset: { workspaceProject: 'thread:test' } }) }
    const ws = { call(action, data) { calls.push([action, data.sid, data.id, data.name]) } }
    vm.runInNewContext(handler, {
      term: { onTitleChange(fn) { onTitle = fn } },
      sid: 'session-7', appID: 'agent', window: { __ws: ws }, __ws: ws,
      document: { getElementById: () => frame },
    })
    onTitle('Fix login')
    assert.deepEqual(calls, dock === 'center' ? [['thread.rename', 'session-7', 'thread:test', 'Fix login']] : [])
  }
})


test('thread titles ignore harness placeholders and extract task names', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/components.go'), 'utf8')
  const start = source.indexOf('term.onTitleChange(function(title)')
  const handler = source.slice(start, source.indexOf('term.onData(function(data)', start))
  for (const project of ['thread:test', 'project']) {
    let onTitle
    const calls = []
    const frame = { dataset: { dock: 'center', taskTitle: 'Existing task' }, closest: () => ({ dataset: { workspaceProject: project } }) }
    const ws = { call(action, data) { calls.push(data.name) } }
    vm.runInNewContext(handler, {
      term: { onTitleChange(fn) { onTitle = fn } }, sid: 'session', appID: 'agent',
      window: { __ws: ws }, __ws: ws, document: { getElementById: () => frame },
    })
    for (const title of ['π - capo', 'Ready', 'Ready | renaming... ⠋', 'Ready | ⠙', 'Working | 01a0c304-7225-78c3-b807-123456789abc', 'Ready | 01a0c304-7225-78c3-b807-123456789abc ⠼']) {
      onTitle(title)
      assert.equal(frame.dataset.taskTitle, 'Existing task')
    }
    assert.deepEqual(calls, [])
    onTitle('π - Fix login - capo')
    assert.equal(frame.dataset.taskTitle, 'Fix login')
    onTitle('Thinking | 01a0c304-7225-78c3-b807-123456789abc ⠋ | Fix sidebar ⠹')
    assert.equal(frame.dataset.taskTitle, 'Fix sidebar')
    assert.deepEqual(calls, project.startsWith('thread:') ? ['Fix login', 'Fix sidebar'] : [])
  }
})


test('standalone thread numbers continue after project agent threads and skip archived threads', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const render = source.slice(source.indexOf('  function renderThreadShortcuts()'), source.indexOf('  let notificationAudio;'))
  const rows = ['base', 'worktree', ...Array(10).fill('thread')].map((kind, i) => ({
    dataset: { kind, projectKey: String(i) }, parentElement: { dataset: { archived: String(i === 3) } }, attributes: {}, badge: null,
    querySelector() { return this.badge },
    append(badge) { this.badge = badge; badge.remove = () => { this.badge = null } },
    setAttribute(key, value) { this.attributes[key] = value },
    removeAttribute(key) { delete this.attributes[key] },
  }))
  const context = vm.createContext({
    document: { querySelectorAll: selector => {
      assert.equal(selector, '.ws-project-row:not([data-kind=project])')
      return rows
    } },
    node: () => ({ setAttribute() {} }),
  })
  vm.runInContext(render + ';renderThreadShortcuts()', context)
  assert.equal(rows[0].dataset.projectShortcut, '1')
  assert.equal(rows[1].dataset.projectShortcut, '2')
  assert.deepEqual(rows.slice(2).map(row => row.dataset.projectShortcut), ['3', '', '4', '5', '6', '7', '8', '9', '', ''])
  assert.equal(rows[2].badge.textContent, '3')
  assert.equal(rows[2].attributes['aria-keyshortcuts'], 'Control+3')
  rows[2].parentElement.dataset.archived = 'true'
  vm.runInContext('renderThreadShortcuts()', context)
  assert.equal(rows[2].badge, null)
  assert.equal(rows[4].dataset.projectShortcut, '3')
})

test('Ctrl+digits selects the shared project-agent and standalone-thread range', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('    const number = /^[1-9]$/'), source.indexOf("    if (binding && (binding === toolKeys['panel-size-down']"))
  for (const [key, code, number] of [['1', 'Digit1', '1'], ['!', 'Digit1', '1'], ['(', 'Digit9', '9']]) {
    for (const repeat of [false, true]) {
      let clicked = 0
      vm.runInNewContext('(function(){' + handler + '})()', {
        event: { key, code, ctrlKey: true, shiftKey: false, repeat, preventDefault() {}, stopImmediatePropagation() {} },
        renderThreadShortcuts() {},
        document: { querySelector(selector) {
          assert.equal(selector, '[data-project-shortcut="' + number + '"]')
          return { click() { clicked++ } }
        } },
      })
      assert.equal(clicked, repeat ? 0 : 1)
    }
    assert.equal(vm.runInNewContext(matching + ';isWorkspaceShortcut(input)', { input: { key, code, control: true, shift: false } }), true)
    assert.equal(vm.runInNewContext(matching + ';isWorkspaceShortcut(input)', { input: { key, code, control: true, shift: true } }), false)
  }
})

test('shared panel focus moves from an agent to Files and preserves file input focus', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/components.go'), 'utf8')
  const handler = source.slice(source.indexOf('window.__libroFocusApp = function(idx)'), source.indexOf('window.__libroFocusAppByID = function(appID)'))
  const agent = {}, filter = {}, pending = []
  let focused = 0
  const tree = { focus() { focused++; document.activeElement = tree } }
  const container = {
    getAttribute: () => 'files',
    contains: el => el === tree || el === filter,
    querySelector: selector => selector === '.ws-file-tree' ? tree : null,
  }
  const strip = { closest: () => ({ style: {}, getAttribute: () => null }) }
  const document = {
    activeElement: agent,
    querySelector: () => null,
    querySelectorAll: selector => selector === '[id^="app-strip-"]' ? [strip] : [],
  }
  const window = { __libroSelectedApp: 'files', __libroSortedApps: () => [container], focus() {} }
  vm.runInNewContext(handler, { window, document, setTimeout: fn => pending.push(fn) })
  window.__libroFocusApp(0)
  assert.equal(document.activeElement, tree)
  assert.equal(focused, 1)
  document.activeElement = filter
  pending.splice(0).forEach(fn => fn())
  assert.equal(document.activeElement, filter)
  assert.equal(focused, 1)
  document.activeElement = agent
  window.__libroSelectedApp = 'agent'
  window.__libroFocusApp(0)
  pending.splice(0).forEach(fn => fn())
  assert.equal(document.activeElement, agent)
  assert.equal(focused, 1)
})

test('thread list keeps all open threads and only the newest ten archived threads', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function renderThreads()'), source.indexOf('  function renderProjects()'))
  const node = () => ({ dataset: {}, children: [], classList: { add() {} }, setAttribute() {}, append(...items) { this.children.push(...items) }, replaceChildren() { this.children = [] } })
  const list = node(), calls = []
  const threads = Array.from({ length: 25 }, (_, id) => ({ id: String(id), name: 'Thread ' + id, archived: id % 2 === 0 }))
  vm.runInNewContext(handler + ';renderThreads()', {
    window: { __libroThreads: threads }, document: { getElementById: () => list, querySelectorAll: () => [] },
    node, button: node, closeSettings() {}, innerWidth: 1000, call(action, data) { calls.push([action, data.name]) },
  })
  const rows = list.children.map(item => item.children[0])
  assert.deepEqual(rows.map(row => row.dataset.projectKey), [...threads.filter(t => !t.archived).reverse(), ...threads.filter(t => t.archived).reverse().slice(0, 10)].map(t => t.id))
  rows.at(-1).onclick()
  assert.deepEqual(calls, [['project.switch', '6']])
  assert.equal(threads.length, 25, 'older archives remain stored')
})

test('closing panels restores focus without revealing hidden terminals', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function restorePanelFocus('), source.indexOf('  function restoreAgentFocus('))
  for (const bottomVisible of [false, true]) for (const hasAgent of [false, true]) for (const selected of ['closed-panel', 'agent']) {
    const panels = [
      { dataset: { appId: 'shell', dockVisible: String(bottomVisible) } },
      ...(hasAgent ? [{ dataset: { appId: 'agent', dockVisible: 'true' } }] : []),
      { dataset: { appId: 'hidden-tool', dockVisible: 'false' } },
    ]
    const window = { __libroSelectedApp: selected }
    const focused = []
    vm.runInNewContext(handler + ';restorePanelFocus()', {
      window, activeGrid: () => ({}), refresh() {}, frames: () => panels,
      dockState: () => ({ agent: 'agent' }),
      select(id) { window.__libroSelectedApp = id; focused.push(id) },
    })
    const expected = hasAgent ? 'agent' : bottomVisible ? 'shell' : ''
    assert.equal(window.__libroSelectedApp, expected)
    assert.deepEqual(focused, expected ? [expected] : [])
  }
})

test('project threads hide archived agents and stay separate from standalone threads', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function renderThreads()'), source.indexOf('  function renderProjects()'))
  const node = () => ({ dataset: {}, children: [], classList: { add() {} }, setAttribute() {}, append(...items) { this.children.push(...items) }, replaceChildren() { this.children = [] } })
  const standalone = node(), project = node(), other = node()
  project.dataset.projectThreads = 'project'
  other.dataset.projectThreads = 'other'
  const calls = []
  const context = vm.createContext({
    window: { __libroThreads: [
      { id: 'standalone', name: 'Standalone' },
      { id: 'one', name: 'One', project: 'project' },
      { id: 'two', name: 'Two', project: 'project', archived: true },
      { id: 'other', name: 'Other', project: 'other' },
    ] },
    document: { getElementById: () => standalone, querySelectorAll: () => [project, other] },
    node, button: (label, icon, onclick) => Object.assign(node(), { onclick }), closeSettings() {}, innerWidth: 1000,
    call: (action, data) => calls.push([action, JSON.parse(JSON.stringify(data))]),
  })
  vm.runInContext(handler + ';renderThreads()', context)
  const ids = list => list.children.map(item => item.children[0].dataset.projectKey)
  assert.deepEqual(ids(standalone), ['standalone'])
  assert.deepEqual(ids(project), ['one'])
  assert.deepEqual(ids(other), ['other'])
  context.window.__libroThreads.push({ id: 'new', name: 'New', project: 'project' })
  context.window.__libroActiveProject = 'new'
  vm.runInContext('renderThreads()', context)
  assert.deepEqual(ids(project), ['one', 'new'])
  project.children[0].children[0].onclick()
  assert.deepEqual(calls, [['project.switch', { name: 'one', focusAgent: true }]])
  const grid = { dataset: { workspaceProject: 'one' } }
  context.document.querySelectorAll = () => [grid]
  context.frames = () => [{ dataset: { dock: 'center', appId: 'agent-one' } }, { dataset: { dock: 'right', appId: 'browser' } }]
  const hidden = new Set()
  context.dockState = () => ({ hidden })
  context.toolOverlapsFrame = () => true
  context.select = id => calls.push(['select', id])
  project.children[0].children[0].onclick()
  assert.deepEqual(calls.at(-1), ['project.switch', { name: 'one', focusAgent: true }])
  assert.equal(hidden.has('browser'), false)
  context.window.__libroActiveProject = 'one'
  project.children[0].children[0].onclick()
  assert.deepEqual(calls.at(-1), ['select', 'agent-one'])
  assert.equal(hidden.has('browser'), true)
})

test('new thread uses the active project context, while standalone creation remains explicit', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function newThread('), source.indexOf('  function threadArchived()'))
  const calls = []
  vm.runInNewContext(handler + ";newThread();newThread('');newThread('other')", {
    window: { __libroActiveProject: 'thread:project' },
    call: (action, data) => calls.push([action, data.project]),
  })
  assert.deepEqual(calls, [['thread.create', 'thread:project'], ['thread.create', ''], ['thread.create', 'other']])
})

test('workspace dividers resize, clamp, persist, and clean up cancelled drags', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const handler = source.slice(source.indexOf('  function resizeHandle('), source.indexOf('  function layoutDocks('))
  for (const kind of ['sidebar', 'terminal']) {
    const prefs = {}
    let handle, shield, saved = 0
    const host = {
      offsetWidth: 216, offsetHeight: 200, parentElement: {clientHeight: 800},
      querySelector: () => handle,
      append: value => { handle = value },
    }
    const context = {
      host, kind, prefs, innerWidth: 1200, schedule() {}, save() { saved++ },
      root: {style: {setProperty() {}}},
      document: {body: {append(value) { shield = value }}},
      node: () => ({style: {}, setAttribute() {}, setPointerCapture() {}, remove() { this.removed = true }}),
    }
    vm.runInNewContext(handler + ';resizeHandle(host, kind)', context)
    handle.onpointerdown({button: 0, pointerId: 1, clientX: 216, clientY: 600, preventDefault() {}, stopPropagation() {}})
    handle.onpointermove({clientX: 300, clientY: 500})
    assert.equal(prefs[kind === 'sidebar' ? 'sidebarWidth' : 'terminalHeight'], 300)
    handle.onpointermove({clientX: 2000, clientY: -2000})
    assert.equal(prefs[kind === 'sidebar' ? 'sidebarWidth' : 'terminalHeight'], kind === 'sidebar' ? 480 : 680)
    handle.onpointercancel()
    assert.equal(shield.removed, true)
    assert.equal(handle.onpointermove, null)
    assert.equal(saved, 1)
    handle.onkeydown({key: kind === 'sidebar' ? 'ArrowRight' : 'ArrowUp', preventDefault() {}})
    assert.equal(prefs[kind === 'sidebar' ? 'sidebarWidth' : 'terminalHeight'], kind === 'sidebar' ? 232 : 216)
    assert.equal(saved, 2)
  }
})

test('project groups contain the original branch first and number every worktree', () => {
  const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
  const render = source.slice(source.indexOf('  function renderProjects()'), source.indexOf('  function toolOverlapsFrame('))
  const shortcuts = source.slice(source.indexOf('  function renderThreadShortcuts()'), source.indexOf('  let notificationAudio;'))
  function node(tag, classes = '', text = '') {
    const el = {
      tag, classes, textContent: text, dataset: {}, attributes: {}, children: [],
      classList: { add() {} },
      setAttribute(key, value) { this.attributes[key] = value },
      removeAttribute(key) { delete this.attributes[key] },
      append(...children) { children.forEach(child => { child.parentElement = this; this.children.push(child) }); this.lastElementChild = this.children.at(-1) },
      replaceChildren() { this.children = [] },
      querySelector(selector) { return this.children.find(child => child.classes.split(' ').includes(selector.slice(1))) || null },
    }
    return el
  }
  const list = node('div')
  const calls = []
  const flatten = el => [el, ...el.children.flatMap(flatten)]
  const context = {
    window: { __libroProjects: [
      { kind:'worktree', name:'mail', branch:'feature', path:'/mail-feature', isActive:true },
      { kind:'project', name:'mail', displayName:'mail', path:'/mail', isGit:true, currentBranch:'main', baseOpened:true },
      { kind:'project', name:'notes', path:'/notes', isGit:false },
    ] },
    document: {
      getElementById: () => list,
      querySelectorAll: () => flatten(list).filter(el => el.classes.split(' ').includes('ws-project-row') && el.dataset.kind !== 'project'),
    },
    node, button: (label, icon, onclick) => Object.assign(node('button', '', label), { onclick }),
    call: (action, data) => calls.push([action, data]), closeSettings() {}, innerWidth:1000,
    newThread() {}, projectSettings() {},
  }
  vm.runInNewContext(render + shortcuts + ';renderProjects();renderThreadShortcuts();', context)
  assert.equal(list.children.length, 2)
  const group = list.children[0]
  assert.equal(group.classes, 'ws-project-group')
  const rows = flatten(group).filter(el => el.classes.split(' ').includes('ws-thread-row'))
  assert.deepEqual(rows.map(row => row.children[1].textContent), ['main', 'feature'])
  assert.deepEqual(rows.map(row => row.dataset.projectShortcut), ['1', '2'])
  rows[0].onclick()
  rows[1].onclick()
  assert.equal(calls[0][0], 'project.switch')
  assert.equal(calls[0][1].name, 'mail')
  assert.equal(calls[1][0], 'worktree.switch')
  assert.equal(calls[1][1].branch, 'feature')
  const nonGit = flatten(list.children[1])
  assert.equal(nonGit.find(el => el.textContent === 'New project thread').disabled, true)
  assert.equal(nonGit.find(el => el.dataset.kind === 'base'), undefined)
  const projectRow = nonGit.find(el => el.dataset.kind === 'project' && el.tag === 'button')
  assert.equal(projectRow.dataset.projectShortcut, undefined)
  projectRow.onclick()
  assert.equal(calls.at(-1)[0], 'project.switch')
  assert.equal(calls.at(-1)[1].name, 'notes')
  context.window.__libroProjects[2].baseOpened = true
  vm.runInNewContext(render + shortcuts + ';renderProjects();renderThreadShortcuts();', context)
  assert.equal(flatten(list.children[1]).find(el => el.dataset.kind === 'base').dataset.projectShortcut, '3')
  assert.equal(flatten(list).some(el => el.classes === 'ws-project-agent'), false)
})
