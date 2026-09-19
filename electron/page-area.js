const fs = require('node:fs/promises')
const path = require('node:path')

async function capturePageArea(target, area, tempDir) {
  if (!area || !['x', 'y', 'width', 'height'].every(key => Number.isFinite(area[key])) || area.width <= 0 || area.height <= 0) {
    throw new Error('Invalid selected area')
  }
  const zoom = target.getZoomFactor()
  const image = await target.capturePage()
  const size = image.getSize()
  const x = Math.max(0, Math.floor(area.x * zoom))
  const y = Math.max(0, Math.floor(area.y * zoom))
  const right = Math.min(size.width, Math.ceil((area.x + area.width) * zoom))
  const bottom = Math.min(size.height, Math.ceil((area.y + area.height) * zoom))
  if (right <= x || bottom <= y || image.isEmpty()) throw new Error('Selected area is outside the page')
  const png = image.crop({ x, y, width: right - x, height: bottom - y }).toPNG()
  const dir = await fs.mkdtemp(path.join(tempDir, 'libro-page-area-'))
  const filename = path.join(dir, 'selection.png')
  await fs.writeFile(filename, png, { mode: 0o600 })
  return filename
}

module.exports = { capturePageArea }
