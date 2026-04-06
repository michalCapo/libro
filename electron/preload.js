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
  setFullScreen: function (flag) {
    ipcRenderer.send('libro-set-fullscreen', !!flag)
  }
})
