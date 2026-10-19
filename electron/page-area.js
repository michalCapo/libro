async function capturePageArea(target, area) {
  let png
  if (area?.fullPage === true) {
    const ownsDebugger = !target.debugger.isAttached()
    if (ownsDebugger) target.debugger.attach('1.3')
    try {
      // contentSize is in device pixels and the clip in zoomed CSS pixels;
      // cssContentSize is rounded, so derive both from devicePixelRatio.
      const {contentSize: content} = await target.debugger.sendCommand('Page.getLayoutMetrics')
      const {result: {value: ratio}} = await target.debugger.sendCommand('Runtime.evaluate', {expression:'devicePixelRatio', returnByValue:true})
      if (!Number.isFinite(ratio) || ratio <= 0) throw new Error('Invalid page pixel ratio')
      const width = Math.round(content.width / ratio), height = Math.round(content.height / ratio)
      if (width <= 0 || height <= 0 || width * height > 24000000 || width > 16000 || height > 16000) {
        throw new Error('Page exceeds 24 megapixels or 16000 pixels per side; select a smaller area')
      }
      const zoom = target.getZoomFactor() / ratio
      const clip = {x:content.x * zoom, y:content.y * zoom, width:content.width * zoom, height:content.height * zoom, scale:1}
      const result = await target.debugger.sendCommand('Page.captureScreenshot', {format:'png', captureBeyondViewport:true, clip})
      const image = require('electron').nativeImage.createFromBuffer(Buffer.from(result.data, 'base64'))
      png = image.resize({width, height}).toPNG({scaleFactor:1})
    } finally {
      if (ownsDebugger && !target.isDestroyed() && target.debugger.isAttached()) target.debugger.detach()
    }
  } else {
    if (!area || !['x', 'y', 'width', 'height', 'pixelRatio'].every(key => Number.isFinite(area[key])) || area.width <= 0 || area.height <= 0 || area.pixelRatio <= 0) {
      throw new Error('Invalid selected area')
    }
    // The capture is in device pixels: CSS pixels times the page's
    // devicePixelRatio, which covers both page zoom and display scaling.
    const image = await target.capturePage()
    const size = image.getSize()
    const scale = area.pixelRatio
    const x = Math.max(0, Math.round(area.x * scale))
    const y = Math.max(0, Math.round(area.y * scale))
    const right = Math.min(size.width, Math.round((area.x + area.width) * scale))
    const bottom = Math.min(size.height, Math.round((area.y + area.height) * scale))
    if (right <= x || bottom <= y || image.isEmpty()) throw new Error('Selected area is outside the page')
    png = image.crop({ x, y, width: right - x, height: bottom - y }).toPNG()
  }
  return 'data:image/png;base64,' + png.toString('base64')
}

function pageToolImages(images) {
  if (!Array.isArray(images) || images.length > 8) throw new Error('Invalid image attachments')
  return images.map(data => {
    if (typeof data !== 'string' || data.length > 14 * 1024 * 1024 || !/^data:image\/(png|jpeg|gif|webp);base64,/.test(data)) throw new Error('Invalid image attachment')
    const image = require('electron').nativeImage.createFromDataURL(data)
    if (image.isEmpty()) throw new Error('Invalid image attachment')
    return 'data:image/png;base64,' + image.toPNG().toString('base64')
  })
}

module.exports = { capturePageArea, pageToolImages }
