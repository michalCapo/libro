const { app, BrowserWindow, Menu, session, ipcMain, shell, clipboard, globalShortcut } = require('electron')
const { spawn } = require('child_process')
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

const port = process.env.LIBRO_PORT || '8100'
const serverURL = `http://localhost:${port}`

let goProcess = null
let mainWindow = null
let isQuitting = false

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

function registerWindowShortcuts() {
  const shortcuts = [
    ['Super+Control+U', () => moveSelectedApp('left')],
    ['Super+Control+I', () => moveSelectedApp('right')],
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
    goProcess = null
  })
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
    webPreferences: {
      webviewTag: true,
      nodeIntegration: false,
      contextIsolation: true,
      preload: path.join(__dirname, 'preload.js'),
    },
  })

  mainWindow.maximize()
  mainWindow.show()

  mainWindow.loadURL(serverURL)

  // Intercept close to show confirmation dialog when apps are running
  mainWindow.on('close', (e) => {
    if (isQuitting) return
    e.preventDefault()
    // Ask the renderer to check for running apps
    mainWindow.webContents.executeJavaScript(`
      if (window.__libroShowCloseDialog) window.__libroShowCloseDialog();
    `)
  })

  // Renderer signals that user confirmed close (or no apps were running)
  ipcMain.on('libro-force-close', () => {
    quitApp().catch((err) => {
      console.error('Failed to quit cleanly:', err.message)
      if (mainWindow && !mainWindow.isDestroyed()) mainWindow.destroy()
    })
  })

  ipcMain.on('libro-toggle-devtools', () => {
    if (mainWindow) mainWindow.webContents.toggleDevTools()
  })

  ipcMain.on('libro-copy-clipboard', (event, text) => {
    if (text) clipboard.writeText(text)
  })

  ipcMain.on('libro-open-path', (event, filePath) => {
    shell.openPath(filePath)
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
    contents.on('console-message', (e, level, message) => {
      if (message === '__libro:inputfocus') webviewInputFocused.set(contents.id, true)
      else if (message === '__libro:inputblur') webviewInputFocused.set(contents.id, false)
    })
    // Reset focus tracking on navigation (old page is unloaded)
    contents.on('did-start-navigation', () => {
      webviewInputFocused.set(contents.id, false)
    })
    contents.on('destroyed', () => {
      webviewInputFocused.delete(contents.id)
    })

    contents.setWindowOpenHandler(({ url, disposition }) => {
      if (url && url !== 'about:blank') {
        // Download links (e.g. <a download>) — trigger download instead of opening a tab
        if (disposition === 'save-to-disk') {
          contents.downloadURL(url)
          return { action: 'deny' }
        }
        if (mainWindow) {
          const safeUrl = url.replace(/\\/g, '\\\\').replace(/'/g, "\\'")
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenNewTab) window.__libroOpenNewTab('${safeUrl}');
          `)
        }
      }
      return { action: 'deny' }
    })
  }

  contents.on('before-input-event', (e, input) => {
    if (input.type !== 'keyDown' && input.type !== 'rawKeyDown') return

    const key = (input.key || '').toLowerCase()
    const code = (input.code || '').toLowerCase()
    const isWebview = contents.getType() === 'webview'
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
        return true
      }
      shortcutEventDedup.set(contents.id, { sig: shortcutSig, at: now })
      return false
    }

    // Win/Super + Plus/Minus: zoom whole application
    if (input.meta && (key === '+' || key === '=' || key === '-')) {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      if (mainWindow) {
        const wc = mainWindow.webContents
        const current = wc.getZoomLevel()
        if (key === '-') {
          wc.setZoomLevel(current - 0.5)
        } else {
          wc.setZoomLevel(current + 0.5)
        }
      }
      return
    }

    // For main window content: directly invoke JS for Super+N and Super+Z shortcuts
    // (native keydown may be intercepted by the desktop environment on Linux)
    if (!isWebview) {
      if (input.meta && input.control && code === 'keyn') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenSearch) window.__libroOpenSearch('left');
          `)
        }
        return
      }
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
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenSearch) window.__libroOpenSearch('right');
          `)
        }
        return
      }
      // Super+R: resize popup — must preventDefault to block browser Reload
      if (input.meta && !input.control && code === 'keyr') {
        if (shouldSkipDuplicateShortcut()) return
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenResizePopup) window.__libroOpenResizePopup();
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

    // Meta+Ctrl shortcuts: u, i (move app left/right)
    if (input.meta && input.control && ['keyu', 'keyi'].includes(code)) {
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

    // Meta (Super/Win) shortcuts: h, l, q, w, n, z, b, f, g, r
    if (input.meta && (['h', 'l', 'q', 'w', 'n', 'z', 'b', 'f', 'g', 'r'].includes(key) || code === 'keyn')) {
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
    }

    // Ctrl+L (select URL bar) and Ctrl+R (reload) for browser apps
    if (input.control && !input.meta && ['l', 'r'].includes(key)) {
      if (shouldSkipDuplicateShortcut()) return
      e.preventDefault()
      if (mainWindow) {
        const safeKey = input.key.replace(/'/g, "\\'")
        const safeCode = (input.code || '').replace(/'/g, "\\'")
        // First try dispatching to document for compatibility
        mainWindow.webContents.executeJavaScript(`
          document.dispatchEvent(new KeyboardEvent('keydown', {
            key: '${safeKey}',
            code: '${safeCode}',
            ctrlKey: true,
            bubbles: true,
            cancelable: true
          }));
        `)
        // Also directly access the selected webview to ensure the action works
        // This handles cases where the main document event doesn't reach handlers
        const reloadOrFocusJS = key === 'r' ? `
          (function() {
            var appId = window.__libroSelectedApp || '';
            if (appId && window.__libroWvReload) {
              window.__libroWvReload(appId);
            }
          })();` : `
          (function() {
            if (window.__libroOpenURLPopup) window.__libroOpenURLPopup();
          })();`
        mainWindow.webContents.executeJavaScript(reloadOrFocusJS)
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

    // Browser vim-style keys (j/k/h/l/g/G/b/f) for webview scrolling/navigation.
    // Handled here in the main process so they work even while the page is loading,
    // before the guest-injected browserShortcutsScript is active.
    if (isWebview && !input.meta && !input.control && !input.alt && !webviewInputFocused.get(contents.id)) {
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
      console.error(e.message)
      app.quit()
      return
    }
  }
  registerWindowShortcuts()
  createWindow()
})

app.on('before-quit', (e) => {
  if (isQuitting) return
  e.preventDefault()
  quitApp().catch((err) => {
    console.error('Failed to quit cleanly:', err.message)
    isQuitting = true
    app.quit()
  })
})

app.on('window-all-closed', () => {
  if (isQuitting) {
    app.quit()
  }
})

app.on('will-quit', () => {
  globalShortcut.unregisterAll()
  if (goProcess) {
    goProcess.kill()
    goProcess = null
  }
})
