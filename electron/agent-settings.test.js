const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
const show = workspace.slice(workspace.indexOf('  function showSettings('), workspace.indexOf('  function closeSettings('))
const save = workspace.slice(workspace.indexOf('  function saveAgentCommand('), workspace.indexOf('  function agentCommandSaved('))
const saveEnvironment = workspace.slice(workspace.indexOf('  function saveAgentEnvironment('), workspace.indexOf('  function agentEnvironmentSaved('))

test('saving a new agent excludes removed tools and preserves removed agents', () => {
  const element = { replaceChildren() {}, focus() {} }
  const fields = {
    '[data-agent-command]': { value: `codex -m gpt-5.6-luna -c 'model_reasoning_effort="xhigh"'` },
    '[data-agent-name]': { value: 'luna' },
    '[data-agent-enabled]': { checked: true },
  }
  const row = { dataset: { agentId: 'custom-luna', custom: 'true' }, querySelector: selector => fields[selector] }
  const form = {
    querySelectorAll: () => [row],
    querySelector: selector => selector.includes(':checked') ? null : element,
  }
  let payload
  vm.runInNewContext(show + save + ';showSettings("md");saveAgentCommand(form)', {
    form, prefs: {}, toolKeys: {},
    window: { __libroPlugins: [
      { id: 'custom-tool-old', dock: 'right', type: 'terminal', removed: true },
      { id: 'custom-tool-site', dock: 'right', type: 'url', removed: true },
      { id: 'custom-old', dock: 'center', type: 'terminal', removed: true },
    ] },
    document: { getElementById: () => element, querySelector: () => element, querySelectorAll: () => [] },
    fillThreadAgents() {}, fillToolKeys() {}, updateToolHints() {}, themePreference() {}, addAgentRow() {}, fillAgentEnvironment() {}, fillAutolaunchAgents() {},
    call(action, data) { assert.equal(action, 'settings.agent-command'); payload = JSON.parse(JSON.stringify(data)) },
  })
  assert.deepEqual(payload.removed, { 'custom-old': true })
  assert.deepEqual(payload.disabled, { 'custom-luna': false, 'custom-old': true })
  assert.equal(payload.custom[0].name, 'luna')
  assert.equal(payload.commands['custom-luna'], fields['[data-agent-command]'].value)
})

test('saving agent environment preserves masked values', () => {
  const status = { textContent: '' }
  const row = {
    dataset: { originalName: 'OPENROUTER_API_KEY' },
    querySelector: selector => ({
      '[data-environment-name]': { value: 'OPENROUTER_API_KEY' },
      '[data-environment-value]': { value: '' },
    })[selector],
  }
  const form = {
    querySelectorAll: () => [row],
    querySelector: () => status,
  }
  let payload
  vm.runInNewContext(saveEnvironment + ';saveAgentEnvironment(form)', {
    form,
    call(action, data) { assert.equal(action, 'settings.agent-environment'); payload = JSON.parse(JSON.stringify(data)) },
  })
  assert.deepEqual(payload.entries, [{name: 'OPENROUTER_API_KEY', value: '', originalName: 'OPENROUTER_API_KEY'}])
  assert.equal(status.textContent, 'Saving…')
})

const saveAll = workspace.slice(workspace.indexOf('  let settingsSaveSteps ='), workspace.indexOf('  let removedAgents ='))
const close = workspace.slice(workspace.indexOf('  function closeSettings('), workspace.indexOf('  function saveSettings('))

function settingsHarness(valid = true) {
  const elements = new Map()
  const content = { inert: false }
  const get = id => {
    if (!elements.has(id)) elements.set(id, { value: id, disabled: false, textContent: '', hidden: false })
    return elements.get(id)
  }
  get('workspace-settings').querySelectorAll = () => [{ reportValidity: () => valid }]
  get('workspace-settings').querySelector = () => content
  const calls = []
  const context = vm.createContext({
    document: { getElementById: get, querySelector: () => content, querySelectorAll: () => [] },
    settingsFocus: null,
    saveAgentCommand: () => calls.push('agents'),
    saveTools: () => calls.push('tools'),
    saveAgentEnvironment: () => calls.push('environment'),
    saveThreadAgent: value => calls.push(value),
    savePageTools: () => calls.push('page-tools'),
    saveSettings: (value, tool) => calls.push(tool ? 'tool-width' : 'width'),
    saveTheme: () => { calls.push('theme'); return true },
    saveNotificationSound: () => { calls.push('sound'); return true },
  })
  vm.runInContext(saveAll + close, context)
  return { get, content, calls, run: code => vm.runInContext(code, context) }
}

test('one Save waits for every section before closing and prevents duplicate saves', () => {
  const h = settingsHarness()
  h.run('saveAllSettings(); saveAllSettings(); closeSettings()')
  assert.deepEqual(h.calls, ['agents'])
  assert.equal(h.get('workspace-settings').hidden, false)
  assert.equal(h.content.inert, true)
  for (let i = 0; i < 7; i++) h.run('settingsSaveFinished(true)')
  assert.deepEqual(h.calls, ['agents', 'tools', 'environment', 'default-thread-agent', 'page-tools', 'width', 'tool-width', 'theme', 'sound'])
  assert.equal(h.get('workspace-settings').hidden, true)
  assert.equal(h.get('settings-save').disabled, false)
  assert.equal(h.content.inert, false)
})

test('a failed save keeps settings open and allows retry', () => {
  const h = settingsHarness()
  h.run('saveAllSettings(); settingsSaveFinished(false, "Invalid command")')
  assert.deepEqual(h.calls, ['agents'])
  assert.equal(h.get('workspace-settings').hidden, false)
  assert.equal(h.get('settings-save-status').textContent, 'Invalid command')
  assert.equal(h.get('settings-cancel').disabled, false)
  h.run('saveAllSettings()')
  assert.deepEqual(h.calls, ['agents', 'agents'])
})

test('Cancel closes without saving and invalid fields prevent any save', () => {
  const h = settingsHarness(false)
  h.run('saveAllSettings()')
  assert.deepEqual(h.calls, [])
  h.run('closeSettings()')
  assert.equal(h.get('workspace-settings').hidden, true)
  assert.deepEqual(h.calls, [])
})
