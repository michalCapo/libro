const {test} = require('node:test')
const {EventEmitter} = require('node:events')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const {performAction, createController, startControlServer} = require('./browser-control')

test('mouse validates bounds and accounts for zoom', async () => {
  const events = []
  const target = {
    executeJavaScriptInIsolatedWorld:async (_world, scripts) => scripts[0].code.includes('width:innerWidth') ? {width:300,height:200} : null,
    getZoomFactor:() => 2,
    sendInputEvent:event => events.push(event),
  }
  await assert.rejects(performAction(target, {action:'click',x:300,y:10}), /viewport/)
  assert.equal(events.length,0)
  await performAction(target,{action:'click',x:20,y:30})
  assert.deepEqual(events.map(e=>e.type),['mouseMove','mouseDown','mouseUp'])
  assert.ok(events.every(e=>e.x===40 && e.y===60))
  await assert.rejects(performAction(target,{action:'navigate',url:'file:///tmp/test'}),/http/)
})

test('controller requires an existing visible owned webview', async () => {
  const host = {executeJavaScript:async () => [{id:'panel',contentsId:4,visible:true}]}
  let focused = false
  const win = {isDestroyed:()=>false,webContents:host,focus:()=>{focused=true}}
  const target = Object.assign(new EventEmitter(), {isDestroyed:()=>false,getType:()=> 'webview',hostWebContents:host,isLoadingMainFrame:()=>false,focus:()=>{},insertText:async text=>{assert.equal(text,'hello')}, session:new EventEmitter(), debugger:Object.assign(new EventEmitter(),{isAttached:()=>true,sendCommand:async()=>({})})})
  const control = createController(()=>win,()=>target)
  await assert.rejects(control({action:'text',panel:'other',text:'hello'}),/not found/)
  assert.equal(focused,false)
  await control({action:'text',panel:'panel',text:'hello'})
  target.hostWebContents = {}
  await assert.rejects(control({action:'text',panel:'panel',text:'hello'}),/no longer/)
})

test('local bridge rejects unauthenticated and web-origin requests and cleans up', async () => {
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'libro-browser-test-'))
  const stop=await startControlServer(dir,'test', async command=>command)
  const descriptor=path.join(dir,'browser-control-test.json')
  try {
    const {port,token}=JSON.parse(fs.readFileSync(descriptor,'utf8'))
    const url=`http://127.0.0.1:${port}/`
    assert.equal((await fetch(url,{method:'POST',body:'{}'})).status,403)
    assert.equal((await fetch(url,{method:'POST',headers:{Authorization:`Bearer ${token}`,Origin:'https://example.test'},body:'{}'})).status,403)
    const response=await fetch(url,{method:'POST',headers:{Authorization:`Bearer ${token}`},body:JSON.stringify({action:'list'})})
    assert.deepEqual(await response.json(),{result:{action:'list'}})
    if(process.platform!=='win32') assert.equal(fs.statSync(descriptor).mode&0o777,0o600)
  } finally {stop();assert.equal(fs.existsSync(descriptor),false);fs.rmSync(dir,{recursive:true,force:true})}
})

test('pause invalidates queued commands and only the user can resume', async () => {
  let resolvePanels
  const host = {executeJavaScript:()=>new Promise(resolve=>{resolvePanels=resolve})}
  const win = {isDestroyed:()=>false,webContents:host}
  const control = createController(()=>win,()=>null)
  const active = control({action:'screenshot',panel:'panel'})
  const queued = control({action:'text',panel:'panel',text:'do not type'})
  const failures=Promise.all([assert.rejects(active,/cancelled/),assert.rejects(queued,/paused/)])
  await new Promise(resolve=>setImmediate(resolve))
  control.setPaused(true)
  resolvePanels([])
  await failures
  assert.deepEqual(await control({action:'status'}),{enabled:true,paused:true})
  await assert.rejects(control({action:'resume'}),/paused/)
  control.setPaused(false)
  assert.deepEqual(await control({action:'status'}),{enabled:true,paused:false})
})

test('late agent download without source cannot open a save dialog', async () => {
  const {pageAction} = require('./browser-page')
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'libro-download-test-'))
  const session=new EventEmitter()
  const target=Object.assign(new EventEmitter(),{session,debugger:new EventEmitter(),downloadURL:()=>{}})
  const signal=new AbortController()
  try {
    const pending=pageAction(target,{action:'download',url:'https://example.test/file'},signal.signal,dir)
    const rejected=assert.rejects(pending,/stopped/)
    signal.abort()
    await rejected
    target.emit('destroyed')
    let prevented=false
    session.emit('will-download',{preventDefault(){prevented=true}}, {getURLChain:()=>['https://example.test/file']},undefined)
    assert.equal(prevented,true,'cancelled agent download must never reach the OS picker')
    prevented=false
    session.emit('will-download',{preventDefault(){prevented=true}}, {getURLChain:()=>['https://example.test/user-file']},undefined)
    assert.equal(prevented,false,'unrelated user download must retain its normal behavior')
  } finally {fs.rmSync(dir,{recursive:true,force:true})}
})

test('feature defaults on and disabling blocks tools and toolbar resume', async () => {
  const control = createController(()=>null,()=>null)
  assert.deepEqual(await control({action:'status'}),{enabled:true,paused:false})
  control.setEnabled(false)
  assert.deepEqual(await control({action:'status'}),{enabled:false,paused:true})
  for (const action of ['list','snapshot','click','pause','stop']) {
    await assert.rejects(control({action,panel:'panel'}),/disabled in Settings/)
  }
  assert.throws(()=>control.setPaused(false),/disabled in Settings/)
  control.setEnabled(true)
  assert.deepEqual(await control({action:'status'}),{enabled:true,paused:false})
  const starting = createController(()=>null,()=>null,{enabled:false})
  await assert.rejects(starting({action:'list'}),/disabled in Settings/)
})

test('disable aborts active work and invalidates queued commands', async () => {
  let finish
  const host={executeJavaScript:()=>new Promise(resolve=>{finish=resolve})}
  const control=createController(()=>({isDestroyed:()=>false,webContents:host}),()=>null)
  const active=control({action:'screenshot',panel:'panel'})
  const queued=control({action:'click',panel:'panel',x:0,y:0})
  const rejected=Promise.all([assert.rejects(active,/cancelled/),assert.rejects(queued,/disabled|paused/)])
  await new Promise(resolve=>setImmediate(resolve))
  control.setEnabled(false)
  control.setEnabled(true)
  finish([])
  await rejected
})

test('application control uses the workspace without a browser panel and respects pause', async () => {
  const scripts = []
  const win = {isDestroyed:()=>false,webContents:{executeJavaScript:async script=>{scripts.push(script);return {status:'starting'}}}}
  const control = createController(()=>win,()=>{throw new Error('unexpected browser lookup')})
  assert.deepEqual(await control({action:'application',operation:'start',project:'/project'}),{status:'starting'})
  assert.match(scripts[0],/libroWorkspace.applicationControl/)
  assert.match(scripts[0],/"project":"\/project"/)
  await assert.rejects(control({action:'application',operation:'shell',project:'/project'}),/Invalid application/)
  control.setPaused(true)
  await assert.rejects(control({action:'application',operation:'restart',project:'/project'}),/paused/)
  control.setEnabled(false)
  await assert.rejects(control({action:'application',operation:'stop',project:'/project'}),/disabled/)
})

test('issues tool reaches the issue endpoint without a panel and propagates results and errors', async () => {
  const vm = require('node:vm')
  const requests = []
  let reply = {result:{id:'issue-id',title:'Fix login',state:'new'}}
  const context = vm.createContext({
    window:{__libroWorkspaceSID:'session'},
    fetch:async (url, options) => {
      requests.push({url,...JSON.parse(options.body)})
      return {ok:true,json:async()=>reply}
    }
  })
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../internal/notes.js'),'utf8'),context)
  const win = {isDestroyed:()=>false,webContents:{executeJavaScript:script=>vm.runInContext(script,context)}}
  const control = createController(()=>win,()=>{throw new Error('unexpected panel lookup')})
  const command = {action:'create',project:'/project',title:'Fix login',body:'Markdown **steps**'}
  assert.deepEqual(await control({action:'issues',command}),reply.result)
  assert.deepEqual(requests[0],{url:'/issues/agent',sid:'session',command})
  reply = {error:'issue not found in this project'}
  await assert.rejects(control({action:'issues',command:{action:'delete',project:'/project',id:'other'}}),/not found/)
  await assert.rejects(control({action:'issues',command:{action:'unknown',project:'/project'}}),/Invalid issues command/)
  control.setPaused(true)
  await assert.rejects(control({action:'issues',command}),/paused/)
  control.setEnabled(false)
  await assert.rejects(control({action:'issues',command}),/disabled/)
  assert.equal(requests.length,2)
})

test('thread controllers fail closed and isolate panel access and pause state', async () => {
  const {createScopedController} = require('./browser-control')
  const first = 'a'.repeat(64), second = 'b'.repeat(64)
  const host = {executeJavaScript:async () => [
    {id:'first',scope:first,contentsId:1,visible:true},
    {id:'second',scope:second,contentsId:2,visible:true},
  ]}
  let targetLookups = 0
  const control = createScopedController(()=>({isDestroyed:()=>false,webContents:host}),()=>{targetLookups++;return null})
  await assert.rejects(control({action:'list'}), /requires a Libro thread/)
  await assert.rejects(control({action:'list',scope:first}), /requires a Libro thread/)
  assert.deepEqual((await control({action:'list'}, first)).map(p=>p.id), ['first'])
  assert.deepEqual((await control({action:'list'}, second)).map(p=>p.id), ['second'])
  for (const action of ['select_panel','screenshot','text','navigate','downloads','cancel_download']) {
    await assert.rejects(control({action,panel:'second'},first), /not found/)
  }
  assert.equal(targetLookups,0)
  await control({action:'stop'},first)
  assert.equal((await control({action:'status'},first)).paused,true)
  assert.equal((await control({action:'status'},second)).paused,false)
  control.setPaused(true)
  assert.equal((await control({action:'status'},second)).paused,true)
  control.setPaused(false)
  assert.equal((await control({action:'status'},first)).paused,false)
  control.setEnabled(false)
  await assert.rejects(control({action:'list'},second), /disabled/)
})

test('thread browser scopes use the same project application controller', async () => {
  const {createScopedController} = require('./browser-control')
  const calls = []
  const host = {executeJavaScript:async script => {calls.push(script);return {status:'running'}}}
  const control = createScopedController(()=>({isDestroyed:()=>false,webContents:host}),()=>null)
  const command = {action:'application',operation:'start',project:'/shared-project'}
  const first = 'a'.repeat(64), second = 'b'.repeat(64)
  await control({action:'pause'},first)
  assert.deepEqual(await control(command,first),{status:'running'})
  assert.deepEqual(await control(command,second),{status:'running'})
  assert.equal(calls.length,2)
  assert.equal(calls[0],calls[1])
  assert.match(calls[0], /applicationControl/)
})
