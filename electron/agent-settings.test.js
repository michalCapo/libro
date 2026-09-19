const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const workspace = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
const show = workspace.slice(workspace.indexOf('  function showSettings('), workspace.indexOf('  function closeSettings('))
const save = workspace.slice(workspace.indexOf('  function saveAgentCommand('), workspace.indexOf('  function agentCommandSaved('))

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
    fillThreadAgents() {}, fillToolKeys() {}, updateToolHints() {}, themePreference() {}, addAgentRow() {},
    call(action, data) { assert.equal(action, 'settings.agent-command'); payload = JSON.parse(JSON.stringify(data)) },
  })
  assert.deepEqual(payload.removed, { 'custom-old': true })
  assert.deepEqual(payload.disabled, { 'custom-luna': false, 'custom-old': true })
  assert.equal(payload.custom[0].name, 'luna')
  assert.equal(payload.commands['custom-luna'], fields['[data-agent-command]'].value)
})
