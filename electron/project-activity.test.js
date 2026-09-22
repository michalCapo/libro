const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const { test } = require('node:test')
const workspace = fs.readFileSync(require('node:path').join(__dirname, '../internal/workspace.js'), 'utf8')
const source = workspace.slice(workspace.indexOf('  const acknowledgedAgents ='), workspace.indexOf('  function renderProjectTerminals()'))

test('interaction acknowledges only its project and a new completion restores the icon', () => {
  const listeners = {}
  const rows = ['project', 'project/branch'].map(key => ({
    dataset: { projectKey: key, kind: key.includes('/') ? 'worktree' : 'project' },
    icon: {}, label: '',
    querySelector(selector) { return selector === 'i' ? this.icon : { textContent: key } },
    setAttribute(name, value) { this.label = value },
  }))
  const grids = rows.map((row, i) => ({ dataset: { workspaceProject: row.dataset.projectKey }, frames: [{ dataset: { appId: String(i), dock: 'center' } }] }))
  const context = vm.createContext({
    window: { __libroAgentStatuses: { 0: 'done', 1: 'done' }, addEventListener(type, fn) { listeners[type] = fn } },
    document: { querySelectorAll(selector) { return selector === '.ws-project-agent' ? [] : selector === '.ws-project-row' ? rows : grids }, addEventListener(type, fn) { listeners[type] = fn } },
    frames: grid => grid.frames,
  })
  vm.runInContext(source, context)
  const render = () => listeners['libro-agent-status']()
  render()
  assert.equal(rows[0].icon.textContent, 'check_circle_outline')
  for (const type of ['pointerdown', 'keydown', 'input', 'wheel']) {
    context.window.__libroAgentStatuses[0] = 'working'; render()
    assert.equal(rows[0].icon.textContent, 'sync')
    context.window.__libroAgentStatuses[0] = 'done'; render()
    assert.equal(rows[0].icon.textContent, 'check_circle_outline')
    listeners[type]({ type, target: { closest(selector) { return selector === '.ws-project' ? { querySelector() { return grids[0] } } : null } } })
    render() // Repeated status snapshots must not bring back the checkmark.
    assert.equal(rows[0].icon.textContent, 'folder_open')
    assert.equal(rows[0].label, 'project')
    assert.equal(rows[1].icon.textContent, 'check_circle_outline')
  }
  vm.runInContext("acknowledgeProjectActivity('project/branch')", context)
  assert.equal(rows[1].icon.textContent, 'account_tree')
})

test('thread interaction clears completion until a new turn completes', () => {
  const listeners = {};
  const tabs = ['a', 'b', 'c'].map(id => ({
    dataset: { agentId: id }, icon: {}, badge: null, shortcut: {},
    querySelector(selector) { return selector === 'i' ? this.icon : selector === 'span' ? { textContent: id } : selector === '.ws-project-shortcut' ? this.shortcut : this.badge },
    insertBefore(badge, before) { this.badge = badge; this.badgeBefore = before; badge.remove = () => { this.badge = null } },
    setAttribute(name, value) { this[name] = value },
  }))
  const context = vm.createContext({
    window: { __libroAgentStatuses: { a: 'working', b: 'done' }, addEventListener() {} },
    document: { querySelectorAll(selector) { return selector === '.ws-project-agent' ? tabs : [] }, addEventListener(type, fn) { listeners[type] = fn } },
    node: () => ({}),
  })
  vm.runInContext(source, context)
  vm.runInContext('renderProjectActivity()', context)
  assert.equal(tabs[0].badge.textContent, 'Working')
  assert.equal(tabs[0].badgeBefore, tabs[0].shortcut)
  assert.equal(tabs[1].badge.textContent, 'Done')
  assert.equal(tabs[1].icon.textContent, 'check_circle_outline')
  assert.equal(tabs[1]['aria-label'], 'b: Done')
  assert.equal(tabs[2].badge, null)
  for (const type of ['pointerdown', 'keydown', 'input', 'wheel']) {
    listeners[type]({ type, target: { closest() { return tabs[1] } } })
    vm.runInContext('renderProjectActivity()', context)
    assert.equal(tabs[1].dataset.agentStatus, '')
    assert.equal(tabs[1].badge, null)
    assert.equal(tabs[1].icon.textContent, 'chat_bubble_outline')
    assert.equal(tabs[1]['aria-label'], 'b')
    assert.equal(tabs[0].badge.textContent, 'Working')
  }
  context.window.__libroAgentStatuses.b = 'working'
  vm.runInContext('renderProjectActivity()', context)
  assert.equal(tabs[1].badge.textContent, 'Working')
  assert.equal(tabs[1].icon.textContent, 'sync')
  context.window.__libroAgentStatuses.b = 'done'
  vm.runInContext('renderProjectActivity()', context)
  assert.equal(tabs[1].dataset.agentStatus, 'done')
  assert.equal(tabs[1].badge.textContent, 'Done')
  assert.equal(tabs[1].icon.textContent, 'check_circle_outline')
  delete context.window.__libroAgentStatuses.b
  vm.runInContext('renderProjectActivity()', context)
  assert.equal(tabs[1].badge, null)
  assert.equal(tabs[1].icon.textContent, 'chat_bubble_outline')
})

test('bottom terminal activity belongs to the project, or to a standalone thread', () => {
  const rows = ['project', 'thread:one', 'thread:two', 'thread:solo'].map(projectKey => ({
    dataset: { projectKey }, icon: null,
    querySelector(selector) { return selector === '.ws-project-terminal' ? this.icon : null },
    insertBefore(icon) { this.icon = icon; icon.remove = () => { this.icon = null } },
  }))
  const grids = [
    { dataset: { workspaceProject: 'thread:one', projectScope: 'project' }, running: true },
    { dataset: { workspaceProject: 'thread:solo', projectScope: '' }, running: true },
  ].map(grid => ({ ...grid, querySelector() { return this.running } }))
  const context = vm.createContext({
    window: { __libroThreads: [{ id: 'thread:one', project: 'project' }, { id: 'thread:two', project: 'project' }, { id: 'thread:solo' }] },
    document: {
      querySelectorAll(selector) { return selector === '.ws-project-row' ? rows : grids },
      createElementNS() { return { classList: { add() {} }, setAttribute() {}, append() {} } },
    },
  })
  const start = workspace.indexOf('  function renderProjectTerminals()')
  vm.runInContext(workspace.slice(start, workspace.indexOf("  window.addEventListener('libro-process-status'", start)), context)
  vm.runInContext('renderProjectTerminals()', context)
  assert.ok(rows[0].icon)
  assert.equal(rows[1].icon, null)
  assert.equal(rows[2].icon, null)
  assert.ok(rows[3].icon)
  grids[0].running = false
  vm.runInContext('renderProjectTerminals()', context)
  assert.equal(rows[0].icon, null)
  assert.ok(rows[3].icon)
})
