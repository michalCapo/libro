// Chat panels: the message box, answers to Claude's requests, and the
// agent state for the sidebar. The server draws the timeline and updates it
// live; this script only sends input and keeps the view at the bottom.
(function () {
  if (window.libroChat) return;
  const chats = new Map();
  const appOf = el => el.dataset.chat;
  const sidOf = el => el.dataset.sid || window.__libroWorkspaceSID || '';
  const send = (el, action, data) => window.__ws?.call(action, {sid: sidOf(el), appId: appOf(el), ...data});

  function submit(el, text) {
    text = String(text || '').trim();
    if (!text) return false;
    send(el, 'chat.send', {text});
    return true;
  }

  // Each card collects its own fields: chosen options, own answers and a
  // reply that declines with a reason.
  function answer(el, card, kind) {
    const request = card.dataset.chatRequest;
    const message = card.querySelector('input[name=message]')?.value.trim() || '';
    if (kind === 'reply' && !message) { card.querySelector('input[name=message]')?.focus(); return; }
    const data = {request, allow: kind === 'allow' || kind === 'always' || kind === 'questions', always: kind === 'always', message};
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
    const scroll = el.querySelector('[data-chat-scroll]');
    const field = el.querySelector('[data-chat-input]');
    const form = el.querySelector('[data-chat-form]');
    const chat = {stick: true, ws: null, attempts: 0, timer: null, closed: false};
    chats.set(el, chat);
    scroll.scrollTop = scroll.scrollHeight;
    scroll.addEventListener('scroll', () => { chat.stick = nearBottom(scroll); }, {passive: true});
    chat.observer = new MutationObserver(() => { if (chat.stick) scroll.scrollTop = scroll.scrollHeight; });
    chat.observer.observe(scroll, {childList: true, subtree: true, characterData: true});
    form.addEventListener('submit', event => {
      event.preventDefault();
      if (submit(el, field.value)) { field.value = ''; chat.stick = true; scroll.scrollTop = scroll.scrollHeight; }
    });
    field.addEventListener('keydown', event => {
      if (event.key !== 'Enter' || event.shiftKey || event.isComposing || event.keyCode === 229) return;
      event.preventDefault();
      form.requestSubmit();
    });
    el.addEventListener('click', event => {
      const link = event.target.closest('.ws-chat-markdown a[href]');
      if (link) {
        event.preventDefault();
        if (/^https?:/i.test(link.href)) window.__libroOpenNewTab?.(link.href);
        return;
      }
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
    const field = el.querySelector('[data-chat-input]');
    if (!field) return false;
    if (execute) return submit(el, (field.value ? field.value + ' ' : '') + text) && ((field.value = ''), true);
    const start = field.selectionStart ?? field.value.length, end = field.selectionEnd ?? start;
    field.setRangeText(String(text), start, end, 'end');
    field.dispatchEvent(new Event('input', {bubbles: true}));
    return true;
  }

  window.libroChat = {mount, dispose, focus, paste, reconnect};
})();
