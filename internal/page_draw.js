// Draws arrows, rectangles and lines on a still picture of a browser page.
// The picture sits exactly over the page, so it looks like drawing on the page.
// Shapes are stored in picture pixels; what the canvas shows is what gets sent.
(() => {
  if (window.__libroPageDraw) return;
  const color = '#e5263d';
  const font = '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
  // One stroke weight for the three tools, drawn to look like what they make.
  const toolIcons = {
    arrow:'<path d="M6 18 18 6M9.5 6H18v8.5"/>',
    rect:'<rect x="4.5" y="6.5" width="15" height="11" rx="1.5"/>',
    line:'<path d="M5.5 18.5l13-13"/>',
  };
  const toolNames = {arrow:'Arrow', rect:'Rectangle', line:'Line'};
  const editors = new Map();
  // Registered when Libro loads, ahead of every workspace shortcut listener, so
  // the editor's own keys such as Ctrl+Enter reach it first.
  window.addEventListener('keydown', event => editors.forEach(editor => editor.keydown(event)), true);

  function segmentDistance(x, y, ax, ay, bx, by) {
    const dx = bx - ax, dy = by - ay;
    const t = dx || dy ? Math.max(0, Math.min(1, ((x - ax) * dx + (y - ay) * dy) / (dx * dx + dy * dy))) : 0;
    return Math.hypot(x - ax - t * dx, y - ay - t * dy);
  }
  function shapeDistance(s, x, y) {
    if (s.type !== 'rect') return segmentDistance(x, y, s.x1, s.y1, s.x2, s.y2);
    return Math.min(segmentDistance(x, y, s.x1, s.y1, s.x2, s.y1), segmentDistance(x, y, s.x2, s.y1, s.x2, s.y2),
      segmentDistance(x, y, s.x2, s.y2, s.x1, s.y2), segmentDistance(x, y, s.x1, s.y2, s.x1, s.y1));
  }
  function wrapText(ctx, text, width) {
    const lines = [];
    for (const paragraph of text.split('\n')) {
      let line = '';
      for (let word of paragraph.split(' ')) {
        const next = line ? line + ' ' + word : word;
        if (line && ctx.measureText(next).width > width) { lines.push(line); line = ''; } else if (line) { line = next; continue; }
        // A word wider than the label, such as a URL, breaks where it must.
        while (ctx.measureText(word).width > width) {
          let fit = word.length - 1;
          while (fit > 1 && ctx.measureText(word.slice(0, fit)).width > width) fit--;
          lines.push(word.slice(0, fit));
          word = word.slice(fit);
        }
        line = word;
      }
      lines.push(line);
    }
    return lines;
  }
  function drawOutline(ctx, s, unit) {
    ctx.beginPath();
    if (s.type === 'rect') { ctx.rect(s.x1, s.y1, s.x2 - s.x1, s.y2 - s.y1); return; }
    ctx.moveTo(s.x1, s.y1);
    if (s.type === 'line') { ctx.lineTo(s.x2, s.y2); return; }
    // The shaft stops inside the head so its square end stays hidden.
    const angle = Math.atan2(s.y2 - s.y1, s.x2 - s.x1), head = 16 * unit;
    ctx.lineTo(s.x2 - Math.cos(angle) * head * .7, s.y2 - Math.sin(angle) * head * .7);
  }
  function drawHead(ctx, s, unit) {
    const angle = Math.atan2(s.y2 - s.y1, s.x2 - s.x1), head = 16 * unit;
    ctx.beginPath();
    ctx.moveTo(s.x2, s.y2);
    ctx.lineTo(s.x2 - head * Math.cos(angle - .45), s.y2 - head * Math.sin(angle - .45));
    ctx.lineTo(s.x2 - head * Math.cos(angle + .45), s.y2 - head * Math.sin(angle + .45));
    ctx.closePath();
  }
  function drawShape(ctx, s, number, unit, selected, showText, width, height) {
    ctx.lineCap = ctx.lineJoin = 'round';
    // A white edge keeps every shape readable on any page colour.
    const pass = (stroke, lineWidth) => {
      ctx.strokeStyle = stroke; ctx.fillStyle = stroke; ctx.lineWidth = lineWidth;
      drawOutline(ctx, s, unit); ctx.stroke();
      if (s.type === 'arrow') { drawHead(ctx, s, unit); ctx.stroke(); ctx.fill(); }
    };
    pass('#fff', (selected ? 7 : 4.5) * unit);
    pass(color, 2.5 * unit);
    const radius = 9 * unit;
    const x = Math.max(radius, Math.min(width - radius, s.type === 'rect' ? Math.min(s.x1, s.x2) : s.x1));
    const y = Math.max(radius, Math.min(height - radius, s.type === 'rect' ? Math.min(s.y1, s.y2) : s.y1));
    ctx.beginPath(); ctx.arc(x, y, radius, 0, 2 * Math.PI);
    ctx.fillStyle = color; ctx.fill();
    ctx.lineWidth = 1.5 * unit; ctx.strokeStyle = '#fff'; ctx.stroke();
    ctx.fillStyle = '#fff'; ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
    ctx.font = '600 ' + 11 * unit + 'px ' + font;
    ctx.fillText(String(number), x, y + .5 * unit);
    if (!showText || !s.text) return;
    // Neutral label, so red only marks the drawings. It sits beside the mark
    // and away from the page area the shape points at.
    ctx.font = 12 * unit + 'px ' + font;
    ctx.textAlign = 'left';
    const lines = wrapText(ctx, s.text, 240 * unit), lineHeight = 17 * unit, padX = 8 * unit, padY = 5 * unit, gap = 5 * unit;
    const boxWidth = Math.max(...lines.map(line => ctx.measureText(line).width)) + 2 * padX;
    const boxHeight = lines.length * lineHeight + 2 * padY;
    let left = x + radius + gap, top = y - boxHeight / 2;
    if (s.type === 'rect') top = y - radius - gap - boxHeight >= 0 ? y - boxHeight - gap : y + radius + gap;
    else if (s.x2 >= s.x1 && x - radius - gap - boxWidth >= 0) left = x - radius - gap - boxWidth;
    left = Math.max(0, Math.min(width - boxWidth, left));
    top = Math.max(0, Math.min(height - boxHeight, top));
    ctx.fillStyle = '#1b1b20';
    ctx.beginPath(); ctx.roundRect(left, top, boxWidth, boxHeight, 6 * unit); ctx.fill();
    ctx.fillStyle = '#fff';
    lines.forEach((line, index) => ctx.fillText(line, left + padX, top + padY + (index + .5) * lineHeight + .5 * unit));
  }
  function promptFor({url, note, width, height, wholePage, annotated, clean, attachments, shapes}) {
    const list = shapes.map((s, index) => (index + 1) + '. ' + toolNames[s.type] + ' from (' + Math.round(s.x1) + ', ' + Math.round(s.y1) + ') to (' + Math.round(s.x2) + ', ' + Math.round(s.y2) + ')' + (s.text ? ': ' + s.text : ''));
    const lines = ['', '', '--- BEGIN Whole page annotation ---'];
    if (note) lines.push('User request: ' + note, '');
    lines.push(note || list.length ? 'Please make the requested change to the whole page below.' : 'Please look at the whole page below.', '', 'Page URL: ' + url);
    if (wholePage) lines.push('Whole-page screenshot: ' + JSON.stringify(wholePage));
    if (list.length) lines.push('Screenshot of the visible part with drawings: ' + JSON.stringify(annotated), 'Clean screenshot of the visible part: ' + JSON.stringify(clean));
    else if (clean) lines.push('Screenshot of the visible part: ' + JSON.stringify(clean));
    lines.push('Open these images and use them with the page URL.');
    attachments.forEach(path => lines.push('Additional image: ' + JSON.stringify(path)));
    if (list.length) lines.push('', 'Drawings, numbered as on the image. Positions are pixels in the ' + width + 'x' + height + ' visible-part images, from the top-left corner:', ...list);
    lines.push('--- END Whole page annotation ---', '', '');
    return lines.join('\n');
  }

  function element(tag, className, parent) {
    const node = document.createElement(tag);
    node.className = className;
    if (parent) parent.appendChild(node);
    return node;
  }
  function iconButton(parent, icon, label, shortcut) {
    const button = element('button', 'ws-page-draw-tool', parent);
    button.type = 'button'; button.title = shortcut ? label + ' (' + shortcut + ')' : label; button.setAttribute('aria-label', label);
    if (toolIcons[icon]) button.innerHTML = '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + toolIcons[icon] + '</svg>';
    else { const glyph = element('i', 'material-icons-round', button); glyph.setAttribute('aria-hidden', 'true'); glyph.textContent = icon; }
    return button;
  }

  async function open({appID, guest, picture, url, wanted, onClose}) {
    const image = new Image();
    image.src = picture;
    // Nothing appears until the picture is ready, so the page never flashes.
    await image.decode();
    if (!wanted() || editors.has(appID)) return false;
    const width = image.naturalWidth, height = image.naturalHeight;
    if (!width || !height) throw new Error('The page picture is empty.');
    const root = element('div', 'ws-page-draw');
    root.tabIndex = -1;
    root.setAttribute('role', 'dialog');
    root.setAttribute('aria-label', 'Draw on page');
    const canvas = element('canvas', 'ws-page-draw-canvas', root);
    canvas.width = width; canvas.height = height;
    // Not marked opaque: the browser would stop painting the covered page, and
    // the full-page capture on Send would then hang.
    const ctx = canvas.getContext('2d');
    const bar = element('div', 'ws-page-draw-bar', root);
    const tools = {arrow:iconButton(bar, 'arrow', 'Arrow', 'A'), rect:iconButton(bar, 'rect', 'Rectangle', 'R'), line:iconButton(bar, 'line', 'Line', 'L')};
    const undoButton = iconButton(bar, 'undo', 'Undo', 'Ctrl+Z');
    const note = element('textarea', 'ws-page-draw-note', bar);
    note.rows = 1; note.placeholder = 'Describe this page (optional)'; note.setAttribute('aria-label', 'Page description');
    const cancelButton = element('button', 'ws-page-draw-action', bar);
    cancelButton.type = 'button'; cancelButton.textContent = 'Cancel';
    const sendButton = element('button', 'ws-page-draw-action ws-page-draw-send', bar);
    sendButton.type = 'button'; sendButton.textContent = 'Send to agent'; sendButton.title = 'Send to agent (Ctrl+Enter)';
    const previews = element('div', 'ws-page-draw-images', bar);
    const box = element('div', 'ws-page-draw-box', root);
    box.hidden = true;
    const input = element('textarea', '', box);
    input.rows = 1; input.placeholder = 'Describe this shape'; input.setAttribute('aria-label', 'Shape description');
    const removeButton = iconButton(box, 'delete', 'Delete shape', 'Delete');

    let shapes = [], history = [], tool = 'arrow', selected = -1, editing = -1, action = null;
    let unit = 1, sending = false, discardTimer = 0, closed = false, images = [], pendingImages = 0;

    function render(forExport) {
      ctx.drawImage(image, 0, 0);
      shapes.forEach((s, index) => drawShape(ctx, s, index + 1, unit, !forExport && index === selected, forExport || index !== editing, width, height));
    }
    function syncControls() {
      Object.entries(tools).forEach(([name, button]) => button.setAttribute('aria-pressed', String(name === tool)));
      undoButton.disabled = !history.length;
      sendButton.disabled = sending || pendingImages > 0;
      sendButton.textContent = sending ? 'Sending…' : 'Send to agent';
      note.readOnly = sending;
    }
    function remember() { history.push(JSON.stringify(shapes)); }
    function layout() {
      const frame = guest.getBoundingClientRect(), host = root.getBoundingClientRect();
      if (!frame.width || !frame.height) return;
      let scale = Math.min(frame.width / width, frame.height / height);
      // One picture pixel per screen pixel keeps page text exactly as sharp.
      const exact = 1 / window.devicePixelRatio;
      if (Math.abs(scale - exact) * Math.max(width, height) < 1.5) scale = exact;
      const shown = {width:width * scale, height:height * scale};
      canvas.style.width = shown.width + 'px';
      canvas.style.height = shown.height + 'px';
      canvas.style.left = frame.left - host.left + (frame.width - shown.width) / 2 + 'px';
      canvas.style.top = frame.top - host.top + (frame.height - shown.height) / 2 + 'px';
      canvas.style.borderRadius = (parseFloat(getComputedStyle(guest).borderTopLeftRadius) || 0) * frame.width / (guest.offsetWidth || frame.width) + 'px';
      unit = 1 / scale;
      render();
      placeBox();
    }
    function placeBox() {
      if (editing < 0) return;
      const s = shapes[editing], area = canvas.getBoundingClientRect(), host = root.getBoundingClientRect();
      const ratio = area.width / width, margin = 8;
      const left = area.left - host.left + Math.min(s.x1, s.x2) * ratio;
      const above = area.top - host.top + Math.min(s.y1, s.y2) * ratio - box.offsetHeight - margin;
      let top = area.top - host.top + Math.max(s.y1, s.y2) * ratio + margin;
      if (top + box.offsetHeight > host.height - margin && above >= margin) top = above;
      box.style.left = Math.max(margin, Math.min(host.width - box.offsetWidth - margin, left)) + 'px';
      box.style.top = Math.max(margin, Math.min(host.height - box.offsetHeight - margin, top)) + 'px';
    }
    function openBox(index) {
      closeBox();
      selected = editing = index;
      input.value = shapes[index].text;
      box.hidden = false;
      placeBox();
      render();
      input.focus();
    }
    function closeBox() {
      if (editing < 0) return;
      const s = shapes[editing], text = input.value.trim();
      if (text !== s.text) { remember(); s.text = text; }
      editing = -1;
      box.hidden = true;
      syncControls();
      render();
      if (root.contains(document.activeElement)) root.focus();
    }
    function undo() {
      if (sending) return;
      closeBox();
      if (!history.length) return;
      shapes = JSON.parse(history.pop());
      selected = -1;
      syncControls();
      render();
      root.focus();
    }
    function removeSelected() {
      if (sending || selected < 0) return;
      closeBox();
      remember();
      shapes.splice(selected, 1);
      selected = -1;
      syncControls();
      render();
      root.focus();
    }
    function close() {
      if (closed) return;
      closed = true;
      clearTimeout(discardTimer);
      editors.delete(appID);
      observer.disconnect();
      guest.removeEventListener('focus', keepFocus);
      const hadFocus = root.contains(document.activeElement);
      root.remove();
      onClose();
      if (hadFocus && window.__libroFocusAppByID) window.__libroFocusAppByID(appID);
    }
    // Throwing work away takes a second press.
    function cancel() {
      if (!root.isConnected) { close(); return; }
      if (sending) return;
      if (!(shapes.length || images.length || note.value.trim()) || discardTimer) { close(); return; }
      cancelButton.textContent = 'Discard?';
      discardTimer = setTimeout(() => { discardTimer = 0; cancelButton.textContent = 'Cancel'; }, 4000);
    }
    async function send() {
      if (sending || closed || pendingImages) return;
      closeBox();
      selected = -1;
      sending = true;
      syncControls();
      render(true);
      const annotated = canvas.toDataURL('image/png');
      render();
      let sent = false;
      try {
        const id = guest.getWebContentsId();
        // Taken now, while the picture hides any movement of the page. A page
        // too large or too slow for it, or one that has since moved to another
        // address, still has the visible part the user saw.
        const whole = guest.getURL() === url ? await Promise.race([window.libroElectron.capturePageArea(id, {fullPage:true}).catch(() => ''), new Promise(done => setTimeout(done, 15000, ''))]) : '';
        const pictures = (shapes.length ? [annotated, picture] : whole ? [] : [picture]).concat(whole ? [whole] : []);
        const paths = await window.__libroSavePageToolImages(appID, pictures);
        const attachments = images.length ? await window.__libroSavePageToolImages(appID, await window.libroElectron.savePageToolImages(id, images.map(image => image.data))) : [];
        if (closed) return;
        const agent = window.__libroPageToolAgent?.(appID) || '';
        const execute = !!window.__libroPageToolsAutoExecute;
        const prompt = promptFor({url, note:note.value.trim(), width, height, shapes, attachments,
          annotated:shapes.length ? paths[0] : '', clean:shapes.length ? paths[1] : whole ? '' : paths[0], wholePage:whole ? paths[paths.length - 1] : ''});
        sent = !!agent && !!window.__libroSendPageToolPrompt?.(prompt, execute, agent);
        window.__libroShowToast?.(sent ? (execute ? 'Prompt sent to agent' : 'Prompt pasted to agent') : 'No active agent panel', sent ? '' : 'Start or select an agent and try again.', sent ? 'success' : 'error');
      } catch (err) {
        if (closed) return;
        window.__libroShowToast?.('Could not save the picture', err.message || 'Try again.', 'error');
      }
      sending = false;
      if (sent) close(); else syncControls();
    }
    note.addEventListener('paste', event => {
      const files = Array.from(event.clipboardData?.items || []).filter(item => item.kind === 'file' && item.type.startsWith('image/')).map(item => item.getAsFile()).filter(Boolean);
      if (!files.length) return;
      event.preventDefault();
      if (sending) return;
      files.forEach(file => {
        if (images.length + pendingImages >= 8 || file.size > 10 * 1024 * 1024) { window.__libroShowToast?.('Image not added', 'Attach up to 8 images, each under 10 MB.', 'error'); return; }
        pendingImages++;
        const reader = new FileReader();
        reader.onload = () => {
          const attachment = {data:String(reader.result)};
          images.push(attachment);
          const preview = element('button', 'ws-page-draw-image', previews);
          preview.type = 'button'; preview.title = 'Remove pasted image'; preview.setAttribute('aria-label', 'Remove pasted image');
          element('img', '', preview).src = attachment.data;
          preview.onclick = () => { images.splice(images.indexOf(attachment), 1); preview.remove(); note.focus(); };
        };
        reader.onloadend = () => { pendingImages--; syncControls(); };
        reader.readAsDataURL(file);
        syncControls();
      });
    });
    function pointer(event) {
      const area = canvas.getBoundingClientRect();
      return {x:Math.max(0, Math.min(width, (event.clientX - area.left) * width / area.width)),
        y:Math.max(0, Math.min(height, (event.clientY - area.top) * height / area.height))};
    }
    // Drawing never asks for text; clicking a shape adds its description.
    canvas.addEventListener('pointerdown', event => {
      if (event.button !== 0 || sending) return;
      event.preventDefault();
      closeBox();
      root.focus();
      const point = pointer(event);
      let hit = -1;
      for (let index = shapes.length - 1; index >= 0 && hit < 0; index--) if (shapeDistance(shapes[index], point.x, point.y) <= 8 * unit) hit = index;
      canvas.setPointerCapture?.(event.pointerId);
      if (hit >= 0) {
        selected = hit;
        action = {move:true, start:point, before:JSON.stringify(shapes), moved:false, base:{...shapes[hit]}};
      } else {
        selected = -1;
        action = {draft:{type:tool, x1:point.x, y1:point.y, x2:point.x, y2:point.y, text:''}};
      }
      render();
    });
    canvas.addEventListener('pointermove', event => {
      const point = pointer(event);
      if (!action) {
        canvas.style.cursor = shapes.some(s => shapeDistance(s, point.x, point.y) <= 8 * unit) ? 'move' : '';
        return;
      }
      if (action.draft) {
        action.draft.x2 = point.x; action.draft.y2 = point.y;
        render();
        drawShape(ctx, action.draft, shapes.length + 1, unit, false, false, width, height);
        return;
      }
      const dx = point.x - action.start.x, dy = point.y - action.start.y;
      if (!action.moved && Math.hypot(dx, dy) < 4 * unit) return;
      action.moved = true;
      // Whole shape stays inside the picture.
      const b = action.base, s = shapes[selected];
      const shiftX = Math.max(-Math.min(b.x1, b.x2), Math.min(width - Math.max(b.x1, b.x2), dx));
      const shiftY = Math.max(-Math.min(b.y1, b.y2), Math.min(height - Math.max(b.y1, b.y2), dy));
      s.x1 = b.x1 + shiftX; s.x2 = b.x2 + shiftX; s.y1 = b.y1 + shiftY; s.y2 = b.y2 + shiftY;
      render();
    });
    function finishPointer(event, cancelled) {
      const current = action;
      action = null;
      if (!current) return;
      if (current.draft) {
        const d = current.draft;
        if (cancelled || Math.hypot(d.x2 - d.x1, d.y2 - d.y1) < 6 * unit) { render(); return; }
        remember();
        shapes.push(d);
        selected = shapes.length - 1;
        syncControls();
        render();
        return;
      }
      if (current.moved) { history.push(current.before); syncControls(); render(); return; }
      if (!cancelled) openBox(selected);
    }
    canvas.addEventListener('pointerup', event => finishPointer(event, false));
    canvas.addEventListener('pointercancel', event => finishPointer(event, true));
    Object.entries(tools).forEach(([name, button]) => { button.onclick = () => { tool = name; closeBox(); syncControls(); root.focus(); }; });
    undoButton.onclick = undo;
    cancelButton.onclick = cancel;
    sendButton.onclick = send;
    removeButton.onclick = removeSelected;

    // Plain keys stay in the editor; other shortcuts keep working.
    function keydown(event) {
      if (!root.isConnected) { close(); return; }
      if (!root.contains(event.target) || event.isComposing) return;
      const typing = event.target === input || event.target === note;
      const command = event.ctrlKey || event.metaKey;
      const stop = () => { event.preventDefault(); event.stopImmediatePropagation(); };
      if (command && event.key === 'Enter') { stop(); send(); return; }
      if (command && !typing && event.key.toLowerCase() === 'z') { stop(); undo(); return; }
      if (event.ctrlKey && typing && event.key.toLowerCase() === 'j') { stop(); event.target.setRangeText('\n', event.target.selectionStart, event.target.selectionEnd, 'end'); return; }
      if (command || event.altKey) return;
      event.stopImmediatePropagation();
      if (event.key === 'Escape') {
        event.preventDefault();
        if (editing >= 0) closeBox();
        else if (selected >= 0) { selected = -1; render(); }
        else cancel();
        return;
      }
      if (event.key === 'Enter' && typing && !event.shiftKey) {
        event.preventDefault();
        if (event.target === input) closeBox(); else send();
        return;
      }
      if (typing) return;
      if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); removeSelected(); return; }
      if (event.key === 'p') { event.preventDefault(); cancel(); return; }
      const next = {a:'arrow', r:'rect', l:'line'}[event.key];
      if (next) { event.preventDefault(); tool = next; syncControls(); }
    }
    // The page behind must not take the keyboard while the picture covers it.
    let lastFocus = note;
    root.addEventListener('focusin', event => { lastFocus = event.target; });
    function keepFocus() { (lastFocus.isConnected && !lastFocus.closest('[hidden]') ? lastFocus : root).focus(); }

    const observer = new ResizeObserver(() => requestAnimationFrame(() => { if (!closed) layout(); }));
    editors.set(appID, {cancel, close, keydown});
    render();
    syncControls();
    guest.parentElement.appendChild(root);
    layout();
    observer.observe(guest);
    observer.observe(guest.parentElement);
    guest.addEventListener('focus', keepFocus);
    note.focus();
    return true;
  }

  window.__libroPageDraw = {open, cancel:appID => editors.get(appID)?.cancel(), close:appID => editors.get(appID)?.close()};
})();
