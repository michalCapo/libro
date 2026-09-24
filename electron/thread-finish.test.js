const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')
const source = fs.readFileSync(path.join(__dirname, '../internal/workspace.js'), 'utf8')
const keyCode = source.slice(source.indexOf('  function dialogKeys('), source.indexOf('  function projectSettings('))
const dialogCode = keyCode + source.slice(source.indexOf('  let finishDialog ='), source.indexOf('  function threadMenu('))

function harness(discard = false, method = 'merge') {
  const calls = []
  const all = []
  const flatten = el => [el, ...el.children.flatMap(flatten)]
  function node(tag, cls = '', text = '') {
    const el = { tag, className:cls, textContent:text, value:'', children:[], events:{}, disabled:false,
      setAttribute() {}, focus() {}, remove() {}, showModal() {}, click() { this.onclick?.() },
      addEventListener(name, fn) { this.events[name] = fn },
      close() { this.events.close?.() },
      append(...children) { this.children.push(...children) },
      replaceChildren(...children) { this.children = children },
      querySelectorAll(selector) { return flatten(this).filter(child => selector.split(',').includes(child.tag)) },
    }
    all.push(el)
    return el
  }
  let sequence = 0
  const context = vm.createContext({
    node, root:node('div'), crypto:{randomUUID:() => String(++sequence)},
    call:(action, data) => calls.push({action, data}),
    window:{__libroActiveProject:'repo/feature', __libroProjects:[{kind:'worktree',name:'repo',branch:'feature'}]},
  })
  vm.runInContext(dialogCode + `;finishThread('repo/feature', ${discard}, '${method}');`, context)
  const field = id => all.find(el => el.id === id)
  const button = label => all.find(el => el.tag === 'button' && el.textContent === label)
  const form = all.find(el => el.tag === 'form')
  const info = {name:'repo/feature',branch:'feature',base:'main',branches:['main'],sourceHead:'source',targetHead:'target',summary:'1 file changed',dirty:'',blocker:''}
  const preview = (override = {}, request = calls.at(-1).data.request) => {
    context.reply = {info:{...info, ...override}}
    context.request = request
    vm.runInContext('finishThreadPreview(request, reply)', context)
  }
  return {calls, field, button, form, preview, context}
}

test('finish preview never mutates Git; submit sends only the reviewed heads and selected method', () => {
  const h = harness()
  assert.deepEqual(h.calls.map(c => c.action), ['thread.finish.preview'])
  const submit = h.button('Merge and remove thread')
  assert.equal(submit.disabled, true)
  h.preview({}, 'stale-request')
  assert.equal(submit.disabled, true)
  h.preview()
  assert.equal(submit.disabled, false)
  h.form.onsubmit({preventDefault(){}})
  h.form.onsubmit({preventDefault(){}})
  assert.deepEqual(h.calls.map(c => c.action), ['thread.finish.preview', 'thread.finish'])
  assert.equal(h.calls[1].data.method, 'merge')
  assert.equal(Object.hasOwn(h.calls[1].data, 'deleteBranch'), false)
  assert.equal(h.calls[1].data.sourceHead, 'source')
  assert.equal(h.calls[1].data.targetHead, 'target')
})

test('dirty previews block local merge and PR submission', () => {
  const h = harness()
  h.preview({dirty:' M file.txt',blocker:'Commit changes first'})
  const submit = h.button('Merge and remove thread')
  assert.equal(submit.disabled, true)
  const pr = harness(false, 'pr')
  pr.preview({dirty:' M file.txt',blocker:'Commit changes first'})
  assert.equal(pr.button('Push branch and create draft PR').disabled, true)
})

test('discard removes the branch without typed confirmation', () => {
  const h = harness(true)
  h.preview({dirty:'?? unsaved.txt'})
  const submit = h.button('Discard and remove thread')
  assert.equal(h.field('finish-thread-confirm'), undefined)
  assert.equal(submit.disabled, false)
  h.form.onsubmit({preventDefault(){}})
  assert.equal(h.calls.at(-1).data.method, 'discard')
  assert.equal(Object.hasOwn(h.calls.at(-1).data, 'deleteBranch'), false)
})

test('failed finish automatically reloads its preview before retry', () => {
  const h = harness()
  h.preview()
  h.form.onsubmit({preventDefault(){}})
  vm.runInContext(`finishThreadResult('repo/feature', {error:'Branch changed'})`, h.context)
  const submit = h.button('Merge and remove thread')
  assert.equal(submit.disabled, true)
  assert.equal(h.button('Refresh changes'), undefined)
  assert.equal(h.calls.at(-1).action, 'thread.finish.preview')
  h.preview()
  assert.equal(submit.disabled, false)
})

test('finish shortcut opens the dialog once and ignores key repeat', () => {
  const start = source.indexOf("    if (binding && binding === toolKeys['finish-thread'])")
  const code = source.slice(start, source.indexOf("    if (binding && binding === toolKeys['new-thread'])", start))
  for (const repeat of [false,true]) {
    let opened = 0
    vm.runInNewContext('(function(){'+code+'})()', {binding:'Alt+M',toolKeys:{'finish-thread':'Alt+M'},event:{repeat,preventDefault(){},stopPropagation(){}},finishThread(){opened++}})
    assert.equal(opened,repeat ? 0 : 1)
  }
})


test('dialog Enter submits and Escape cancels without repeats or disabled actions', () => {
  let handler, submitted = 0, cancelled = 0
  const submit = {disabled:false}, cancel = {disabled:false, click(){cancelled++}}
  vm.runInNewContext(keyCode + ';dialogKeys(dialog, form, submit, cancel)', {
    dialog:{addEventListener(name, fn){handler = fn}},
    form:{requestSubmit(button){assert.equal(button, submit); submitted++}}, submit, cancel,
  })
  const press = (key, extra = {}) => handler({key,target:{tagName:'BUTTON'},preventDefault(){},stopImmediatePropagation(){},...extra})
  press('Enter')
  press('Escape')
  assert.equal(submitted, 1)
  assert.equal(cancelled, 1)
  press('Enter', {repeat:true})
  press('Escape', {repeat:true})
  press('Enter', {isComposing:true})
  press('Enter', {target:{tagName:'TEXTAREA'}})
  press('Enter', {target:{tagName:'SELECT'}})
  submit.disabled = cancel.disabled = true
  press('Enter')
  press('Escape')
  assert.equal(submitted, 1)
  assert.equal(cancelled, 1)
})

test('Ctrl+; opens thread actions once and is accepted by shortcut normalization', () => {
  const normalize = source.slice(source.indexOf('  function shortcut(event)'), source.indexOf('  function zoom(action)'))
  const start = source.indexOf("    if (binding && binding === toolKeys['thread-actions'])")
  const handler = source.slice(start, source.indexOf("    if (binding && binding === toolKeys['finish-thread'])", start))
  for (const repeat of [false, true]) {
    let opened = 0
    vm.runInNewContext(normalize + "const binding = shortcut(event); (function(){" + handler + '})()', {
      toolKeys:{'thread-actions':'Ctrl+;'},
      event:{key:';',ctrlKey:true,repeat,preventDefault(){},stopImmediatePropagation(){}},
      threadActionPalette(){opened++},
    })
    assert.equal(opened, repeat ? 0 : 1)
  }
})

test('thread menu and palette share actions bound to the chosen worktree', () => {
  const actions = source.slice(source.indexOf('  function threadActions('), source.indexOf('  function threadActionPalette('))
  const calls = []
  vm.runInNewContext(actions + ";threadActions({name:'repo',branch:'feature'}).forEach(action => action.run())", {
    finishThread:(name, discard = false, method = 'merge') => calls.push([discard ? 'discard' : method, name]),
    projectSettings:name => calls.push(['settings', name]),
  })
  assert.deepEqual(calls, [['merge','repo/feature'], ['squash','repo/feature'], ['pr','repo/feature'], ['settings','repo/feature'], ['discard','repo/feature']])
})


test('merge, squash and PR open dedicated dialogs without a method dropdown', () => {
  for (const method of ['merge', 'squash', 'pr']) {
    const h = harness(false, method)
    h.preview()
    assert.equal(h.field('finish-thread-method'), undefined)
    h.form.onsubmit({preventDefault(){}})
    assert.equal(h.calls.at(-1).data.method, method)
  }
})


test('failed merge asks before sending to the exact thread agent; Escape sends nothing', () => {
  for (const confirm of [false, true]) {
    const h = harness()
    const sent = []
    h.context.window.__libroSendPageToolPrompt = (...args) => { sent.push(args); return true }
    h.preview()
    h.form.onsubmit({preventDefault(){}})
    vm.runInContext(`finishThreadResult('repo/feature', {error:'Merge conflict', agentMerge:{appID:'thread-agent',workspace:'repo/feature',prompt:'Resolve this merge'}})`, h.context)
    assert.deepEqual(sent, [])
    const dialog = h.context.root.children.at(-1)
    if (confirm) {
      dialog.children.find(el => el.tag === 'form').onsubmit({preventDefault(){}})
      assert.deepEqual(sent, [['Resolve this merge', true, 'thread-agent']])
      assert.equal(h.calls.at(-1).data.name, 'repo/feature')
    } else {
      dialog.events.keydown({key:'Escape',target:{tagName:'BUTTON'},preventDefault(){},stopImmediatePropagation(){}})
      assert.deepEqual(sent, [])
    }
  }
})


test('thread palette closes base and worktree workspaces without a Git deletion action', () => {
  const code = source.slice(source.indexOf('  function threadActions('), source.indexOf('  function threadMenu('))
  for (const kind of ['project', 'worktree']) {
    const calls = []
    const all = []
    const node = (tag, cls = '', text = '') => {
      const el = {tag, textContent:text, dataset:{}, children:[],
        setAttribute() {}, addEventListener() {}, showModal() {}, focus() {}, close() {},
        append(...children) { this.children.push(...children) },
      }
      all.push(el)
      return el
    }
    vm.runInNewContext(code + ';threadActionPalette();', {
      node, root:node('div'), document:{getElementById() {}},
      button:label => node('button', '', label),
      window:{__libroActiveProject:kind === 'worktree' ? 'repo/feature' : 'repo',
        __libroProjects:[{kind,name:'repo',branch:'feature'}]},
      call:action => calls.push(action),
    })
    const entry = all.find(el => el.dataset.label === 'close thread')
    assert.ok(entry)
    entry.onclick()
    assert.deepEqual(calls, ['project.close'])
  }
})
