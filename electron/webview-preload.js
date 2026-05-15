// Preload for pages loaded inside Libro's persistent webview session.
// Keep this file small and avoid touching storage APIs: Discord is sensitive to
// early page-environment checks during auth/session restore.
const { contextBridge } = require('electron')

try {
  contextBridge.executeInMainWorld({
    func: () => {
      const hostname = window.location && window.location.hostname
      const isDiscord = hostname === 'discord.com' ||
        hostname.endsWith('.discord.com') ||
        hostname === 'discordapp.com' ||
        hostname.endsWith('.discordapp.com')

      if (!isDiscord) return

      // Discord's auth/session logic has historically treated differences
      // between outer* and inner* dimensions as DevTools/sidebar embedding and
      // invalidated the local auth token. In a Libro panel, the outer window is
      // the whole Electron window while the inner size is the webview, so make
      // Discord see normal browser-like dimensions.
      Object.defineProperty(window, 'outerWidth', {
        configurable: true,
        get: () => window.innerWidth,
      })
      Object.defineProperty(window, 'outerHeight', {
        configurable: true,
        get: () => window.innerHeight,
      })
    },
  })
} catch (err) {
  // Best-effort site quirk only.
}
