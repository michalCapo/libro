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
    document: { querySelectorAll(selector) { return selector === '.ws-project-row' ? rows : grids }, addEventListener(type, fn) { listeners[type] = fn } },
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
