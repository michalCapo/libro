const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const page = require('./browser-page')

// Run in an isolated world so page scripts cannot replace the cursor controller.
function cursorScript(x, y, pressed) {
  return `(() => {
    let cursor = globalThis.libroAgentCursor;
    if (!cursor || !cursor.isConnected) {
      cursor = document.createElement('div');
      cursor.setAttribute('popover', 'manual');
      cursor.setAttribute('aria-hidden', 'true');
      cursor.style.cssText = 'position:fixed;inset:auto;margin:0;padding:0;border:0;overflow:visible;background:transparent;pointer-events:none;z-index:2147483647;width:24px;height:30px';
      const root = cursor.attachShadow({mode:'closed'});
      root.innerHTML = '<svg width="24" height="30" viewBox="0 0 24 30"><path d="M2 2L2 23L8 18L13 28L18 25L13 16L22 16Z" fill="#2452df" stroke="white" stroke-width="2"/></svg><span style="position:absolute;left:19px;top:22px;background:#2452df;color:white;padding:2px 5px;border-radius:4px;font:12px system-ui">Agent</span>';
      globalThis.libroAgentCursor = cursor;
      globalThis.libroAgentCursorLabel = root.querySelector('span');
    }
    const modals = document.querySelectorAll('dialog:modal');
    (modals[modals.length-1] || document.documentElement).appendChild(cursor);
    cursor.style.left = ${x} + 'px'; cursor.style.top = ${y} + 'px';
    cursor.style.opacity = ${pressed ? '0.7' : '1'};
    const label = globalThis.libroAgentCursorLabel;
    label.style.left = ${x} > innerWidth - 80 ? '-50px' : '19px';
    label.style.top = ${y} > innerHeight - 55 ? '-20px' : '22px';
    cursor.showPopover();
    clearTimeout(globalThis.libroAgentCursorTimer);
    globalThis.libroAgentCursorTimer = setTimeout(() => cursor.remove(), 5000);
  })()`
}

async function settleInput(target) {
  await target.executeJavaScriptInIsolatedWorld(998, [{code:'new Promise(resolve => { setTimeout(resolve, 100); requestAnimationFrame(() => requestAnimationFrame(resolve)); })'}])
}

async function performAction(target, command, signal) {
  page.check(signal)
  const { action } = command
  if (action === 'screenshot') {
    const viewport = await target.executeJavaScriptInIsolatedWorld(998, [{code:'({width:innerWidth,height:innerHeight})'}])
    const capture = await target.capturePage()
    const image = capture.resize({width:viewport.width,height:viewport.height})
    return { mimeType: 'image/png', data: image.toPNG({scaleFactor:1}).toString('base64'), ...image.getSize() }
  }
  if (action === 'navigate') {
    const url = new URL(command.url)
    if (!['http:', 'https:', 'about:'].includes(url.protocol) || (url.protocol === 'about:' && url.href !== 'about:blank')) throw new Error('Use an http, https, or about:blank URL')
    await target.loadURL(url.href)
  } else if (action === 'reload') target.reload()
  else if (action === 'back' || action === 'forward') {
    const history = target.navigationHistory
    if (action === 'back' && history.canGoBack()) history.goBack()
    if (action === 'forward' && history.canGoForward()) history.goForward()
  } else if (action === 'text') {
    if (typeof command.text !== 'string' || command.text.length > 100000) throw new Error('text must be a string up to 100000 characters')
    await target.insertText(command.text)
  } else if (action === 'key') {
    if (typeof command.key !== 'string' || !command.key || command.key.length > 40) throw new Error('key is required')
    const modifiers = command.modifiers || []
    if (!Array.isArray(modifiers) || modifiers.some(m => !['shift', 'control', 'alt', 'meta'].includes(m))) throw new Error('Invalid modifiers')
    const passthrough = await target.executeJavaScript('window.__libroKeyboardPassthrough')
    target.__libroAgentInput = true
    try {
      await target.executeJavaScript('window.__libroKeyboardPassthrough = true')
      page.check(signal)
      target.sendInputEvent({ type: 'keyDown', keyCode: command.key, modifiers })
      target.sendInputEvent({ type: 'keyUp', keyCode: command.key, modifiers })
      // Let native input reach the page before restoring Libro shortcuts.
      await settleInput(target)
    } finally {
      target.__libroAgentInput = false
      if (!target.isDestroyed()) await target.executeJavaScript('window.__libroKeyboardPassthrough = ' + JSON.stringify(!!passthrough))
    }
  } else if (['move', 'click', 'down', 'up', 'scroll'].includes(action)) {
    if (command.ref || command.selector) {
      const rect = await page.elementAction(target, {...command,action:'bounds'}, signal)
      command = {...command,x:Math.round(rect.x + rect.width/2),y:Math.round(rect.y + rect.height/2)}
    }
    const { x, y } = command
    const viewport = await target.executeJavaScriptInIsolatedWorld(998, [{code:'({width:innerWidth,height:innerHeight})'}])
    if (!Number.isInteger(x) || !Number.isInteger(y) || x < 0 || y < 0 || x >= viewport.width || y >= viewport.height) throw new Error('x and y must be inside the viewport (CSS pixels)')
    const button = command.button || 'left'
    if (!['left', 'middle', 'right'].includes(button)) throw new Error('Invalid mouse button')
    if (action === 'scroll' && (!Number.isFinite(command.deltaY) || Math.abs(command.deltaY) > 10000 || !Number.isFinite(command.deltaX || 0) || Math.abs(command.deltaX || 0) > 10000)) throw new Error('Invalid scroll delta')
    page.check(signal)
    await target.executeJavaScriptInIsolatedWorld(998, [{code:cursorScript(x, y, action === 'down' || action === 'click')}])
    // Electron input coordinates are device-independent pixels, including page zoom.
    page.check(signal)
    const zoom = target.getZoomFactor()
    const point = { x: Math.round(x * zoom), y: Math.round(y * zoom) }
    if (action === 'scroll') target.sendInputEvent({type:'mouseWheel', ...point, deltaX:command.deltaX || 0, deltaY:command.deltaY})
    else {
      target.sendInputEvent({type:'mouseMove', ...point, ...(action === 'move' && command.button ? {button, modifiers:[button + 'ButtonDown']} : {})})
      if (action === 'click' || action === 'down') {
        target.__libroAgentPointer = {...point,button}
        target.sendInputEvent({type:'mouseDown', ...point, button, clickCount:1})
      }
      if (action === 'click' || action === 'up') {
        target.sendInputEvent({type:'mouseUp', ...point, button, clickCount:1})
        delete target.__libroAgentPointer
      }
    }
    await settleInput(target)
  } else throw new Error('Unknown browser action')
  return { ok: true }
}

function createController(getWindow, fromId, options = {}) {
  let queue = Promise.resolve(), generation = 0, paused = false, active = null, navigating = null
  let enabled = options.enabled !== false
  const targets = new Set()
  const getState = () => ({enabled, paused})
  function setPaused(value, cancelDownloads = false) {
    if (!value && !enabled) throw new Error('Agent browser control is disabled in Settings')
    paused = value
    if (value) {
      generation++
      active?.abort()
      if (navigating && !navigating.isDestroyed()) navigating.stop()
      for (const target of targets) {
        if (target.isDestroyed()) continue
        if (target.__libroAgentPointer) {
          target.sendInputEvent({type:'mouseUp',...target.__libroAgentPointer,clickCount:1})
          delete target.__libroAgentPointer
        }
        target.executeJavaScriptInIsolatedWorld(998,[{code:'globalThis.libroAgentCursor?.remove()'}]).catch(()=>{})
        page.stop(target,cancelDownloads)
      }
    }
    options.onState?.(getState())
    return getState()
  }
  const dispatch = command => {
    if (command.action === 'status') return Promise.resolve(getState())
    if (!enabled) return Promise.reject(new Error('Agent browser control is disabled in Settings'))
    if (command.action === 'pause' || command.action === 'stop') return Promise.resolve(setPaused(true,command.action==='stop'))
    const submitted = generation
    const run = queue.then(async () => {
      if (!enabled) throw new Error('Agent browser control is disabled in Settings')
      if (command.action !== 'list' && (paused || submitted !== generation)) throw new Error('Browser control paused by user; resume using the browser toolbar')
      const abort = new AbortController()
      active = abort
      const signal = abort.signal
      let timer
      const execute = async () => {
        const win = getWindow()
        if (!win || win.isDestroyed()) throw new Error('Desktop window is not available')
        if (command.action === 'issues') {
          const args = command.command
          if (!args || !['list', 'read', 'create', 'set_status', 'delete'].includes(args.action) || typeof args.project !== 'string' || !args.project) throw new Error('Invalid issues command')
          return win.webContents.executeJavaScript(`window.libroNotes.control(${JSON.stringify(args)})`)
        }
        if (command.action === 'application') {
          if (!['status', 'start', 'restart', 'stop'].includes(command.operation) || typeof command.project !== 'string' || !command.project) throw new Error('Invalid application command')
          return win.webContents.executeJavaScript(`window.libroWorkspace.applicationControl(${JSON.stringify({operation:command.operation, project:command.project})})`)
        }
        const allPanels = await win.webContents.executeJavaScript(`Object.entries(window.__libroWebviews || {}).flatMap(([id, wv]) => {
          try { const rect = wv.getBoundingClientRect(); return [{id, sid:wv.getAttribute('data-sid'), scope:wv.getAttribute('data-browser-scope'), title:wv.getTitle(), url:wv.getURL(), contentsId:wv.getWebContentsId(), visible:rect.width>0 && rect.height>0 && wv.checkVisibility({checkVisibilityCSS:true})}]; } catch (_) { return []; }
        })`)
        page.check(signal)
        const panels = options.scope ? allPanels.filter(panel => panel.scope === options.scope) : allPanels
        if (command.action === 'list') return panels.map(({contentsId, ...panel}) => ({...panel,paused}))
        const panel = panels.find(panel => panel.id === command.panel)
        if (!panel) throw new Error('Browser panel not found; use list first')
        const target = fromId(panel.contentsId)
        if (!target || target.isDestroyed() || target.getType() !== 'webview' || target.hostWebContents !== win.webContents) throw new Error('Browser panel is no longer available')
        if (!targets.has(target)) {
          targets.add(target)
          target.once('destroyed', () => targets.delete(target))
        }
        if (command.action === 'select_panel') {
          const selected = await win.webContents.executeJavaScript(`(() => {
            const id=${JSON.stringify(command.panel)};
            const frame=document.getElementById('frame-'+id);
            if(!frame || !window.libroWorkspace) return false;
            const project=frame.closest('[data-workspace-project]');
            if(project && project.dataset.workspaceProject!==window.__libroActiveProject) throw new Error('Switch to the panel project first');
            window.libroWorkspace.select(id);return true;
          })()`)
          if (!selected) throw new Error('Panel selection is unavailable')
          return {ok:true}
        }
        if (!panel.visible) throw new Error('Show the browser panel before controlling it')
        if (target.isLoadingMainFrame() && !['wait','diagnostics','downloads','cancel_download'].includes(command.action)) throw new Error('Page is loading; use wait to await readiness')
        await page.connect(target,signal)
        page.check(signal)
        if (['snapshot','wait','diagnostics','select_option','check','upload','download','downloads','cancel_download'].includes(command.action) || (command.action==='screenshot' && (command.fullPage || command.ref || command.selector))) {
          return page.pageAction(target,command,signal,options.downloadDir)
        }
        win.focus()
        target.focus()
        if (command.action === 'navigate') navigating = target
        try { return await performAction(target, command, signal) } finally { if (navigating === target) navigating = null }
      }
      try {
        const cancelled = new Promise((_,reject) => {
          signal.addEventListener('abort',()=>reject(new Error('Browser command cancelled or timed out')),{once:true})
          timer=setTimeout(()=>abort.abort(),25000)
        })
        return await Promise.race([execute(),cancelled])
      } finally {clearTimeout(timer);if(active===abort) active=null}
    })
    queue = run.catch(() => {})
    return run
  }
  dispatch.setPaused = setPaused
  dispatch.getState = getState
  dispatch.setEnabled = value => {
    if (typeof value !== 'boolean') throw new Error('enabled must be boolean')
    if (enabled === value) return getState()
    enabled = value
    const state = setPaused(!enabled, !enabled)
    if (!enabled) for (const target of targets) if (!target.isDestroyed()) page.suspend(target)
    return state
  }
  return dispatch
}

// Each thread has its own command queue, pause state and managed downloads.
function createScopedController(getWindow, fromId, options = {}) {
  const controllers = new Map()
  let enabled = options.enabled !== false, paused = false
  const getState = () => ({enabled, paused:paused || [...controllers.values()].some(controller => controller.getState().paused)})
  const dispatch = (command, scope) => {
    if (['application', 'issues'].includes(command.action)) return globalController(command)
    if (typeof scope !== 'string' || !/^[a-f0-9]{64}$/.test(scope)) return Promise.reject(new Error('Browser control requires a Libro thread; restart the agent panel'))
    if (!controllers.has(scope)) {
      const controller = createController(getWindow, fromId, {...options, enabled, scope, onState:() => options.onState?.(getState())})
      if (paused) controller.setPaused(true)
      controllers.set(scope, controller)
    }
    return controllers.get(scope)(command)
  }
  const globalController = createController(getWindow, fromId, options)
  dispatch.getState = getState
  dispatch.setEnabled = value => {
    globalController.setEnabled(value)
    if (enabled === value) return getState()
    enabled = value
    paused = !enabled
    for (const controller of controllers.values()) controller.setEnabled(value)
    options.onState?.(getState())
    return getState()
  }
  dispatch.setPaused = (value, cancelDownloads = false) => {
    globalController.setPaused(value, cancelDownloads)
    paused = value
    for (const controller of controllers.values()) controller.setPaused(value, cancelDownloads)
    options.onState?.(getState())
    return getState()
  }
  return dispatch
}

async function startControlServer(directory, instance, dispatch) {
  const token = crypto.randomBytes(32).toString('hex')
  const descriptor = path.join(directory, `browser-control-${instance}.json`)
  const server = http.createServer((req, res) => {
    res.setHeader('Content-Type', 'application/json')
    if (req.method !== 'POST' || req.url !== '/' || req.headers.origin || req.headers.authorization !== `Bearer ${token}`) {
      res.writeHead(403).end(JSON.stringify({error:'Forbidden'})); return
    }
    let body = ''
    req.setEncoding('utf8')
    req.on('data', chunk => { body += chunk; if (body.length > 1024 * 1024) req.destroy() })
    req.on('end', async () => {
      try {
        const command = JSON.parse(body)
        if (!command || typeof command !== 'object' || Array.isArray(command)) throw new Error('Expected a command object')
        res.end(JSON.stringify({result:await dispatch(command, req.headers['x-libro-browser-scope'])}))
      } catch (error) { res.writeHead(400).end(JSON.stringify({error:error.message})) }
    })
    req.on('error', () => {})
  })
  server.requestTimeout = 10000
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  try {
    fs.writeFileSync(descriptor, JSON.stringify({port:server.address().port, token}), {mode:0o600})
    fs.chmodSync(descriptor, 0o600)
  } catch (error) { server.close(); throw error }
  return () => {
    server.closeAllConnections()
    server.close()
    try {
      if (JSON.parse(fs.readFileSync(descriptor, 'utf8')).token === token) fs.unlinkSync(descriptor)
    } catch (_) {}
  }
}

module.exports = { cursorScript, performAction, createController, createScopedController, startControlServer }
