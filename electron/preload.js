// Preload script — runs in the renderer process with limited Node.js access.
// Webview tags are enabled via webPreferences.webviewTag in main.js.
const { ipcRenderer, contextBridge } = require('electron')

// Expose IPC methods to the renderer page for close confirmation flow
contextBridge.exposeInMainWorld('libroElectron', {
  forceClose: function () {
    ipcRenderer.send('libro-force-close')
  },
  toggleDevTools: function () {
    ipcRenderer.send('libro-toggle-devtools')
  },
  toggleWebviewDevTools: function (webContentsId, bounds, panel) {
    ipcRenderer.send('libro-toggle-webview-devtools', webContentsId, bounds, panel)
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
  openPath: function (filePath) {
    ipcRenderer.send('libro-open-path', filePath)
  },
  cancelDownload: function (id) {
    ipcRenderer.send('libro-cancel-download', id)
  },
  copyToClipboard: function (text) {
    ipcRenderer.send('libro-copy-clipboard', text)
  }
})
