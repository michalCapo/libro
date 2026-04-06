const { app, BrowserWindow, Menu, session, ipcMain } = require('electron')
const { spawn } = require('child_process')
const path = require('path')
const http = require('http')
const os = require('os')

const port = process.env.LIBRO_PORT || '8100'
const serverURL = `http://localhost:${port}`

let goProcess = null
let mainWindow = null

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

function createWindow() {
  // Persistent partition for webview sessions (shared cookies across webviews)
  session.fromPartition('persist:libro')

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
    e.preventDefault()
    // Ask the renderer to check for running apps
    mainWindow.webContents.executeJavaScript(`
      if (window.__libroShowCloseDialog) window.__libroShowCloseDialog();
    `)
  })

  // Renderer signals that user confirmed close (or no apps were running)
  ipcMain.on('libro-force-close', () => {
    if (mainWindow) mainWindow.destroy()
  })

  ipcMain.on('libro-toggle-devtools', () => {
    if (mainWindow) mainWindow.webContents.toggleDevTools()
  })

  ipcMain.on('libro-set-fullscreen', (event, flag) => {
    if (mainWindow) mainWindow.setFullScreen(!!flag)
  })

  mainWindow.on('closed', () => {
    mainWindow = null
    app.quit()
  })
}

// Intercept keyboard shortcuts from webview guest pages in the main process.
// The renderer-side <webview> before-input-event is unreliable — this catches
// input at the webContents level before the guest page can consume it.
app.on('web-contents-created', (event, contents) => {
  contents.on('before-input-event', (e, input) => {
    if (input.type !== 'keyDown') return

    const key = (input.key || '').toLowerCase()
    const code = (input.code || '').toLowerCase()
    const isWebview = contents.getType() === 'webview'

    // Win/Super + Plus/Minus: zoom whole application
    if (input.meta && (key === '+' || key === '=' || key === '-')) {
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
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenSearch) window.__libroOpenSearch('left');
          `)
        }
        return
      }
      if (input.meta && !input.control && code === 'keyn') {
        e.preventDefault()
        if (mainWindow) {
          mainWindow.webContents.executeJavaScript(`
            if (window.__libroOpenSearch) window.__libroOpenSearch('right');
          `)
        }
        return
      }
      // Super+Z: zen mode toggle — must preventDefault to block browser Undo
      if (input.meta && !input.control && code === 'keyz') {
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
      return
    }

    // Meta+Ctrl shortcuts: h, l (move app left/right), n (new app left)
    if (input.meta && input.control && ['keyh', 'keyl', 'keyn'].includes(code)) {
      e.preventDefault()
      if (mainWindow) {
        const safeCode = (input.code || '').replace(/'/g, "\\'")
        mainWindow.webContents.executeJavaScript(`
          document.dispatchEvent(new KeyboardEvent('keydown', {
            code: '${safeCode}',
            metaKey: true,
            ctrlKey: true,
            bubbles: true,
            cancelable: true
          }));
        `)
      }
      return
    }

    // Meta (Super/Win) shortcuts: h, l, d, n, z
    if (input.meta && (['h', 'l', 'd', 'n', 'z'].includes(key) || code === 'keyn')) {
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
      e.preventDefault()
      if (mainWindow) {
        const safeKey = input.key.replace(/'/g, "\\'")
        const safeCode = (input.code || '').replace(/'/g, "\\'")
        mainWindow.webContents.executeJavaScript(`
          document.dispatchEvent(new KeyboardEvent('keydown', {
            key: '${safeKey}',
            code: '${safeCode}',
            ctrlKey: true,
            bubbles: true,
            cancelable: true
          }));
        `)
      }
      return
    }

    // Ctrl+0–9 shortcuts (project switching)
    if (input.control && key >= '0' && key <= '9') {
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
  createWindow()
})

app.on('window-all-closed', () => {
  app.quit()
})

app.on('will-quit', () => {
  if (goProcess) {
    goProcess.kill()
    goProcess = null
  }
})
