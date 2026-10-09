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
    form, prefs: {}, toolKeys: {}, fillVoice() {},
    window: { __libroPlugins: [
      { id: 'custom-tool-old', dock: 'right', type: 'terminal', removed: true },
      { id: 'custom-tool-site', dock: 'right', type: 'url', removed: true },
      { id: 'custom-old', dock: 'center', type: 'terminal', removed: true },
    ] },
    document: { getElementById: () => element, querySelector: () => element, querySelectorAll: () => [] },
    fillThreadAgents() {}, fillToolKeys() {}, updateToolHints() {}, themePreference() {}, addAgentRow() {}, fillAgentEnvironment() {}, fillAutolaunchAgents() {}, fillEditorTools() {},
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

const saveAutoUpdate = workspace.slice(workspace.indexOf('  function saveAgentAutoUpdate('), workspace.indexOf('  let savedThreadAgent ='))
const saveAll = workspace.slice(workspace.indexOf('  let settingsSaveSteps ='), workspace.indexOf('  let removedAgents ='))
const close = workspace.slice(workspace.indexOf('  function closeSettings('), workspace.indexOf('  function saveSettings('))
const voice = workspace.slice(workspace.indexOf('  function saveVoice('), workspace.indexOf('  function showSettings('))

function settingsHarness(valid = true) {
  const elements = new Map()
  const content = { inert: false }
  const get = id => {
    if (!elements.has(id)) elements.set(id, { value: id, disabled: false, textContent: '', hidden: false, dataset: {} })
    return elements.get(id)
  }
  get('agent-auto-update').value = 'off'
  get('workspace-settings').querySelectorAll = () => [{ reportValidity: () => valid }]
  get('workspace-settings').querySelector = () => content
  const calls = []
  const toasts = []
  let stored
  const context = vm.createContext({
    prefs: { notificationSound: false },
    localStorage: { setItem(key, value) { stored = JSON.parse(value) } },
    window: { __libroShowToast: (...args) => toasts.push(args) },
    document: { getElementById: get, querySelector: () => content, querySelectorAll: () => [] },
    settingsFocus: null,
    saveAgentCommand: () => calls.push('agents'),
    saveTools: () => calls.push('tools'),
    saveAgentEnvironment: () => calls.push('environment'),
    call: (action, data) => { assert.ok(['settings.voice', 'settings.agent-updates'].includes(action)); calls.push({...data}) },
    saveThreadAgent: value => calls.push(value),
    savePageTools: () => calls.push('page-tools'),
    saveSettings: (value, tool) => calls.push(tool ? 'tool-width' : 'width'),
    saveTheme: () => { calls.push('theme'); return true },
    saveNotificationSound: () => { calls.push('sound'); return true },
  })
  vm.runInContext(saveAutoUpdate + saveAll + close + voice, context)
  return { get, content, calls, toasts, context, stored: () => stored, run: code => vm.runInContext(code, context) }
}

test('one Save waits for every section before showing a toast and keeps settings open', () => {
  const h = settingsHarness()
  h.run('saveAllSettings(); saveAllSettings(); closeSettings()')
  assert.deepEqual(h.calls, ['agents'])
  assert.equal(h.get('workspace-settings').hidden, false)
  assert.equal(h.content.inert, true)
  assert.deepEqual(h.toasts, [])
  for (let i = 0; i < 8; i++) h.run('settingsSaveFinished(true)')
  assert.deepEqual(h.calls, ['agents', 'tools', 'environment', {key:'openrouter-key', clearKey:false}, {enabled:false}, 'default-thread-agent', 'page-tools', 'width', 'theme', 'sound'])
  assert.equal(h.get('workspace-settings').hidden, false)
  assert.deepEqual(h.toasts, [['Settings saved', '', 'success']])
  assert.equal(h.get('settings-save').disabled, false)
  assert.equal(h.content.inert, false)
})

test('a failed save keeps settings open and allows retry', () => {
  const h = settingsHarness()
  h.run('saveAllSettings(); settingsSaveFinished(false, "Invalid command")')
  assert.deepEqual(h.calls, ['agents'])
  assert.equal(h.get('workspace-settings').hidden, false)
  assert.equal(h.get('settings-save-status').textContent, 'Invalid command')
  assert.deepEqual(h.toasts, [])
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

test('OpenRouter key is sent once, never shown, and can be removed', () => {
  const h = settingsHarness()
  h.get('openrouter-key').value = 'sk-or-new'
  h.run("saveVoice('sk')")
  assert.deepEqual(h.calls, [{key:'sk-or-new', clearKey:false}])
  h.run("voiceSaved(false, null)")
  assert.equal(h.get('openrouter-key').value, 'sk-or-new')
  assert.match(h.get('voice-status').textContent, /Could not save/)
  h.run("voiceSaved(true, {language:'sk', savedKey:true})")
  assert.equal(h.get('openrouter-key').value, '')
  assert.equal(h.get('openrouter-key').placeholder, 'Saved key')
  h.run("clearOpenRouterKey(); saveVoice('')")
  assert.deepEqual(h.calls[1], {key:'', clearKey:true})
})

test('automatic update preference waits for the backend and leaves local settings alone', () => {
  const h = settingsHarness()
  assert.equal(h.run("saveAgentAutoUpdate('off')"), null)
  assert.deepEqual(h.calls, [{enabled:false}])
  assert.equal(h.stored(), undefined)
  h.run('agentUpdatesSaved(true, false)')
  assert.equal(h.context.window.__libroAgentAutoUpdate, false)
  assert.equal(h.get('agent-auto-update').value, 'off')
  h.context.call = () => { throw Error('disconnected') }
  assert.equal(h.run("saveAgentAutoUpdate('on')"), false)
  assert.equal(h.get('agent-auto-update').value, 'off')
  assert.match(h.get('agent-auto-update-status').textContent, /Could not save/)
})

test('editor choices follow enabled CLI tool rows and keep the selected tool', () => {
  const { JSDOM } = require('jsdom')
  const dom = new JSDOM('<select id="editor-tool"></select><div id="tool-command-rows"></div>', {runScripts:'outside-only'})
  try {
    const w = dom.window
    const rows = w.document.getElementById('tool-command-rows')
    for (const [id, type, checked, removed] of [
      ['nvim', 'terminal', true, false],
      ['custom-tool-editor', 'terminal', true, false],
      ['site', 'url', true, false],
      ['disabled', 'terminal', false, false],
      ['removed', 'terminal', true, true],
    ]) {
      const row = w.document.createElement('div')
      Object.assign(row.dataset, {agentId:id, toolType:type, removed:String(removed)})
      row.innerHTML = '<input data-agent-enabled type="checkbox"><input data-agent-name>'
      row.querySelector('[data-agent-enabled]').checked = checked
      row.querySelector('[data-agent-name]').value = id
      rows.append(row)
    }
    const start = workspace.indexOf('  function fillEditorTools(')
    w.eval(workspace.slice(start, workspace.indexOf('  function saveTools(', start)) + ';window.fillEditorTools=fillEditorTools')
    w.fillEditorTools('custom-tool-editor')
    const select = w.document.getElementById('editor-tool')
    assert.deepEqual(Array.from(select.options, option => option.value), ['', 'nvim', 'custom-tool-editor'])
    assert.equal(select.value, 'custom-tool-editor')
    rows.children[1].querySelector('[data-agent-enabled]').checked = false
    w.fillEditorTools()
    assert.equal(select.value, '')
  } finally { dom.window.close() }
})
