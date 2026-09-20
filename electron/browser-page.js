const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const {setTimeout: delay} = require('node:timers/promises')

const states = new WeakMap()
const sessions = new WeakMap()
const LIMIT = 200
function append(list, entry) { list.push({time:Date.now(), ...entry}); if (list.length > LIMIT) list.shift() }
function check(signal) { if (signal?.aborted) throw new Error('Browser work stopped by user') }
function observe(target) {
  if (states.has(target)) return states.get(target)
  const state = {epoch:crypto.randomUUID(), refs:new Set(), console:[], network:[], requests:new Map(), downloads:[], enabled:false, monitoring:false, ownsDebugger:false}
  states.set(target, state)
  target.on('did-start-navigation', (_event, _url, inPlace, mainFrame) => {
    if (mainFrame && !inPlace) { state.epoch = crypto.randomUUID(); state.refs.clear() }
  })
  target.on('console-message', (details) => {
    if (!state.monitoring || !details || !['warning','error',2,3].includes(details.level)) return
    append(state.console, {level:details.level, message:String(details.message || '').slice(0,4000), source:details.sourceId, line:details.lineNumber})
  })
  target.debugger.on('detach', () => { state.enabled = false; state.ownsDebugger = false; state.requests.clear() })
  target.debugger.on('message', (_event, method, params) => {
    if (!state.monitoring) return
    if (method === 'Network.requestWillBeSent') {
      if (state.requests.size >= 1000) state.requests.delete(state.requests.keys().next().value)
      state.requests.set(params.requestId, {url:params.request.url, method:params.request.method})
    } else if (method === 'Network.responseReceived' && params.response.status >= 400) {
      append(state.network, {url:params.response.url, status:params.response.status, kind:'http'})
    } else if (method === 'Network.loadingFailed') {
      append(state.network, {...state.requests.get(params.requestId), error:params.errorText, cancelled:!!params.canceled, kind:'failed'})
      state.requests.delete(params.requestId)
    } else if (method === 'Network.loadingFinished') state.requests.delete(params.requestId)
    else if (method === 'Runtime.exceptionThrown') {
      const detail = params.exceptionDetails
      append(state.console, {level:'error', message:String(detail.exception?.description || detail.text).slice(0,4000), source:detail.url, line:detail.lineNumber})
    }
  })
  let downloads = sessions.get(target.session)
  if (!downloads) {
    downloads = {pending:new Set(), cancelled:new Map()}
    sessions.set(target.session, downloads)
    target.session.on('will-download', (event, item, source) => {
      const chain = item.getURLChain()
      const pending = [...downloads.pending].find(pending =>
        (!source || pending.owner === states.get(source)) && chain.includes(pending.url))
      if (!pending) {
        // Electron can omit the source after its webContents is destroyed.
        // Keep cancelled requests at session scope so they cannot open a picker.
        for (const [url, expires] of downloads.cancelled) {
          if (expires <= Date.now()) downloads.cancelled.delete(url)
          else if (chain.includes(url)) { event.preventDefault(); return }
        }
        return // Ordinary user downloads keep Electron's normal save flow.
      }
      downloads.pending.delete(pending)
      pending.owner.pendingDownload = null
      if (pending.cancelled) { event.preventDefault(); pending.reject(new Error('Browser download cancelled')); return }
      item.setSavePath(pending.path)
      const record = {id:crypto.randomUUID(), url:item.getURL(), path:pending.path, state:'progressing', received:0, total:item.getTotalBytes(), item}
      pending.owner.downloads.push(record)
      if (pending.owner.downloads.length > LIMIT) pending.owner.downloads.shift()
      item.on('updated', (_event, state) => {record.state=state;record.received=item.getReceivedBytes();record.total=item.getTotalBytes()})
      item.once('done', (_event, state) => {record.state=state;record.received=item.getReceivedBytes();delete record.item})
      pending.resolve(record.id)
    })
  }
  state.downloadSession = downloads
  target.once('destroyed', () => {
    if (state.pendingDownload) {
      state.pendingDownload.cancelled = true
      state.pendingDownload.reject(new Error('Browser panel closed'))
    }
    for (const entry of state.downloads) if (entry.item) entry.item.cancel()
  })
  return state
}

async function connect(target, signal) {
  check(signal)
  const state = observe(target)
  if (!target.debugger.isAttached()) { target.debugger.attach('1.3'); state.ownsDebugger = true }
  if (!state.enabled) {
    await target.debugger.sendCommand('Network.enable')
    check(signal)
    await target.debugger.sendCommand('Runtime.enable')
    check(signal)
    state.enabled = true
  }
  check(signal)
  state.monitoring = true
  return state
}
async function cdp(target, method, params, signal) {
  check(signal)
  return target.debugger.sendCommand(method, params)
}
function reference(state, node) { const ref = `${state.epoch}:${node}`; state.refs.add(ref); return ref }

// Every lookup runs in an isolated world; selectors also traverse open shadow roots.
async function element(target, command, signal) {
  const state = await connect(target, signal)
  const {frameTree} = await cdp(target, 'Page.getFrameTree', {}, signal)
  const {executionContextId} = await cdp(target, 'Page.createIsolatedWorld', {frameId:frameTree.frame.id, worldName:'libro-browser-elements'}, signal)
  let object
  if (command.ref) {
    if (!state.refs.has(command.ref) || !command.ref.startsWith(state.epoch + ':')) throw new Error('Stale element reference; take a new snapshot')
    const backendNodeId = Number(command.ref.slice(state.epoch.length + 1))
    ;({object} = await cdp(target,'DOM.resolveNode',{backendNodeId,executionContextId},signal))
  } else {
    if (typeof command.selector !== 'string' || !command.selector || command.selector.length > 2000) throw new Error('Provide ref from snapshot or a CSS selector')
    const expression = `(() => {
      const matches=[]; const selector=${JSON.stringify(command.selector)};
      function visit(root) { matches.push(...root.querySelectorAll(selector)); for (const el of root.querySelectorAll('*')) if(el.shadowRoot) visit(el.shadowRoot); }
      visit(document);
      if(matches.length>1) throw new Error('Selector matches multiple elements; use a specific selector or ref');
      return matches[0] || null;
    })()`
    const response = await cdp(target,'Runtime.evaluate',{expression,contextId:executionContextId},signal)
    if (response.exceptionDetails) throw new Error(response.exceptionDetails.exception?.description || 'Invalid selector')
    object = response.result
  }
  if (!object?.objectId || object.subtype === 'null') throw new Error('Element not found')
  return object.objectId
}
async function onElement(target, objectId, fn, args, signal) {
  const response = await cdp(target,'Runtime.callFunctionOn', {objectId, functionDeclaration:fn.toString(), arguments:args.map(value=>({value})), returnByValue:true, awaitPromise:true}, signal)
  if (response.exceptionDetails) throw new Error(response.exceptionDetails.exception?.description || 'Element action failed')
  return response.result.value
}
async function release(target, objectId) {
  try { await target.debugger.sendCommand('Runtime.releaseObject',{objectId}) } catch (_) {}
}
async function withElement(target, command, signal, fn) {
  const objectId = await element(target,command,signal)
  try { return await fn(objectId) } finally { await release(target,objectId) }
}

async function snapshot(target, command, signal) {
  const state = await connect(target,signal)
  const epoch = state.epoch
  const nodes = []
  if (command.format && !['dom','accessibility'].includes(command.format)) throw new Error('format must be dom or accessibility')
  if (command.format === 'dom') {
    const {root} = await cdp(target,'DOM.getDocument',{depth:-1,pierce:true},signal)
    function visit(node, parent) {
      if (nodes.length >= 1000) return
      let ref = parent
      if (node.nodeType === 1) {
        ref=reference(state,node.backendNodeId)
        const attributes={}
        for(let i=0;i<(node.attributes || []).length;i+=2) if(['id','role','type','aria-label','aria-checked','aria-expanded','placeholder','alt'].includes(node.attributes[i])) attributes[node.attributes[i]]=node.attributes[i+1].slice(0,500)
        nodes.push({ref,parent,tag:node.localName,attributes})
      }
      for (const child of [...(node.children || []),...(node.shadowRoots || [])]) visit(child,ref)
    }
    visit(root,null)
  } else {
    const tree = await cdp(target,'Accessibility.getFullAXTree',{},signal)
    for (const node of tree.nodes) {
      if (node.ignored || nodes.length >= 1000) continue
      nodes.push({ref:node.backendDOMNodeId ? reference(state,node.backendDOMNodeId) : undefined, role:node.role?.value, name:String(node.name?.value || '').slice(0,500), states:(node.properties || []).filter(p=>['checked','disabled','expanded','selected','required','level'].includes(p.name)).map(p=>({name:p.name,value:p.value.value}))})
    }
  }
  if (state.epoch !== epoch) throw new Error('Page changed during snapshot; try again')
  return {url:target.getURL(),format:command.format || 'accessibility',nodes,truncated:nodes.length>=1000}
}

async function elementAction(target, command, signal) {
  return withElement(target,command,signal,objectId=>onElement(target,objectId,function(command) {
    if(this.nodeType!==1 || !this.isConnected) throw new Error('Element is detached or is not an element')
    if (command.action==='select_option') {
      if(this.tagName!=='SELECT' || this.disabled) throw new Error('Target must be an enabled select')
      const values=command.values
      if(!Array.isArray(values) || values.some(v=>typeof v!=='string') || (!this.multiple && values.length!==1)) throw new Error('values must be an array of option values')
      for(const value of values) if(!Array.from(this.options).some(o=>o.value===value && !o.disabled && !(o.parentElement.tagName==='OPTGROUP' && o.parentElement.disabled))) throw new Error('Option missing or disabled')
      for(const option of this.options) option.selected=values.includes(option.value)
      this.dispatchEvent(new Event('input',{bubbles:true}));this.dispatchEvent(new Event('change',{bubbles:true}))
      return {values:Array.from(this.selectedOptions).map(o=>o.value)}
    }
    if (command.action==='check') {
      if(this.tagName!=='INPUT' || !['checkbox','radio'].includes(this.type) || this.disabled || typeof command.checked!=='boolean') throw new Error('Target must be an enabled checkbox/radio and checked must be boolean')
      if(this.type==='radio' && !command.checked) throw new Error('Select another radio option to uncheck this one')
      if(this.checked!==command.checked) this.click()
      if(this.checked!==command.checked) throw new Error('Page did not accept requested checked state')
      return {checked:this.checked}
    }
    this.scrollIntoView({block:'center',inline:'center',behavior:'instant'})
    const r=this.getBoundingClientRect()
    if(r.width<=0 || r.height<=0 || getComputedStyle(this).visibility==='hidden') throw new Error('Element is not visible')
    return {x:r.x,y:r.y,width:r.width,height:r.height}
  },[command],signal))
}

async function waitFor(target, command, signal) {
  const timeout=command.timeoutMs ?? 10000
  if(!Number.isInteger(timeout) || timeout<0 || timeout>20000) throw new Error('timeoutMs must be between 0 and 20000')
  if(!command.selector && !command.ref && !command.url && !command.state) throw new Error('Provide selector/ref, url, or state')
  const state=command.state || (command.selector || command.ref ? 'visible' : 'complete')
  if(!['visible','hidden','attached','detached','interactive','complete'].includes(state)) throw new Error('Invalid wait state')
  const deadline=Date.now()+timeout
  do {
    check(signal)
    let ready=!command.url || target.getURL()===command.url
    if(ready && (command.selector || command.ref)) {
      try {
        ready=await withElement(target,command,signal,id=>onElement(target,id,function(state) {
          const attached=this.isConnected
          const visible=attached && this.nodeType===1 && this.checkVisibility({checkVisibilityCSS:true}) && this.getBoundingClientRect().width>0 && this.getBoundingClientRect().height>0
          return state==='attached'?attached:state==='detached'?!attached:state==='hidden'?!visible:visible
        },[state],signal))
      } catch(error) {
        if(!/Element not found|Stale element reference|Could not find node/.test(error.message)) throw error
        ready=['hidden','detached'].includes(state)
      }
    } else if(ready) {
      ready=state==='interactive' || !target.isLoadingMainFrame()
      if(ready) ready=await target.executeJavaScriptInIsolatedWorld(998,[{code:`document.readyState === 'complete' || (${JSON.stringify(state)} === 'interactive' && document.readyState === 'interactive')`}])
    }
    if(ready) return {ok:true,url:target.getURL(),state}
    if(Date.now()>=deadline) break
    await delay(Math.min(100,deadline-Date.now()),undefined,{signal})
  } while(true)
  throw new Error('Timed out waiting for browser state')
}

async function screenshot(target, command, signal) {
  await connect(target,signal)
  if(command.fullPage && (command.ref || command.selector)) throw new Error('Choose fullPage or an element, not both')
  let clip
  if(command.ref || command.selector) {
    const r=await elementAction(target,{...command,action:'bounds'},signal)
    const scroll=await target.executeJavaScriptInIsolatedWorld(998,[{code:'({x:scrollX,y:scrollY})'}])
    clip={x:r.x+scroll.x,y:r.y+scroll.y,width:r.width,height:r.height,scale:1}
  } else {
    const metrics=await cdp(target,'Page.getLayoutMetrics',{},signal)
    const r=command.fullPage?metrics.cssContentSize:metrics.cssVisualViewport
    clip={x:command.fullPage?r.x:r.pageX,y:command.fullPage?r.y:r.pageY,width:command.fullPage?r.width:r.clientWidth,height:command.fullPage?r.height:r.clientHeight,scale:1}
  }
  if(clip.width<=0 || clip.height<=0 || clip.width*clip.height>24000000 || clip.width>16000 || clip.height>16000) throw new Error('Screenshot exceeds 24 megapixels or 16000 pixels per side; use an element screenshot')
  const result=await cdp(target,'Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip},signal)
  const {nativeImage}=require('electron')
  const image=nativeImage.createFromBuffer(Buffer.from(result.data,'base64')).resize({width:Math.ceil(clip.width),height:Math.ceil(clip.height)})
  return {mimeType:'image/png',data:image.toPNG({scaleFactor:1}).toString('base64'),...image.getSize()}
}

async function upload(target, command, signal) {
  if(!Array.isArray(command.files) || command.files.length>20 || command.files.some(file=>typeof file!=='string' || !path.isAbsolute(file))) throw new Error('files must contain up to 20 absolute file paths')
  for(const file of command.files) if(!fs.statSync(file).isFile()) throw new Error('Upload path must be a file')
  return withElement(target,command,signal,async objectId=> {
    const valid=await onElement(target,objectId,function(count) {return this.isConnected && this.tagName==='INPUT' && this.type==='file' && !this.disabled && (this.multiple || count<=1)},[command.files.length],signal)
    if(!valid) throw new Error('Target must be an enabled file input with matching multiple setting')
    if (command.files.length) await cdp(target,'DOM.setFileInputFiles',{objectId,files:command.files},signal)
    else await onElement(target,objectId,function() {
      this.value=''
      this.dispatchEvent(new Event('input',{bubbles:true}))
      this.dispatchEvent(new Event('change',{bubbles:true}))
    },[],signal)
    return {ok:true,count:command.files.length}
  })
}

async function download(target, command, signal, downloadDir) {
  const url=new URL(command.url)
  if(!['http:','https:'].includes(url.protocol)) throw new Error('Download URL must use http or https')
  const state=observe(target)
  if(state.pendingDownload) throw new Error('A download is already starting')
  fs.mkdirSync(downloadDir,{recursive:true,mode:0o700})
  const directory=fs.mkdtempSync(path.join(downloadDir,'libro-'))
  const filename=command.filename || path.basename(url.pathname) || 'download'
  if(typeof filename!=='string' || filename!==path.basename(filename) || /[\\/:\x00-\x1f]/.test(filename) || ['.','..'].includes(filename)) {fs.rmdirSync(directory);throw new Error('filename must be a plain file name')}
  let timer, abort
  try {
    return await new Promise((resolve,reject)=> {
      state.downloadSession.cancelled.delete(url.href)
      state.pendingDownload={owner:state,url:url.href,path:path.join(directory,filename),reject,resolve:id=>resolve({id,path:path.join(directory,filename),state:'progressing'})}
      state.downloadSession.pending.add(state.pendingDownload)
      abort=()=>reject(new Error('Browser work stopped by user'))
      signal?.addEventListener('abort',abort,{once:true})
      timer=setTimeout(()=>reject(new Error('Download did not start within 10 seconds')),10000)
      check(signal)
      target.downloadURL(url.href)
    })
  } finally {
    clearTimeout(timer);signal?.removeEventListener('abort',abort)
    if(state.pendingDownload) {
      state.downloadSession.cancelled.set(url.href,Date.now()+30000)
      state.downloadSession.pending.delete(state.pendingDownload)
      for (const [url,expires] of state.downloadSession.cancelled) if(expires<Date.now()) state.downloadSession.cancelled.delete(url)
      state.pendingDownload=null;try{fs.rmdirSync(directory)}catch(_) {}
    }
  }
}

function suspend(target) {
  const state = states.get(target)
  if (!state) return
  state.monitoring = false
  state.console.length = 0
  state.network.length = 0
  state.requests.clear()
  state.refs.clear()
  if (state.ownsDebugger && target.debugger.isAttached()) target.debugger.detach()
  state.enabled = false
}

function stop(target, cancelDownloads) {
  const state=states.get(target)
  if(cancelDownloads) for(const record of state?.downloads || []) if(record.item) record.item.cancel()
}
async function pageAction(target, command, signal, downloadDir) {
  const state=observe(target)
  switch(command.action) {
    case 'snapshot': return snapshot(target,command,signal)
    case 'wait': return waitFor(target,command,signal)
    case 'screenshot': return screenshot(target,command,signal)
    case 'select_option': case 'check': return elementAction(target,command,signal)
    case 'upload': return upload(target,command,signal)
    case 'download': return download(target,command,signal,downloadDir)
    case 'downloads': return state.downloads.map(({item,...record})=>record)
    case 'cancel_download': {
      const record=state.downloads.find(record=>record.id===command.downloadId)
      if(!record) throw new Error('Download not found')
      record.item?.cancel();return {ok:true}
    }
    case 'diagnostics': {
      await connect(target,signal)
      const result={console:[...state.console],network:[...state.network],note:'Captured since browser control connected; reload to capture page startup.'}
      if(command.clear) {state.console.length=0;state.network.length=0}
      return result
    }
    default: throw new Error('Unknown page action')
  }
}
module.exports={observe,connect,elementAction,pageAction,stop,suspend,check}
