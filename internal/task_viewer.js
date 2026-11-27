// Fits a saved task's images to the panel and opens a large one in a viewer:
// the wheel zooms at the pointer, dragging moves it, a click beside it or
// Escape closes it. Ctrl or Cmd+click follows an image's link. thread_task.go
// allows only this script by its hash.
(function () {
  var style = document.createElement('style')
  style.textContent =
    // No height:auto: a spacer image would grow to its width.
    'img{max-width:100%!important;max-height:100vh!important;object-fit:contain}' +
    'img.libro-zoom{cursor:zoom-in}' +
    '#libro-viewer{position:fixed;inset:0;z-index:2147483647;background:rgba(0,0,0,.85);overflow:hidden;cursor:grab;touch-action:none}' +
    '#libro-viewer.libro-drag{cursor:grabbing}' +
    '#libro-viewer img{position:absolute;left:0;top:0;max-width:none!important;max-height:none!important;transform-origin:0 0;user-select:none;-webkit-user-drag:none}'
  document.head.appendChild(style)

  var viewer, image, width, height, fit, scale, x, y, drag

  // Large images, or ones shown smaller than they are, can be zoomed; icons not.
  function zoomable(el) {
    var shrunk = el.naturalWidth > el.clientWidth + 1 || el.naturalHeight > el.clientHeight + 1
    var big = Math.max(el.clientWidth, el.clientHeight) >= 160 && Math.min(el.clientWidth, el.clientHeight) >= 32
    return big || shrunk && Math.max(el.naturalWidth, el.naturalHeight) >= 100 && Math.min(el.clientWidth, el.clientHeight) >= 16
  }

  function mark() {
    document.querySelectorAll('img').forEach(function (el) {
      if (!viewer || el !== image) el.classList.toggle('libro-zoom', el.naturalWidth > 0 && zoomable(el))
    })
  }

  function show() {
    image.style.transform = 'translate(' + x + 'px,' + y + 'px) scale(' + scale + ')'
  }

  function fitScale() {
    return Math.min(1, (innerWidth - 32) / width, (innerHeight - 32) / height)
  }

  function reset() {
    fit = fitScale()
    scale = fit
    x = (innerWidth - width * scale) / 2
    y = (innerHeight - height * scale) / 2
    show()
  }

  function zoom(to, cx, cy) {
    to = Math.min(Math.max(to, fit / 2), Math.max(fit * 20, 8))
    x = cx - (cx - x) * to / scale
    y = cy - (cy - y) * to / scale
    scale = to
    show()
  }

  function close() {
    if (!viewer) return
    viewer.remove()
    viewer = image = drag = null
  }

  function open(el) {
    width = el.naturalWidth
    height = el.naturalHeight
    viewer = document.createElement('div')
    viewer.id = 'libro-viewer'
    image = document.createElement('img')
    image.src = el.currentSrc || el.src
    image.alt = el.alt
    image.style.width = width + 'px'
    image.style.height = height + 'px'
    viewer.appendChild(image)
    document.body.appendChild(viewer)
    reset()
    viewer.addEventListener('wheel', function (e) {
      e.preventDefault()
      var delta = e.deltaY * (e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? innerHeight : 1)
      zoom(scale * Math.exp(-delta * 0.002), e.clientX, e.clientY)
    }, { passive: false })
    viewer.addEventListener('dblclick', function (e) {
      if (e.target !== image) return
      if (scale !== fit) reset()
      else zoom(Math.max(1, fit * 2), e.clientX, e.clientY)
    })
    viewer.addEventListener('pointerdown', function (e) {
      if (e.button !== 0 || drag) return
      drag = { id: e.pointerId, x: e.clientX, y: e.clientY, left: x, top: y, target: e.target, moved: false }
    })
    viewer.addEventListener('pointermove', function (e) {
      if (!drag || e.pointerId !== drag.id) return
      var dx = e.clientX - drag.x, dy = e.clientY - drag.y
      // Capture starts with the drag, so a click or double click keeps its target.
      if (!drag.moved && Math.abs(dx) + Math.abs(dy) > 3) {
        drag.moved = true
        viewer.setPointerCapture(e.pointerId)
        viewer.classList.add('libro-drag')
      }
      if (!drag.moved) return
      x = drag.left + dx
      y = drag.top + dy
      show()
    })
    var release = function (e) {
      if (!drag || e.pointerId !== drag.id) return
      var last = drag
      drag = null
      viewer.classList.remove('libro-drag')
      if (e.type === 'pointerup' && !last.moved && last.target !== image) close()
    }
    viewer.addEventListener('pointerup', release)
    viewer.addEventListener('pointercancel', release)
  }

  document.addEventListener('click', function (e) {
    var el = e.target
    if (viewer || !el || el.tagName !== 'IMG' || !el.classList.contains('libro-zoom') || e.ctrlKey || e.metaKey || e.shiftKey) return
    e.preventDefault()
    open(el)
  }, true)
  document.addEventListener('keydown', function (e) {
    if (viewer && e.key === 'Escape') close()
  })
  // A resize keeps the zoom the user chose and refits a fitted image.
  addEventListener('resize', function () {
    if (viewer && scale === fit) reset()
    else if (viewer) fit = fitScale()
    mark()
  })
  addEventListener('load', mark)
  document.addEventListener('load', function (e) { if (e.target.tagName === 'IMG') mark() }, true)
})()
