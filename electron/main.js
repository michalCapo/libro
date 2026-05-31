const { app, BrowserWindow, Menu, session, ipcMain, shell, clipboard, globalShortcut, webContents, WebContentsView, nativeImage, powerMonitor } = require('electron')
const { spawn } = require('child_process')
const fs = require('fs')
const path = require('path')
const http = require('http')
const os = require('os')

// Libro intentionally hosts arbitrary external pages in Electron webviews.
// Electron's CSP warning is not actionable for those guest pages in development.
process.env.ELECTRON_DISABLE_SECURITY_WARNINGS = 'true'

// Enable PipeWire for WebRTC capture on Linux.
// This is primarily required for screen sharing on modern Wayland desktops.
app.commandLine.appendSwitch('enable-features', 'WebRTCPipeWireCapturer')

// Pin Electron's profile directory so persistent webview partitions survive relaunches.
app.setPath('userData', path.join(app.getPath('appData'), 'libro'))
app.setName('Libro')
if (process.platform === 'windows') {
  app.setAppUserModelId('com.michalcapo.libro')
}

const port = process.env.LIBRO_PORT || '8100'
const serverURL = `http://localhost:${port}`

let goProcess = null
let mainWindow = null
let isQuitting = false
let goProcessForceKillTimer = null
let webviewPreloadRegistered = false
const devtoolsOverlays = new Map()
const MAIN_WINDOW_ZOOM_STEP = 0.15
let lastMainWindowZoomAction = { action: '', at: 0 }
let lastResumeTerminalRefreshAt = 0

function resolveAppIconPath() {
  const candidates = [
    path.join(__dirname, '..', 'winres', 'icon.png'),
    path.join(process.resourcesPath || '', 'app', 'winres', 'icon.png'),
  ]
  for (const candidate of candidates) {
    if (candidate && fs.existsSync(candidate)) return candidate
  }
  return null
}

const appIconPath = resolveAppIconPath()
const appIcon = appIconPath ? nativeImage.createFromPath(appIconPath) : null

if (process.platform === 'darwin' && appIcon && !appIcon.isEmpty() && app.dock) {
  app.dock.setIcon(appIcon)
}

function withWebContents(id) {
  const contents = webContents.fromId(Number(id))
  if (!contents || contents.isDestroyed()) return null
  return contents
}

function panelShortcut(panel) {
  if (panel === 'elements') return { keyCode: 'C', code: 'KeyC', shift: true }
  if (panel === 'console') return { keyCode: 'J', code: 'KeyJ', shift: true }
  return null
}

function selectDevToolsPanel(devtoolsContents, panel) {
  const shortcut = panelShortcut(panel)
  if (!devtoolsContents || !shortcut) return
  const control = process.platform !== 'darwin'
  const meta = process.platform === 'darwin'
  const event = {
    type: 'keyDown',
    keyCode: shortcut.keyCode,
    code: shortcut.code,
    shift: !!shortcut.shift,
    control,
    meta,
  }
  const release = { ...event, type: 'keyUp' }
  for (const delay of [150, 325]) {
    setTimeout(() => {
      try {
        devtoolsContents.sendInputEvent(event)
        devtoolsContents.sendInputEvent(release)
      } catch (err) {
        console.error('Failed to switch DevTools panel:', err.message)
      }
    }, delay)
  }
}

function activateDevToolsPicker(devtoolsContents) {
  if (!devtoolsContents) return
  const isMac = process.platform === 'darwin'
  setTimeout(() => {
    try {
      devtoolsContents.sendInputEvent({
        type: 'keyDown',
        keyCode: 'C',
        code: 'KeyC',
        control: !isMac,
        shift: !isMac,
        meta: isMac,
        alt: isMac,
      })
      devtoolsContents.sendInputEvent({
        type: 'keyUp',
        keyCode: 'C',
        code: 'KeyC',
        control: !isMac,
        shift: !isMac,
        meta: isMac,
        alt: isMac,
      })
    } catch (err) {
      console.error('Failed to activate DevTools picker:', err.message)
    }
  }, 260)
}

function normalizeBounds(bounds) {
  if (!bounds) return null
  const x = Math.round(Number(bounds.x) || 0)
  const y = Math.round(Number(bounds.y) || 0)
  const width = Math.max(1, Math.round(Number(bounds.width) || 0))
  const height = Math.max(1, Math.round(Number(bounds.height) || 0))
  return { x, y, width, height }
}

function resetDevtoolsZoom(contents) {
  if (!contents || contents.isDestroyed()) return
  try {
    contents.setZoomLevel(0)
  } catch (err) {}
  try {
    contents.setVisualZoomLevelLimits(1, 1)
  } catch (err) {}
}

function refocusDevtoolsTarget(target) {
  if (!target || target.isDestroyed()) return
  for (const delay of [0, 40, 120, 260, 500]) {
    setTimeout(() => {
      try {
        if (mainWindow && !mainWindow.isDestroyed()) mainWindow.focus()
      } catch (err) {}
      try {
        if (!target.isDestroyed()) target.focus()
      } catch (err) {}
      try {
        if (!target.isDestroyed()) target.executeJavaScript('window.focus();').catch(() => {})
      } catch (err) {}
    }, delay)
  }
}

function destroyDevtoolsOverlay(targetId) {
  const target = withWebContents(targetId)
  const entry = target ? devtoolsOverlays.get(target.id) : devtoolsOverlays.get(Number(targetId))
  if (!entry) return
  if (mainWindow && !mainWindow.isDestroyed()) {
    try {
      if (entry.attached && mainWindow.contentView && typeof mainWindow.contentView.removeChildView === 'function') {
        mainWindow.contentView.removeChildView(entry.view)
      }
    } catch (err) {
      console.error('Failed to detach DevTools view:', err.message)
    }
  }
  try {
    if (entry.view && entry.view.webContents && !entry.view.webContents.isDestroyed()) {
      entry.view.webContents.close()
    }
  } catch (err) {
    console.error('Failed to destroy DevTools view:', err.message)
  }
  devtoolsOverlays.delete(Number(targetId))
}

function closeDevtoolsOverlay(target) {
  if (!target || target.isDestroyed()) return
  try {
    if (target.isDevToolsOpened()) {
      target.closeDevTools()
    }
  } catch (err) {}
  destroyDevtoolsOverlay(target.id)
  refocusDevtoolsTarget(target)
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('libro-webview-devtools-closed', target.id)
  }
}

function ensureDevtoolsOverlay(targetId, bounds) {
  const target = withWebContents(targetId)
  const normalizedBounds = normalizeBounds(bounds)
  if (!target || !normalizedBounds || !mainWindow || mainWindow.isDestroyed()) return null

  let entry = devtoolsOverlays.get(target.id)
  if (!entry) {
    const view = new WebContentsView()
    entry = { view, opened: false, attached: false }
    devtoolsOverlays.set(target.id, entry)
    resetDevtoolsZoom(view.webContents)
    view.webContents.on('did-finish-load', () => {
      resetDevtoolsZoom(view.webContents)
    })
    try {
      target.setDevToolsWebContents(view.webContents)
    } catch (err) {
      console.error('Failed to bind DevTools view:', err.message)
      try { view.webContents.close() } catch (closeErr) {}
      devtoolsOverlays.delete(target.id)
      return null
    }
    view.webContents.on('before-input-event', (event, input) => {
      if (input.type !== 'keyDown' && input.type !== 'rawKeyDown') return
      const key = (input.key || '').toLowerCase()
      if (input.meta || input.control || input.alt || input.shift || key !== 'c') return
      event.preventDefault()
      closeDevtoolsOverlay(target)
    })
  }

  try {
    if (!entry.attached) {
      mainWindow.contentView.addChildView(entry.view)
      entry.attached = true
    }
    entry.view.setBounds(normalizedBounds)
    entry.view.setVisible(true)
  } catch (err) {
    console.error('Failed to place DevTools view:', err.message)
    return null
  }

  return { target, devtools: entry.view.webContents, entry }
}

const allowedPermissions = new Set([
  'media',
  'display-capture',
  'fullscreen',
  'clipboard-read',
  'clipboard-sanitized-write',
  'notifications',
  'idle-detection',
  'pointerLock',
  'openExternal',
])

function isAllowedPermission(permission) {
  return allowedPermissions.has(permission)
}

function getSupportedBrowserUserAgent() {
  const fallback = app.userAgentFallback || ''
  return fallback
    .replace(/\sElectron\/[^\s]+/g, '')
    .replace(/\sLibro\/[^\s]+/g, '')
    .trim()
}

function dispatchToMainWindow(js) {
  if (!mainWindow || mainWindow.isDestroyed()) return
  mainWindow.webContents.executeJavaScript(js).catch((err) => {
    console.error('Failed to dispatch shortcut JS:', err.message)
  })
}

function showQuitCommandOnlyToast() {
  dispatchToMainWindow(`
    if (window.__libroNotifyQuitCommandOnly) {
      window.__libroNotifyQuitCommandOnly();
    }
  `)
}

function moveSelectedApp(direction) {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow) {
    console.log(`[libro-shortcut] ignored move ${direction}: main window not focused`)
    return
  }
  console.log(`[libro-shortcut] firing move ${direction}`)
  dispatchToMainWindow(`
    if (window.__libroMoveSelectedApp) {
      window.__libroMoveSelectedApp('${direction}');
    }
  `)
}

function openMoveProjectPopup() {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow) {
    console.log('[libro-shortcut] ignored move-project popup: main window not focused')
    return
  }
  console.log('[libro-shortcut] firing move-project popup')
  dispatchToMainWindow(`
    if (window.__libroOpenMoveProject) {
      window.__libroOpenMoveProject();
    }
  `)
}

function triggerProjectDialogShortcut() {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow) {
    console.log('[libro-shortcut] ignored project dialog: main window not focused')
    return
  }
  console.log('[libro-shortcut] firing project dialog')
  dispatchToMainWindow(`
    (function(){
      var dialog = document.getElementById('project-dialog');
      var input = document.getElementById('project-input');
      if (dialog && !dialog.classList.contains('hidden')) {
        if (input) input.focus();
        return;
      }
      if (window.__libroOpenProjectDialog) window.__libroOpenProjectDialog();
    })();
  `)
}


function triggerTerminalAppShortcut() {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow) {
    console.log('[libro-shortcut] ignored terminal: main window not focused')
    return
  }
  console.log('[libro-shortcut] firing terminal')
  dispatchToMainWindow(`
    if (window.__libroOpenTerminalApp) {
      window.__libroOpenTerminalApp();
    }
  `)
}

function triggerBrowserAppShortcut() {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow) {
    console.log('[libro-shortcut] ignored browser: main window not focused')
    return
  }
  console.log('[libro-shortcut] firing browser')
  dispatchToMainWindow(`
    if (window.__libroOpenBrowserApp) {
      window.__libroOpenBrowserApp();
    }
  `)
}

function triggerNvimAppShortcut() {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow) {
    console.log('[libro-shortcut] ignored nvim: main window not focused')
    return
  }
  console.log('[libro-shortcut] firing nvim')
  dispatchToMainWindow(`
    if (window.__libroOpenNvimApp) {
      window.__libroOpenNvimApp();
    }
  `)
}

function adjustMainWindowZoom(action) {
  const focusedWindow = BrowserWindow.getFocusedWindow()
  if (!focusedWindow || focusedWindow !== mainWindow || !mainWindow || mainWindow.isDestroyed()) {
    console.log(`[libro-shortcut] ignored zoom ${action}: main window not focused`)
    return
  }

  const now = Date.now()
  if (lastMainWindowZoomAction.action === action && now - lastMainWindowZoomAction.at < 80) {
    return
  }
  lastMainWindowZoomAction = { action, at: now }

  const wc = mainWindow.webContents
  const current = wc.getZoomLevel()
  if (action === 'reset') {
    wc.setZoomLevel(0)
  } else if (action === 'out') {
    wc.setZoomLevel(current - MAIN_WINDOW_ZOOM_STEP)
  } else {
    wc.setZoomLevel(current + MAIN_WINDOW_ZOOM_STEP)
  }
  console.log(`[libro-shortcut] zoom ${action}: ${wc.getZoomLevel()}`)
}

function registerWindowShortcuts() {
  const shortcuts = [
    ['Super+[', () => moveSelectedApp('left')],
    ['Super+]', () => moveSelectedApp('right')],
    ['Super+Control+Y', () => openMoveProjectPopup()],
    ['Super+N', () => triggerProjectDialogShortcut()],
    ['Super+Enter', () => triggerTerminalAppShortcut()],
    ['Super+B', () => triggerBrowserAppShortcut()],
    ['Super+E', () => triggerNvimAppShortcut()],
    ['Super+=', () => adjustMainWindowZoom('in')],
    ['Super+Plus', () => adjustMainWindowZoom('in')],
    ['Super+-', () => adjustMainWindowZoom('out')],
    ['Super+_', () => adjustMainWindowZoom('out')],
    ['Super+0', () => adjustMainWindowZoom('reset')],
  ]

  for (const [accelerator, handler] of shortcuts) {
    try {
      const ok = globalShortcut.register(accelerator, handler)
      console.log(`[libro-shortcut] register ${accelerator}: ${ok ? 'ok' : 'failed'}`)
    } catch (err) {
      console.error(`Failed to register shortcut ${accelerator}:`, err.message)
    }
  }
}

// Track input focus state per webview webContents for browser shortcut key handling.
// When an input/textarea/select/contentEditable is focused inside a webview,
// we must NOT intercept plain keys (j/k/h/l etc.) so the user can type.
const webviewInputFocused = new Map()
const webviewBrowserMode = new Map()
const webviewKeyboardPassthrough = new Map()
const shortcutEventDedup = new Map()

// Find the Go binary — look next to the electron dir, or in PATH
function findGoBinary() {
  const fs = require('fs')
  // Next to electron/ directory (project root)
  const candidates = [
    path.join(__dirname, '..', 'libro'),
    path.join(__dirname, '..', 'libro.exe'),
  ]
  for (const p of candidates) {
    if (fs.existsSync(p)) return p
  }
  return 'libro' // fall back to PATH
}

function startGoServer() {
  const bin = findGoBinary()
  goProcess = spawn(bin, ['--no-desktop'], {
    env: { ...process.env },
    stdio: 'inherit',
  })
  goProcess.on('error', (err) => {
    console.error('Failed to start Go server:', err.message)
  })
  goProcess.on('exit', (code) => {
    console.log('Go server exited with code', code)
    if (goProcessForceKillTimer) {
      clearTimeout(goProcessForceKillTimer)
      goProcessForceKillTimer = null
    }
    goProcess = null
  })
}

function stopGoServer() {
  if (!goProcess) return
  const child = goProcess
  try {
    child.kill('SIGTERM')
  } catch (err) {
    console.error('Failed to stop Go server:', err.message)
  }
  if (goProcessForceKillTimer) clearTimeout(goProcessForceKillTimer)
  goProcessForceKillTimer = setTimeout(() => {
    if (goProcess === child) {
      try {
        child.kill('SIGKILL')
      } catch (err) {}
      goProcess = null
    }
    goProcessForceKillTimer = null
  }, 2500)
}

function refreshTerminalFramesAfterResume() {
  if (!mainWindow || mainWindow.isDestroyed()) return
  const now = Date.now()
  if (now - lastResumeTerminalRefreshAt < 3000) return
  lastResumeTerminalRefreshAt = now
  setTimeout(() => {
    if (!mainWindow || mainWindow.isDestroyed()) return
    mainWindow.webContents.executeJavaScript(`
      (function(){
        var frames = document.querySelectorAll('iframe[data-terminal-iframe]');
        for (var i = 0; i < frames.length; i++) {
          var frame = frames[i];
          var src = frame.getAttribute('src') || '';
          if (!src || src.indexOf('/ttyd/') === -1) continue;
          var base = src.split('#')[0].split('?')[0];
          frame.setAttribute('src', base + '?resume=' + Date.now());
        }
      })();
    `).catch((err) => {
      console.error('Failed to refresh terminal frames after resume:', err.message)
    })
  }, 1000)
}

// Check if the Go server is already running (launched by the Go binary in desktop mode)
function isServerRunning() {
  return new Promise((resolve) => {
    http.get(serverURL, (res) => {
      res.resume()
      resolve(true)
    }).on('error', () => {
      resolve(false)
    })
  })
}

function waitForServer(retries = 50) {
  return new Promise((resolve, reject) => {
    let attempt = 0
    const check = () => {
      http.get(serverURL, (res) => {
        res.resume()
        resolve()
      }).on('error', () => {
        attempt++
        if (attempt >= retries) {
          reject(new Error('Server did not start in time'))
        } else {
          setTimeout(check, 100)
        }
      })
    }
    check()
  })
}

function registerLibroWebviewPreload(libroSession) {
  if (webviewPreloadRegistered) return

  const id = 'libro-webview-quirks'
  try {
    if (typeof libroSession.getPreloadScripts === 'function') {
      const existing = libroSession.getPreloadScripts().some((script) => script.id === id)
      if (existing) {
        webviewPreloadRegistered = true
        return
      }
    }
    libroSession.registerPreloadScript({
      id,
      type: 'frame',
      filePath: path.join(__dirname, 'webview-preload.js'),
    })
    webviewPreloadRegistered = true
  } catch (err) {
    console.error('Failed to register webview preload:', err.message)
  }
}

async function flushLibroSessionData() {
  const libroSession = session.fromPartition('persist:libro')
  try {
    await libroSession.cookies.flushStore()
  } catch (err) {
    console.error('Failed to flush cookie store:', err.message)
  }
  try {
    await libroSession.flushStorageData()
  } catch (err) {
    console.error('Failed to flush session storage:', err.message)
  }
}

async function quitApp() {
  if (isQuitting) return
  isQuitting = true
  await flushLibroSessionData()
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.destroy()
    return
  }
  app.quit()
}

function createWindow() {
  // Persistent partition for webview sessions (shared cookies across webviews)
  const libroSession = session.fromPartition('persist:libro')
  registerLibroWebviewPreload(libroSession)

  // Allow embedded browser apps to use media devices and related browser APIs.
  // Teams relies on `media` for microphone/camera/speaker access and
  // `display-capture` for screen sharing.
  libroSession.setPermissionRequestHandler((webContents, permission, callback, details) => {
    callback(isAllowedPermission(permission))
  })

  libroSession.setPermissionCheckHandler((webContents, permission, requestingOrigin, details) => {
    return isAllowedPermission(permission)
  })

  libroSession.setDevicePermissionHandler((details) => {
    return true
  })

  libroSession.setDisplayMediaRequestHandler((request, callback) => {
    // Let Chromium use the system picker when available. If it falls back to
    // this handler, grant the requesting tab/window for app-based screen share.
    callback({ video: request.frame })
  }, { useSystemPicker: true })

  // Handle file downloads from webviews — auto-save to Downloads, show toast
  let nextDownloadId = 1
  const activeDownloads = new Map() // id -> DownloadItem

  libroSession.on('will-download', (event, item) => {
    const id = nextDownloadId++
    const filename = item.getFilename()
    const downloadsDir = app.getPath('downloads')
    const savePath = path.join(downloadsDir, filename)
    item.setSavePath(savePath)
    activeDownloads.set(id, item)

    const safeFilename = JSON.stringify(filename)
    const totalBytes = item.getTotalBytes()

    // Show downloading toast with cancel button
    if (mainWindow) {
      mainWindow.webContents.executeJavaScript(`
        if (window.__libroShowDownloadProgress) window.__libroShowDownloadProgress(${id}, ${safeFilename}, 0, ${totalBytes});
      `)
    }

    item.on('updated', (e, state) => {
      if (!mainWindow) return
      if (state === 'progressing' && !item.isPaused()) {
        mainWindow.webContents.executeJavaScript(`
          if (window.__libroUpdateDownloadProgress) window.__libroUpdateDownloadProgress(${id}, ${item.getReceivedBytes()}, ${totalBytes});
        `)
      }
    })

    item.once('done', (e, state) => {
      activeDownloads.delete(id)
      if (!mainWindow) return
      if (state === 'completed') {
        const safePath = JSON.stringify(savePath)
        mainWindow.webContents.executeJavaScript(`
          if (window.__libroShowDownloadToast) window.__libroShowDownloadToast(${id}, ${safeFilename}, ${safePath});
        `)
      } else if (state === 'cancelled') {
        mainWindow.webContents.executeJavaScript(`
          if (window.__libroRemoveDownloadToast) window.__libroRemoveDownloadToast(${id});
        `)
      } else {
        mainWindow.webContents.executeJavaScript(`
          if (window.__libroShowDownloadFailed) window.__libroShowDownloadFailed(${id}, ${safeFilename});
        `)
      }
    })
  })

  ipcMain.on('libro-cancel-download', (event, id) => {
    const item = activeDownloads.get(id)
    if (item) item.cancel()
  })

  mainWindow = new BrowserWindow({
    width: 1920,
    height: 1080,
    show: false,
    frame: false,
    autoHideMenuBar: true,
    ...(appIconPath ? { icon: appIconPath } : {}),
    webPreferences: {
      webviewTag: true,
      nodeIntegration: false,
      contextIsolation: true,
      preload: path.join(__dirname, 'preload.js'),
    },
  })

  mainWindow.maximize()
  mainWindow.show()

  mainWindow.loadURL(serverURL).catch((err) => {
    if (err && err.code === 'ERR_ABORTED') return
    console.error(`Failed to load Libro UI at ${serverURL}:`, err && err.message ? err.message : err)
  })

  // Native window-close requests are ignored unless quit was requested explicitly.
  mainWindow.on('close', (e) => {
    if (isQuitting) return
    e.preventDefault()
    showQuitCommandOnlyToast()
  })

  // Renderer signals that user confirmed close (or no apps were running)
  ipcMain.on('libro-force-close', () => {
    quitApp().catch((err) => {
      console.error('Failed to quit cleanly:', err.message)
      if (mainWindow && !mainWindow.isDestroyed()) mainWindow.destroy()
    })
  })

  ipcMain.on('libro-toggle-devtools', () => {
    if (!mainWindow || mainWindow.isDestroyed()) return
    const contents = mainWindow.webContents
    if (contents.isDevToolsOpened()) {
      contents.closeDevTools()
      return
    }
    // Docked DevTools do not lay out reliably after renderer zoom changes.
    // Opening them detached keeps the inspected window zoomed while DevTools
    // stays at its own native scale.
    contents.openDevTools({ mode: 'detach', activate: false })
  })

  ipcMain.on('libro-toggle-maximize', () => {
    if (!mainWindow || mainWindow.isDestroyed()) return
    if (mainWindow.isMaximized()) {
      mainWindow.unmaximize()
      return
    }
    mainWindow.maximize()
  })

  ipcMain.on('libro-toggle-webview-devtools', (event, targetId, bounds, panel) => {
    const pair = ensureDevtoolsOverlay(targetId, bounds)
    if (!pair) return
    try {
      if (pair.entry.opened && pair.target.isDevToolsOpened()) {
        closeDevtoolsOverlay(pair.target)
      } else {
        pair.target.openDevTools({ mode: 'detach', activate: false })
        resetDevtoolsZoom(pair.devtools)
        pair.entry.opened = true
        selectDevToolsPanel(pair.devtools, panel)
        refocusDevtoolsTarget(pair.target)
      }
    } catch (err) {
      console.error('Failed to toggle webview DevTools:', err.message)
    }
  })

  ipcMain.on('libro-open-webview-devtools', (event, targetId, bounds, panel) => {
    const pair = ensureDevtoolsOverlay(targetId, bounds)
    if (!pair) return
    try {
      if (!pair.target.isDevToolsOpened()) {
        pair.target.openDevTools({ mode: 'detach', activate: false })
        pair.entry.opened = true
      }
      resetDevtoolsZoom(pair.devtools)
      selectDevToolsPanel(pair.devtools, panel)
      refocusDevtoolsTarget(pair.target)
    } catch (err) {
      console.error('Failed to open webview DevTools:', err.message)
    }
  })

  ipcMain.on('libro-close-webview-devtools', (event, targetId) => {
    const target = withWebContents(targetId)
    if (!target) return
    try {
      closeDevtoolsOverlay(target)
    } catch (err) {
      console.error('Failed to close webview DevTools:', err.message)
    }
  })

  ipcMain.on('libro-inspect-webview-element', (event, targetId, bounds, x, y) => {
    const pair = ensureDevtoolsOverlay(targetId, bounds)
    if (!pair) return
    try {
      if (!pair.target.isDevToolsOpened()) {
        pair.target.openDevTools({ mode: 'detach', activate: false })
        pair.entry.opened = true
      }
      resetDevtoolsZoom(pair.devtools)
      pair.target.inspectElement(Number(x) || 0, Number(y) || 0)
      selectDevToolsPanel(pair.devtools, 'elements')
      activateDevToolsPicker(pair.devtools)
      refocusDevtoolsTarget(pair.target)
    } catch (err) {
      console.error('Failed to inspect webview element:', err.message)
    }
  })

  ipcMain.on('libro-update-webview-devtools-bounds', (event, targetId, bounds) => {
    const target = withWebContents(targetId)
    const entry = target ? devtoolsOverlays.get(target.id) : null
    if (!entry || !entry.opened || !target.isDevToolsOpened()) return
    ensureDevtoolsOverlay(targetId, bounds)
  })

  ipcMain.on('libro-copy-clipboard', (event, text) => {
    if (text) clipboard.writeText(text)
  })

  ipcMain.on('libro-open-path', (event, filePath) => {
    shell.openPath(filePath)
  })

  ipcMain.on('libro-open-downloads-folder', () => {
    shell.openPath(app.getPath('downloads'))
  })

  ipcMain.on('libro-zoom-in', () => {
    adjustMainWindowZoom('in')
  })

  ipcMain.on('libro-zoom-out', () => {
    adjustMainWindowZoom('out')
  })

  ipcMain.on('libro-zoom-reset', () => {
    adjustMainWindowZoom('reset')
  })

  mainWindow.on('closed', () => {
    mainWindow = null
    if (isQuitting) {
      app.quit()
    }
  })
}

// Intercept keyboard shortcuts from webview guest pages in the main process.
// The renderer-side <webview> before-input-event is unreliable — this catches
// input at the webContents level before the guest page can consume it.
app.on('web-contents-created', (event, contents) => {
  // Intercept new window requests from webviews — open as a new browser app
  if (contents.getType() === 'webview') {
    const supportedUA = getSupportedBrowserUserAgent()
    if (supportedUA) {
      contents.setUserAgent(supportedUA)
    }

    // Track input focus state via console messages from the injected browserShortcutsScript.
    // This lets the main process know whether to intercept plain keys (j/k/h/l etc.)
    // or let them through to text input fields.
    contents.on('console-message', (e) => {
      const message = e.message || ''
      if (message === '__libro:passthrough') {
        webviewKeyboardPassthrough.set(contents.id, true)
        webviewInputFocused.set(contents.id, true)
        setWebviewBrowserMode('insert')
      } else if (message === '__libro:inputfocus') {
        webviewInputFocused.set(contents.id, true)
        setWebviewBrowserMode('insert')
      } else if (message === '__libro:inputblur') {
        if (webviewKeyboardPassthrough.get(contents.id)) {
          webviewInputFocused.set(contents.id, true)
          setWebviewBrowserMode('insert')
        } else {
          webviewInputFocused.set(contents.id, false)
          setWebviewBrowserMode('normal')
        }
      }
    })
    // Reset focus tracking only for real document navigations.
    // In-page navigations such as history.pushState should preserve passthrough
    // mode so apps like mail do not fall back to normal mode on j/k movement.
    contents.on('did-start-navigation', (_event, _url, isInPlace, isMainFrame) => {
      if (!isMainFrame || isInPlace) return
      webviewInputFocused.set(contents.id, false)
      webviewKeyboardPassthrough.delete(contents.id)
      setWebviewBrowserMode('normal')
    })
    contents.on('destroyed', () => {
      webviewInputFocused.delete(contents.id)
      webviewBrowserMode.delete(contents.id)
      webviewKeyboardPassthrough.delete(contents.id)
      destroyDevtoolsOverlay(contents.id)
    })

    contents.setWindowOpenHandler(({ url, disposition }) => {
      // Download links (e.g. <a download>) — trigger download instead of opening a tab
      if (disposition === 'save-to-disk' && url && url !== 'about:blank') {
        contents.downloadURL(url)
        return { action: 'deny' }
      }
      // Allow script-driven about:blank popups to open as native windows.
      // Some sites render generated content into the new window after calling
      // window.open(), so converting these to Libro tabs loses the content.
      if (url === 'about:blank') {
        return {
          action: 'allow',
          overrideBrowserWindowOptions: {
            autoHideMenuBar: true,
            parent: mainWindow || undefined,
            width: 960,
            height: 720
          }
        }
      }
      if (mainWindow) {
        const nextUrl = url || ''
        const safeUrl = nextUrl.replace(/\\/g, '\\\\').replace(/'/g, "\\'")
        mainWindow.webContents.executeJavaScript(`
          if (window.__libroOpenNewTab) window.__libroOpenNewTab('${safeUrl}');
        `)
      }
      return { action: 'deny' }
    })
  }

  const setWebviewBrowserMode = (mode) => {
    webviewBrowserMode.set(contents.id, mode)
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.executeJavaScript(`
        if (window.__libroSetBrowserModeByWcId) window.__libroSetBrowserModeByWcId(${contents.id}, ${JSON.stringify(mode)});
      `).catch(() => {})
    }
  }

  contents.on('before-input-event', (e, input) => {
    if (input.type !== 'keyDown' && input.type !== 'rawKeyDown') return

    const key = (input.key || '').toLowerCase()
    const code = (input.code || '').toLowerCase()
    const isWebview = contents.getType() === 'webview'
    const isMainWindowContents = !!(mainWindow && !mainWindow.isDestroyed() && contents.id === mainWindow.webContents.id)

    // Native terminals live in the host BrowserWindow. Non-Super input should
    // go straight to the renderer/xterm instead of synchronously traversing the
    // main-process shortcut path on every typed character or control sequence.
    if (isMainWindowContents && !input.meta) return

    const shortcutSig = [
      code,
      key,
      input.meta ? 'meta' : '',
      input.control ? 'ctrl' : '',
      input.alt ? 'alt' : '',
      input.shift ? 'shift' : '',
    ].join('|')

    const shouldSkipDuplicateShortcut = () => {
      const now = Date.now()
      const last = shortcutEventDedup.get(contents.id)
      if (last && last.sig === shortcutSig && now-last.at < 80) {
        e.preventDefault()
        return true
      }
      shortcutEventDedup.set(contents.id, { sig: shortcutSig, at: now })
      return false
    }

    const isZoomInKey = key === '+' || key === '=' || code === 'equal' || code === 'numpadadd'
    const isZoomOutKey = key === '-' || code === 'minus' || code === 'numpadsubtract'
    const isZoomResetKey = key === '0' || code === 'digit0' || code === 'numpad0'

    // Cmd/Super + Plus/Minus/0: zoom whole application
    if (input.meta && !input.control && !input.alt && (isZoomInKey || isZoomOutKey || isZoomResetKey)) {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      adjustMainWindowZoom(isZoomResetKey ? 'reset' : (isZoomOutKey ? 'out' : 'in'))
      return
    }

    // For the main window host page: directly invoke JS for a few Super shortcuts
    // that desktop environments or Chromium can intercept before the page sees them.
    if (isMainWindowContents) {
      // Super+Ctrl+U/I: move app left/right — must preventDefault to avoid
      // Chromium accelerators consuming the event before page JS sees it
      if (input.meta && input.control && (code === 'keyu' || code === 'keyi')) {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          const direction = code === 'keyu' ? 'left' : 'right'
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroMoveSelectedApp) {
              window.__libroMoveSelectedApp('${direction}');
            } else {
              document.dispatchEvent(new KeyboardEvent('keydown', {
                code: '${input.code || ''}',
                metaKey: true,
                ctrlKey: true,
                bubbles: true,
                cancelable: true
              }));
            }
          `)
        }
        return
      }
      if (input.meta && !input.control && code === 'keyn') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        triggerProjectDialogShortcut()
        return
      }
      if (input.meta && !input.control && (code === 'enter' || key === 'enter')) {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        triggerTerminalAppShortcut()
        return
      }
      if (input.meta && !input.control && code === 'keyo') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenSearch) window.__libroOpenSearch('right');
          `)
        }
        return
      }
      // Super+Ctrl+Y: move selected app to project popup
      if (input.meta && input.control && code === 'keyy') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenMoveProject) window.__libroOpenMoveProject();
          `)
        }
        return
      }
      // Super+B: new browser with URL popup
      if (input.meta && !input.control && code === 'keyb') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenBrowserApp) window.__libroOpenBrowserApp();
          `)
        }
        return
      }
      // Super+E: open nvim terminal app
      if (input.meta && !input.control && code === 'keye') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenNvimApp) window.__libroOpenNvimApp();
          `)
        }
        return
      }
      if (input.meta && !input.control && (code === 'comma' || code === 'period')) {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          const safeKey = input.key.replace(/'/g, "\\'")
          const safeCode = (input.code || '').replace(/'/g, "\\'")
          mainWindow.webContents.executeJavaScript(`
            document.dispatchEvent(new KeyboardEvent('keydown', {
              key: '${safeKey}',
              code: '${safeCode}',
              metaKey: true,
              bubbles: true,
              cancelable: true
            }));
          `)
        }
        return
      }
      // Super+;: command palette
      if (input.meta && !input.control && code === 'semicolon') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenCommandPalette) window.__libroOpenCommandPalette();
          `)
        }
        return
      }
      // Super+X: toggle active project shortcut assignment
      if (input.meta && !input.control && code === 'keyx') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            document.dispatchEvent(new KeyboardEvent('keydown', {
              key: 'x', code: 'KeyX', metaKey: true, bubbles: true, cancelable: true
            }));
          `)
        }
        return
      }
      // Super+Z: zen mode toggle — must preventDefault to block browser Undo
      if (input.meta && !input.control && code === 'keyz') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            document.dispatchEvent(new KeyboardEvent('keydown', {
              key: 'z', code: 'KeyZ', metaKey: true, bubbles: true, cancelable: true
            }));
          `)
        }
        return
      }
      // Super+F: toggle maximize width on selected app
      if (input.meta && !input.control && code === 'keyf') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            document.dispatchEvent(new KeyboardEvent('keydown', {
              key: 'f', code: 'KeyF', metaKey: true, bubbles: true, cancelable: true
            }));
          `)
        }
        return
      }
      return
    }

    // Guest contents (regular webviews and Chromium-managed child contents such as
    // PDF/document viewers opened from a webview) should forward app shortcuts to
    // the main host page. These child contents are not always reported as "webview",
    // so do not special-case them as host content.

    // Meta+Ctrl shortcuts: u, i, y (move app / open move-project popup)
    if (input.meta && input.control && ['keyu', 'keyi', 'keyy'].includes(code)) {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      if (mainWindow) {
        if (code === 'keyy') {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenMoveProject) window.__libroOpenMoveProject();
          `)
        } else {
          const direction = code === 'keyu' ? 'left' : 'right'
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroMoveSelectedApp) {
              window.__libroMoveSelectedApp('${direction}');
            } else {
              document.dispatchEvent(new KeyboardEvent('keydown', {
                code: '${input.code || ''}',
                metaKey: true,
                ctrlKey: true,
                bubbles: true,
                cancelable: true
              }));
            }
          `)
        }
      }
      return
    }

    // Meta (Super/Win) shortcuts: h, l, q, n, enter, z, o, f, g, r, x, ;, ,, .
    if (input.meta && !input.control && code === 'keyn') {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      triggerProjectDialogShortcut()
      return
    }
    if (input.meta && !input.control && (code === 'enter' || key === 'enter')) {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      triggerTerminalAppShortcut()
      return
    }
    if (input.meta && !input.control && (['h', 'l', 'q', 'b', 'e', 'z', 'o', 'f', 'g', 'r', 'x', ';', ',', '.'].includes(key) || ['keyo', 'keyb', 'keye', 'semicolon', 'comma', 'period'].includes(code))) {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      if (mainWindow) {
        const safeKey = input.key.replace(/'/g, "\\'")
        const safeCode = (input.code || '').replace(/'/g, "\\'")
        mainWindow.webContents.executeJavaScript(`
          document.dispatchEvent(new KeyboardEvent('keydown', {
            key: '${safeKey}',
            code: '${safeCode}',
            metaKey: true,
            bubbles: true,
            cancelable: true
          }));
        `)
      }
      return
    }

    if (input.meta && input.control && code === 'keyy') {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      if (mainWindow) {
        mainWindow.webContents.executeJavaScript(`
          if (window.__libroOpenMoveProject) window.__libroOpenMoveProject();
        `)
      }
      return
    }

    // Ctrl+0–9 shortcuts (project switching)
    if (input.control && key >= '0' && key <= '9') {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      if (mainWindow) {
        mainWindow.webContents.executeJavaScript(`
          document.dispatchEvent(new KeyboardEvent('keydown', {
            key: '${key}',
            code: '${input.code || ''}',
            ctrlKey: true,
            bubbles: true,
            cancelable: true
          }));
        `)
      }
    }

    // Vim-style mode toggle: 'i' enters insert mode, 'Esc' exits.
    // Source of truth lives here in the main process so the swap happens
    // before any other intercept can fire on the same event loop tick.
    if (isWebview && !input.meta && !input.control && !input.alt && !input.shift) {
      const currentMode = webviewBrowserMode.get(contents.id) || 'normal'
      if (currentMode !== 'insert' && key === 'i' && !webviewInputFocused.get(contents.id)) {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        setWebviewBrowserMode('insert')
        return
      }
      if (currentMode === 'insert' && key === 'escape') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        contents.executeJavaScript(`
          (function(){
            var el = document.activeElement;
            while (el && el.shadowRoot && el.shadowRoot.activeElement) el = el.shadowRoot.activeElement;
            if (el && typeof el.blur === 'function') { try { el.blur(); } catch(err) {} }
          })();
        `).catch(() => {})
        setWebviewBrowserMode('normal')
        return
      }
    }

    // Browser vim-style keys (j/k/h/l/g/G/b/f) for webview scrolling/navigation.
    // Handled here in the main process so they work even while the page is loading,
    // before the guest-injected browserShortcutsScript is active.
    if (isWebview && !input.meta && !input.control && !input.alt && !webviewInputFocused.get(contents.id) && webviewBrowserMode.get(contents.id) !== 'insert') {
      // Shift+G = scroll to bottom
      if (input.shift && key === 'g') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        contents.executeJavaScript("window.scrollTo({top:document.documentElement.scrollHeight,behavior:'smooth'})").catch(() => {})
        return
      }
      const browserKeyActions = {
        'j': "window.scrollBy({top:800,behavior:'smooth'})",
        'k': "window.scrollBy({top:-800,behavior:'smooth'})",
        'h': "window.scrollBy({left:-480,behavior:'smooth'})",
        'l': "window.scrollBy({left:480,behavior:'smooth'})",
        'g': "window.scrollTo({top:0,behavior:'smooth'})",
        'b': "history.back()",
        'f': "history.forward()",
      }
      if (!input.shift && browserKeyActions[key]) {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        contents.executeJavaScript(browserKeyActions[key]).catch(() => {})
        return
      }
      // Bare 'o' opens URL popup, bare 'r' reloads — host-window actions.
      if (!input.shift && (key === 'o' || key === 'r')) {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          const js = key === 'o'
            ? `if (window.__libroOpenURLPopup) window.__libroOpenURLPopup();`
            : `(function(){var a=window.__libroSelectedApp||'';if(a && window.__libroWvReload) window.__libroWvReload(a);})();`
          mainWindow.webContents.executeJavaScript(js).catch(() => {})
        }
        return
      }
    }
  })
})

app.on('ready', async () => {
  Menu.setApplicationMenu(null)
  const alreadyRunning = await isServerRunning()
  if (!alreadyRunning) {
    startGoServer()
    try {
      await waitForServer()
    } catch (e) {
      console.error(`${e.message}; ${serverURL} is not accepting connections`)
      app.quit()
      return
    }
  }
  registerWindowShortcuts()
  createWindow()
  powerMonitor.on('resume', refreshTerminalFramesAfterResume)
})

app.on('before-quit', (e) => {
  if (isQuitting) return
  e.preventDefault()
  showQuitCommandOnlyToast()
})

app.on('window-all-closed', () => {
  if (isQuitting) {
    app.quit()
  }
})

app.on('will-quit', () => {
  globalShortcut.unregisterAll()
  stopGoServer()
})
