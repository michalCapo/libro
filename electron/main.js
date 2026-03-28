const { app, BrowserWindow, Menu, session } = require('electron')
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

  mainWindow.on('closed', () => {
    mainWindow = null
    app.quit()
  })
}

app.on('ready', async () => {
  Menu.setApplicationMenu(null)
  startGoServer()
  try {
    await waitForServer()
  } catch (e) {
    console.error(e.message)
    app.quit()
    return
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
