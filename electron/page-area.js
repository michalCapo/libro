const fs = require('node:fs/promises')
const path = require('node:path')

async function capturePageArea(target, area, tempDir) {
  let png
  if (area?.fullPage === true) {
    const ownsDebugger = !target.debugger.isAttached()
    if (ownsDebugger) target.debugger.attach('1.3')
    try {
      const {cssContentSize: clip} = await target.debugger.sendCommand('Page.getLayoutMetrics')
      if (clip.width <= 0 || clip.height <= 0 || clip.width * clip.height > 24000000 || clip.width > 16000 || clip.height > 16000) {
        throw new Error('Page exceeds 24 megapixels or 16000 pixels per side; select a smaller area')
      }
      const result = await target.debugger.sendCommand('Page.captureScreenshot', {format:'png', captureBeyondViewport:true, clip:{...clip, scale:1}})
      const image = require('electron').nativeImage.createFromBuffer(Buffer.from(result.data, 'base64'))
      png = image.resize({width:Math.ceil(clip.width), height:Math.ceil(clip.height)}).toPNG({scaleFactor:1})
    } finally {
      if (ownsDebugger && !target.isDestroyed() && target.debugger.isAttached()) target.debugger.detach()
    }
  } else {
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
    png = image.crop({ x, y, width: right - x, height: bottom - y }).toPNG()
  }
  const dir = await fs.mkdtemp(path.join(tempDir, 'libro-page-area-'))
  const filename = path.join(dir, 'selection.png')
  await fs.writeFile(filename, png, { mode: 0o600 })
  return filename
}

async function savePageToolImages(images, tempDir) {
  if (!Array.isArray(images) || images.length > 8) throw new Error('Invalid image attachments')
  const buffers = images.map(data => {
    if (typeof data !== 'string' || data.length > 14 * 1024 * 1024 || !/^data:image\/(png|jpeg|gif|webp);base64,/.test(data)) throw new Error('Invalid image attachment')
    const image = require('electron').nativeImage.createFromDataURL(data)
    if (image.isEmpty()) throw new Error('Invalid image attachment')
    return image.toPNG()
  })
  const dir = await fs.mkdtemp(path.join(tempDir, 'libro-prompt-images-'))
  try {
    const filenames = []
    for (const [index, png] of buffers.entries()) {
      const filename = path.join(dir, `image-${index + 1}.png`)
      await fs.writeFile(filename, png, { mode: 0o600 })
      filenames.push(filename)
    }
    return filenames
  } catch (error) {
    await fs.rm(dir, { recursive: true, force: true })
    throw error
  }
}

module.exports = { capturePageArea, savePageToolImages }
