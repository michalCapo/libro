// Preload script — runs in the renderer process with limited Node.js access.
// Webview tags are enabled via webPreferences.webviewTag in main.js.
const { ipcRenderer, contextBridge, webFrame } = require('electron')

// Expose IPC methods to the renderer page for close confirmation flow
contextBridge.exposeInMainWorld('libroElectron', {
  setBrowserControlEnabled: function (enabled) {
    return ipcRenderer.invoke('libro-browser-control-enabled', enabled)
  },
  browserControlState: function (action) {
    return ipcRenderer.invoke('libro-browser-control-state', action)
  },
  onBrowserControlState: function (callback) {
    if (typeof callback === 'function') ipcRenderer.on('libro-browser-control-state', (_event, state) => callback(state))
  },
  capturePageArea: function (webContentsId, area) {
    return ipcRenderer.invoke('libro-capture-page-area', webContentsId, area)
  },
  focusWorkspace: function () {
    return ipcRenderer.invoke('libro-focus-workspace')
  },
  setWorkspaceShortcuts: function (bindings) {
    ipcRenderer.send('libro-workspace-shortcuts', bindings)
  },
  forceClose: function () {
    ipcRenderer.send('libro-force-close')
  },
  toggleDevTools: function () {
    ipcRenderer.send('libro-toggle-devtools')
  },
  toggleMaximize: function () {
    ipcRenderer.send('libro-toggle-maximize')
  },
  openWebviewDevTools: function (webContentsId, bounds, panel) {
    ipcRenderer.send('libro-open-webview-devtools', webContentsId, bounds, panel)
  },
  closeWebviewDevTools: function (webContentsId) {
    ipcRenderer.send('libro-close-webview-devtools', webContentsId)
  },
  inspectWebviewElement: function (webContentsId, bounds, x, y) {
    ipcRenderer.send('libro-inspect-webview-element', webContentsId, bounds, x, y)
  },
  updateWebviewDevToolsBounds: function (webContentsId, bounds) {
    ipcRenderer.send('libro-update-webview-devtools-bounds', webContentsId, bounds)
  },
  getZoomFactor: function () {
    try {
      return webFrame.getZoomFactor()
    } catch (err) {
      return 1
    }
  },
  zoomIn: function () {
    ipcRenderer.send('libro-zoom-in')
  },
  zoomOut: function () {
    ipcRenderer.send('libro-zoom-out')
  },
  zoomReset: function () {
    ipcRenderer.send('libro-zoom-reset')
  },
  copyToClipboard: function (text) {
    ipcRenderer.send('libro-copy-clipboard', text)
  },
  onWebviewDevToolsClosed: function (callback) {
    if (typeof callback !== 'function') return
    ipcRenderer.on('libro-webview-devtools-closed', (_event, webContentsId) => {
      callback(webContentsId)
    })
  }
})
