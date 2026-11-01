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
        if (chat.images.length >= maxImages) break;
        chat.images.push({blob, url: URL.createObjectURL(blob)});
      } catch { showError(chat, 'Could not read one of the images.'); }
    }
    renderTray(chat);
    return true;
  }

  async function submit(el, chat) {
    const text = chat.field.value.trim();
    if ((!text && !chat.images.length) || chat.busy) return;
    chat.busy = true; chat.send.disabled = true;
    showError(chat, '');
    try {
      const images = chat.images.length ? await upload(el, chat.images.map(image => image.blob)) : [];
      send(el, 'chat.send', {text, images});
      chat.images.forEach(image => URL.revokeObjectURL(image.url));
      chat.images = []; renderTray(chat);
      chat.field.value = '';
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

  function nearBottom(scroll) { return scroll.scrollHeight - scroll.scrollTop - scroll.clientHeight < 48; }

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
    scroll.scrollTop = scroll.scrollHeight;
    scroll.addEventListener('scroll', () => { chat.stick = nearBottom(scroll); }, {passive: true});
    chat.observer = new MutationObserver(() => { if (chat.stick) scroll.scrollTop = scroll.scrollHeight; });
    chat.observer.observe(scroll, {childList: true, subtree: true, characterData: true});
    form.addEventListener('submit', event => { event.preventDefault(); submit(el, chat); });
    field.addEventListener('keydown', event => {
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
        if (/^https?:/i.test(link.href)) window.__libroOpenNewTab?.(link.href);
        return;
      }
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

  window.libroChat = {mount, dispose, focus, paste, reconnect};
})();
