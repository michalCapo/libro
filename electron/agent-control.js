const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')

function createController(getWindow) {
  let queue = Promise.resolve()
  return command => {
    const run = queue.then(async () => {
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
        throw new Error('Unknown Libro action')
      }
      try {
        return await Promise.race([
          execute(),
          new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Libro command timed out')), 25000) }),
        ])
      } finally { clearTimeout(timer) }
    })
    queue = run.catch(() => {})
    return run
  }
}

async function startControlServer(directory, instance, dispatch) {
  const token = crypto.randomBytes(32).toString('hex')
  const descriptor = path.join(directory, `desktop-control-${instance}.json`)
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
        res.end(JSON.stringify({result:await dispatch(command)}))
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

module.exports = { createController, startControlServer }
