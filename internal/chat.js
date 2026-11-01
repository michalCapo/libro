// Chat panels: the message box with pasted images, answers to Claude's
// questions, and the agent state for the sidebar. The server draws the
// timeline and updates it live; this script only sends input and keeps the
// view at the bottom.
(function () {
  if (window.libroChat) return;
  const chats = new Map();
  const appOf = el => el.dataset.chat;
  const sidOf = el => el.dataset.sid || window.__libroWorkspaceSID || '';
  const send = (el, action, data) => window.__ws?.call(action, {sid: sidOf(el), appId: appOf(el), ...data});
  const maxImages = 8;
  const maxSide = 2000;

  // Images become PNGs no larger than maxSide, so Claude can read them inline.
  async function toPNG(file) {
    const bitmap = await createImageBitmap(file);
    const scale = Math.min(1, maxSide / Math.max(bitmap.width, bitmap.height));
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(bitmap.width * scale));
    canvas.height = Math.max(1, Math.round(bitmap.height * scale));
    canvas.getContext('2d').drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close();
    return new Promise((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error('Could not read the image.')), 'image/png'));
  }

  async function upload(el, blobs) {
    const form = new FormData();
    blobs.forEach(blob => form.append('images', blob, 'image.png'));
    const query = new URLSearchParams({sid: sidOf(el), id: appOf(el)});
    const response = await fetch('/attachments?' + query, {method: 'POST', body: form});
    if (!response.ok) throw new Error((await response.text()).trim() || 'Could not add the images.');
    return (await response.json()).paths;
  }

  function showError(chat, text) { chat.error.textContent = text || ''; }

  function renderTray(chat) {
    chat.tray.replaceChildren(...chat.images.map((image, index) => {
      const item = document.createElement('div');
      item.className = 'group/image relative h-16 w-16 shrink-0 overflow-hidden rounded-xl bg-(--ws-sunken) ring-1 ring-(--ws-line)';
      const img = document.createElement('img');
      img.className = 'h-full w-full object-cover';
      img.src = image.url; img.alt = `Image ${index + 1}`;
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'absolute right-1 top-1 flex h-5 w-5 cursor-pointer items-center justify-center rounded-full border-0 bg-black/60 p-0 text-white hover:bg-black/80';
      remove.title = remove.ariaLabel = `Remove image ${index + 1}`;
      remove.innerHTML = '<i class="material-icons-round text-[14px] leading-none" aria-hidden="true">close</i>';
      remove.addEventListener('click', () => {
        URL.revokeObjectURL(image.url);
        chat.images.splice(chat.images.indexOf(image), 1);
        renderTray(chat);
        chat.field.focus();
      });
      item.append(img, remove);
      return item;
    }));
  }

  async function addImages(chat, files) {
    files = files.filter(file => file?.type.startsWith('image/'));
    if (!files.length) return false;
    showError(chat, '');
    if (chat.images.length + files.length > maxImages) showError(chat, `Add at most ${maxImages} images to one message.`);
    for (const file of files.slice(0, maxImages - chat.images.length)) {
      try {
        const blob = await toPNG(file);
        if (chat.closed) return false;
        if (chat.images.length >= maxImages) break;
        chat.images.push({blob, url: URL.createObjectURL(blob)});
      } catch { showError(chat, 'Could not read one of the images.'); }
    }
    renderTray(chat);
    return true;
  }

  async function submit(el, chat) {
    const draft = chat.field.value, text = draft.trim(), sent = chat.images.slice();
    if ((!text && !sent.length) || chat.busy) return;
    chat.busy = true; chat.send.disabled = true;
    showError(chat, '');
    try {
      const images = sent.length ? await upload(el, sent.map(image => image.blob)) : [];
      send(el, 'chat.send', {text, images});
      // Only what was sent goes: text and images added during the upload stay.
      sent.forEach(image => URL.revokeObjectURL(image.url));
      chat.images = chat.images.filter(image => !sent.includes(image)); renderTray(chat);
      chat.field.value = chat.field.value.startsWith(draft) ? chat.field.value.slice(draft.length).trimStart() : chat.field.value;
      chat.stick = true; chat.scroll.scrollTop = chat.scroll.scrollHeight;
    } catch (error) {
      showError(chat, error.message);
    } finally {
      chat.busy = false; chat.send.disabled = false;
    }
  }

  // Each card collects its own fields: chosen options, own answers and a
  // reply that declines with a reason.
  function answer(el, card, kind) {
    const request = card.dataset.chatRequest;
    const message = card.querySelector('input[name=message]')?.value.trim() || '';
    if (kind === 'reply' && !message) { card.querySelector('input[name=message]')?.focus(); return; }
    const data = {request, allow: kind === 'allow' || kind === 'questions', message};
    if (kind === 'questions') {
      const answers = {};
      for (const [i, fieldset] of [...card.querySelectorAll('[data-question]')].entries()) {
        const chosen = [...fieldset.querySelectorAll(`input[name=q${i}]:checked`)].map(input => input.value);
        const own = fieldset.querySelector(`input[name=q${i}-other]`)?.value.trim();
        if (own) chosen.push(own);
        if (!chosen.length) { fieldset.querySelector('input')?.focus(); return; }
        answers[fieldset.dataset.question] = chosen.join(', ');
      }
      data.answers = answers;
    }
    card.querySelectorAll('button, input').forEach(control => { control.disabled = true; });
    send(el, 'chat.answer', data);
  }

  function atBottom(scroll) { return scroll.scrollHeight - scroll.scrollTop - scroll.clientHeight < 4; }

  function mount(el) {
    if (chats.has(el)) return;
    const chat = {
      stick: true, ws: null, attempts: 0, timer: null, closed: false, busy: false, images: [], drag: 0,
      scroll: el.querySelector('[data-chat-scroll]'), field: el.querySelector('[data-chat-input]'), form: el.querySelector('[data-chat-form]'),
      tray: el.querySelector('[data-chat-tray]'), error: el.querySelector('[data-chat-error]'), send: el.querySelector('[data-chat-send]'),
      box: el.querySelector('[data-chat-box]'),
    };
    const {scroll, field, form, box} = chat;
    chats.set(el, chat);
    // The chat follows new content while the reader is at the bottom. The
    // reader's own scrolling up stops it at once, before new content can pull
    // the chat back down; scrolling down to the very end starts it again.
    // Scroll events alone can't tell: one that comes after more content was
    // added looks like scrolling up and would leave a fast reply behind.
    const follow = () => { if (chat.stick && !chat.held) scroll.scrollTop = scroll.scrollHeight; };
    const leave = () => { chat.stick = false; };
    scroll.scrollTop = scroll.scrollHeight;
    let last = scroll.scrollTop;
    scroll.addEventListener('scroll', () => {
      const top = scroll.scrollTop;
      if (top > last && atBottom(scroll)) chat.stick = true;
      else if (top < last && !atBottom(scroll)) leave();
      last = top;
    }, {passive: true});
    scroll.addEventListener('wheel', event => { if (event.deltaY < 0) leave(); }, {passive: true});
    scroll.addEventListener('keydown', event => {
      if (['ArrowUp', 'PageUp', 'Home'].includes(event.key) && !event.target.closest('input,textarea')) leave();
    });
    // A finger or the scroll bar holds the chat still until let go. A touch
    // that scrolls cancels its pointer, so touches end only with touchend.
    const hold = kind => { chat.held = kind; leave(); };
    const release = event => {
      if (chat.held !== (event.type.startsWith('touch') ? 'touch' : 'pointer')) return;
      chat.held = false; chat.stick = atBottom(scroll);
    };
    scroll.addEventListener('pointerdown', event => { if (event.pointerType === 'mouse' && event.target === scroll) hold('pointer'); });
    scroll.addEventListener('touchstart', () => hold('touch'), {passive: true});
    chat.release = release;
    for (const type of ['pointerup', 'touchend', 'pointercancel', 'touchcancel']) window.addEventListener(type, release);
    chat.observer = new MutationObserver(follow);
    chat.observer.observe(scroll, {childList: true, subtree: true, characterData: true});
    // A taller message box, a resized panel or a loaded picture moves the end too.
    chat.resize = new ResizeObserver(follow);
    chat.resize.observe(scroll);
    scroll.addEventListener('load', follow, true);
    form.addEventListener('submit', event => { event.preventDefault(); submit(el, chat); });
    field.addEventListener('keydown', event => {
      // Esc stops Claude at once while it works, like the stop button.
      if (event.key === 'Escape' && !event.isComposing && el.querySelector('[data-chat-stop]')) {
        event.preventDefault(); event.stopPropagation();
        send(el, 'chat.stop', {});
        return;
      }
      if (event.key !== 'Enter' || event.shiftKey || event.isComposing || event.keyCode === 229) return;
      event.preventDefault();
      form.requestSubmit();
    });
    field.addEventListener('paste', event => {
      const files = [...event.clipboardData?.items || []].filter(item => item.kind === 'file').map(item => item.getAsFile());
      if (files.some(file => file?.type.startsWith('image/'))) { event.preventDefault(); addImages(chat, files); }
    });
    el.querySelector('[data-chat-pick]').addEventListener('change', event => {
      addImages(chat, [...event.target.files]);
      event.target.value = '';
      field.focus();
    });
    const files = event => [...event.dataTransfer?.items || []].some(item => item.kind === 'file');
    box.addEventListener('dragenter', event => { if (files(event)) { chat.drag++; box.dataset.drop = ''; } });
    box.addEventListener('dragleave', () => { if (--chat.drag <= 0) { chat.drag = 0; delete box.dataset.drop; } });
    box.addEventListener('dragover', event => { if (files(event)) event.preventDefault(); });
    box.addEventListener('drop', event => {
      chat.drag = 0; delete box.dataset.drop;
      if (!files(event)) return;
      event.preventDefault();
      addImages(chat, [...event.dataTransfer.files]);
    });
    el.addEventListener('click', event => {
      const link = event.target.closest('.ws-chat-markdown a[href]');
      if (link) {
        event.preventDefault();
        const path = localPath(link);
        if (path === null) { if (/^https?:/i.test(link.href)) window.__libroOpenNewTab?.(link.href); }
        else send(el, 'chat.view', {path});
        return;
      }
      const path = event.target.closest('[data-chat-path]');
      if (path) { send(el, 'chat.view', {path: path.dataset.chatPath}); return; }
      const images = event.target.closest('[data-chat-images]');
      if (images) { send(el, 'chat.view', {item: images.dataset.chatImages}); return; }
      if (event.target.closest('[data-chat-mic]')) { field.focus(); window.libroVoice?.toggle(); return; }
      if (event.target.closest('[data-chat-stop]')) { send(el, 'chat.stop', {}); return; }
      const button = event.target.closest('[data-chat-answer]');
      if (button) answer(el, button.closest('[data-chat-request]'), button.dataset.chatAnswer);
    });
    el.addEventListener('change', event => {
      const quick = event.target.closest('[data-chat-quick]');
      if (quick?.checked) answer(el, quick.closest('[data-chat-request]'), 'questions');
    });
    el.addEventListener('keydown', event => {
      if ((event.key === 'Enter' || event.key === ' ') && event.target.matches('[data-chat-path]')) {
        event.preventDefault(); send(el, 'chat.view', {path: event.target.dataset.chatPath}); return;
      }
      if (event.key !== 'Enter' || event.isComposing) return;
      const input = event.target.closest('[data-chat-request] input[type=text]');
      if (!input) return;
      event.preventDefault();
      const card = input.closest('[data-chat-request]');
      answer(el, card, input.name === 'message' ? 'reply' : 'questions');
    });
    connect(el, chat);
    if ((window.__libroSelectedApp || '') === appOf(el)) setTimeout(() => focus(el), 60);
  }

  // The agent's state and description reach the sidebar over the same
  // socket as terminal agents.
  function connect(el, chat) {
    if (chat.closed || !el.isConnected) return;
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${location.host}/terminal/ws/${encodeURIComponent(appOf(el))}?sid=${encodeURIComponent(sidOf(el))}`);
    chat.ws = ws;
    ws.onopen = () => { chat.attempts = 0; };
    ws.onmessage = event => {
      let msg;
      try { msg = JSON.parse(event.data); } catch { return; }
      if (msg.type === 'agent-status' || msg.type === 'agent-title') window.__libroAgentMessage?.(appOf(el), sidOf(el), msg);
    };
    ws.onclose = () => {
      window.__libroAgentMessage?.(appOf(el), sidOf(el), {type: 'closed'});
      if (chat.closed || !el.isConnected || ++chat.attempts >= 4) return;
      chat.timer = setTimeout(() => connect(el, chat), Math.min(1200, 150 * chat.attempts));
    };
  }

  function reconnect(el) {
    const chat = chats.get(el);
    if (!chat) return;
    clearTimeout(chat.timer);
    if (chat.ws) { chat.ws.onclose = null; chat.ws.close(); }
    chat.attempts = 0;
    setTimeout(() => connect(el, chat), 120);
  }

  function dispose(el) {
    const chat = chats.get(el);
    if (!chat) return;
    chat.closed = true;
    clearTimeout(chat.timer);
    chat.observer.disconnect();
    chat.resize.disconnect();
    for (const type of ['pointerup', 'touchend', 'pointercancel', 'touchcancel']) window.removeEventListener(type, chat.release);
    chat.images.forEach(image => URL.revokeObjectURL(image.url));
    if (chat.ws) { chat.ws.onclose = null; chat.ws.close(); }
    chats.delete(el);
  }

  function focus(el) {
    if (el.contains(document.activeElement)) return;
    const target = el.querySelector('[data-chat-request] button:not(:disabled)') || el.querySelector('[data-chat-input]');
    target?.focus({preventScroll: true});
  }

  // Page tools and voice typing put text into the message box.
  function paste(el, text, execute) {
    const chat = chats.get(el);
    if (!chat) return false;
    const field = chat.field;
    if (execute) {
      field.value = (field.value ? field.value + ' ' : '') + text;
      submit(el, chat);
      return true;
    }
    const start = field.selectionStart ?? field.value.length, end = field.selectionEnd ?? start;
    field.setRangeText(String(text), start, end, 'end');
    field.dispatchEvent(new Event('input', {bubbles: true}));
    return true;
  }

  // A link's file path, or null for a web or other link.
  function localPath(link) {
    const href = link.getAttribute('href');
    if (/^(https?:)?\/\/|^#|^(mailto|tel):/i.test(href) || /^[a-z][a-z0-9+.-]*:\/\//i.test(href) && !/^file:/i.test(href)) return null;
    const path = href.split('#')[0];
    try { return decodeURIComponent(path); } catch { return path; }
  }

  // The viewer is a sheet from the right, as in Kodri: a file named in the
  // chat or a message's images. The server picks what kind it is.
  function view(sheet) {
    document.getElementById('chat-view')?.remove();
    const dialog = document.createElement('dialog');
    dialog.id = 'chat-view';
    dialog.className = 'm-0 h-dvh max-h-none w-full max-w-none overflow-hidden border-0 bg-transparent p-0 text-(--ws-fg) backdrop:bg-black/50';
    dialog.setAttribute('aria-label', sheet.title);
    const shade = document.createElement('div');
    shade.className = 'absolute inset-0';
    shade.addEventListener('click', () => dialog.close());
    const panel = document.createElement('div');
    panel.className = 'absolute inset-y-0 right-0 flex w-full flex-col bg-(--ws-sunken) shadow-2xl ring-1 ring-(--ws-line) md:w-[min(60rem,calc(100vw-6rem))]';
    const head = document.createElement('div');
    head.className = 'flex min-h-14 shrink-0 items-center gap-2 border-b border-(--ws-line) px-5';
    const icon = {image: 'image', html: 'language', pdf: 'picture_as_pdf', markdown: 'article', text: 'description'}[sheet.kind] || 'draft';
    head.innerHTML = `<i class="material-icons-round shrink-0 text-xl text-(--ws-muted)" aria-hidden="true">${icon}</i>`;
    const title = document.createElement('h2');
    title.className = 'min-w-0 flex-1 truncate font-mono text-[14px] font-medium';
    title.textContent = title.title = sheet.title;
    const close = document.createElement('button');
    close.type = 'button';
    close.className = '-mr-2 flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center rounded-full border-0 bg-transparent text-(--ws-muted) transition-colors hover:bg-(--ws-hover) hover:text-(--ws-fg) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--ws-accent)';
    close.title = close.ariaLabel = 'Close';
    close.innerHTML = '<i class="material-icons-round text-xl" aria-hidden="true">close</i>';
    close.addEventListener('click', () => dialog.close());
    head.append(title, close);
    const body = document.createElement('div');
    body.className = 'min-h-0 flex-1 overflow-auto';
    const frame = (src, sandbox) => {
      const iframe = document.createElement('iframe');
      iframe.className = 'block h-full w-full border-0 bg-white';
      if (sandbox) iframe.setAttribute('sandbox', 'allow-scripts');
      iframe.src = src; iframe.title = sheet.title;
      return iframe;
    };
    if (sheet.kind === 'image') {
      body.className += ' flex flex-col items-center gap-4 p-5';
      sheet.urls.forEach((url, index) => {
        const img = document.createElement('img');
        img.className = 'max-w-full rounded-lg ring-1 ring-(--ws-line)';
        img.src = url; img.alt = sheet.urls.length > 1 ? `Image ${index + 1}` : sheet.title;
        body.append(img);
      });
    } else if (sheet.kind === 'html') body.append(frame(sheet.urls[0], true));
    else if (sheet.kind === 'pdf') body.append(frame(sheet.urls[0], false));
    else if (sheet.kind === 'markdown') {
      const md = document.createElement('div');
      md.className = sheet.class + ' mx-auto max-w-3xl px-6 py-5';
      md.innerHTML = sheet.html;
      body.append(md);
    } else if (sheet.kind === 'text') {
      const pre = document.createElement('pre');
      pre.className = 'm-0 min-h-full whitespace-pre p-5 font-mono text-[13px] leading-relaxed';
      pre.textContent = 'Loading…';
      fetch(sheet.urls[0]).then(response => response.ok ? response.text() : Promise.reject()).then(text => { pre.textContent = text; }, () => { pre.textContent = 'Could not read the file.'; });
      body.append(pre);
    } else {
      const note = document.createElement('p');
      note.className = 'p-10 text-center text-(--ws-muted)';
      note.textContent = sheet.note || 'No preview for this file.';
      body.append(note);
    }
    // Links in a Markdown file open in a browser tab or, for its own files,
    // in the viewer; never in place of Libro.
    body.addEventListener('click', event => {
      const link = event.target.closest('a[href]');
      if (!link) return;
      event.preventDefault();
      const path = localPath(link), chat = document.querySelector(`[data-chat="${CSS.escape(sheet.app)}"]`);
      if (path === null) { if (/^https?:/i.test(link.href)) window.__libroOpenNewTab?.(link.href); }
      else if (chat && path) send(chat, 'chat.view', {path: path.startsWith('/') || path.startsWith('~/') || !sheet.path ? path : sheet.path.replace(/[^/]*$/, '') + path});
    });
    panel.append(head, body);
    dialog.append(shade, panel);
    dialog.addEventListener('close', () => dialog.remove());
    document.body.append(dialog);
    dialog.showModal();
    close.focus();
  }

  window.libroChat = {mount, dispose, focus, paste, reconnect, view};
})();
