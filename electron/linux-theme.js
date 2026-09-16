const { execFile, spawn } = require('node:child_process')
const { createInterface } = require('node:readline')

// Chromium can miss the color-scheme preference on minimal Wayland desktops.
// Feed it through nativeTheme so the existing media-query listener keeps Auto
// up to date, without changing Libro's saved Light/Dark overrides.
async function followLinuxTheme(app, nativeTheme, platform = process.platform) {
  if (platform !== 'linux') return
  const args = ['org.gnome.desktop.interface', 'color-scheme']
  const apply = (value) => {
    const mode = value.trim().replace(/^color-scheme:\s*/, '')
    if (mode === "'prefer-dark'") nativeTheme.themeSource = 'dark'
    else if (mode === "'prefer-light'") nativeTheme.themeSource = 'light'
    else if (mode === "'default'") nativeTheme.themeSource = 'system'
  }

  const monitor = spawn('gsettings', ['monitor', ...args], { stdio: ['ignore', 'pipe', 'ignore'] })
  const lines = createInterface({ input: monitor.stdout })
  let changed = false
  lines.on('line', (line) => {
    changed = true
    apply(line)
  })
  monitor.on('error', () => lines.close()) // Keep Chromium's default if unavailable.
  app.once('will-quit', () => {
    lines.close()
    monitor.kill()
  })

  await new Promise((resolve) => {
    execFile('gsettings', ['get', ...args], { timeout: 2000 }, (err, stdout) => {
      if (!err && !changed) apply(stdout)
      resolve()
    })
  })
}

module.exports = { followLinuxTheme }
