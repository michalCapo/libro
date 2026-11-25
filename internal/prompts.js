(function () {
  // The agent's terminal socket sends the prompts of its current session. The
  // strip above the agent shows the first one and opens a list of all of them
  // over the panel. j/k move between prompts, Enter expands one, Esc closes.
  const lists = new Map();
  const node = (tag, cls, text) => {
    const el = document.createElement(tag); el.className = cls;
    if (text !== undefined) el.textContent = text;
    return el;
  };
  const icon = name => { const el = node('i', 'material-icons-round', name); el.setAttribute('aria-hidden', 'true'); return el; };
  const mounts = appID => [...document.querySelectorAll('[data-agent-prompts="' + CSS.escape(appID) + '"]')];
  const oneLine = text => text.replace(/\s+/g, ' ').trim();
  const shortcutHint = () => { const key = window.libroWorkspace?.shortcutFor('agent-prompts'); return key ? ' (' + key + ')' : ''; };

  function set(appID, prompts) {
    prompts = Array.isArray(prompts) ? prompts : [];
    const current = lists.get(appID) || [];
    // A reconnect sends the same list again; the open list keeps its focus.
    if (current.length === prompts.length && current.every((text, index) => text === prompts[index])) return;
    lists.set(appID, prompts);
    mounts(appID).forEach(render);
  }

  function render(el) {
    const prompts = lists.get(el.dataset.agentPrompts) || [];
    if (!prompts.length) { el.replaceChildren(); return; }
    if (!el.firstChild) build(el);
    const trigger = el.querySelector('.ws-prompts-trigger');
    trigger.querySelector('.ws-prompts-first').textContent = oneLine(prompts[0]);
    trigger.querySelector('.ws-prompts-count').textContent = prompts.length;
    trigger.title = 'Prompts' + shortcutHint();
    trigger.setAttribute('aria-label', 'Prompts, ' + prompts.length + '. First: ' + oneLine(prompts[0]).slice(0, 200));
    const popover = el.querySelector('.ws-prompts-popover');
    if (popover.matches(':popover-open')) fill(el);
  }

  function build(el) {
    const appID = el.dataset.agentPrompts;
    const trigger = node('button', 'ws-prompts-trigger'); trigger.type = 'button';
    trigger.setAttribute('aria-haspopup', 'dialog');
    trigger.append(icon('history'), node('span', 'ws-prompts-first'), node('span', 'ws-prompts-count'));
    const popover = node('div', 'ws-prompts-popover');
    popover.id = 'agent-prompts-' + appID;
    popover.setAttribute('popover', 'auto'); popover.setAttribute('role', 'dialog'); popover.setAttribute('aria-label', 'Prompts');
    popover.tabIndex = -1;
    trigger.setAttribute('aria-controls', popover.id);
    const list = node('ol', 'ws-prompts-list');
    popover.append(list);
    el.append(trigger, popover);
    // Pressing the strip while the list is open closes it before the click; the click must not reopen it.
    let closedAt = 0;
    trigger.onclick = () => { if (performance.now() - closedAt > 300) toggle(el); };
    // The list follows the panel when the window, sidebar or tools change its size.
    const resize = new ResizeObserver(() => place(el));
    popover.addEventListener('toggle', event => {
      const open = event.newState === 'open';
      trigger.setAttribute('aria-expanded', String(open));
      if (open) { resize.observe(el.parentElement); return; }
      resize.disconnect();
      // Esc returns to the agent; a click elsewhere keeps the focus it moved.
      const focus = document.activeElement;
      if (!focus || focus === document.body || popover.contains(focus)) window.__libroFocusTerminalFrame?.(appID);
    });
    // beforetoggle runs before the click that follows the closing press; toggle runs later.
    popover.addEventListener('beforetoggle', event => { if (event.newState === 'closed') closedAt = performance.now(); });
    // Another dialog, such as the command palette, would otherwise take the list's keys.
    popover.addEventListener('focusout', event => {
      if (!popover.contains(event.relatedTarget) && popover.matches(':popover-open')) popover.hidePopover();
    });
    popover.addEventListener('keydown', event => {
      const step = {j: 1, ArrowDown: 1, k: -1, ArrowUp: -1}[event.key];
      if (!step || event.ctrlKey || event.altKey || event.metaKey) return;
      event.preventDefault(); event.stopPropagation();
      const items = [...list.querySelectorAll('.ws-prompt')];
      const index = items.indexOf(event.target.closest('.ws-prompts-item')?.querySelector('.ws-prompt'));
      items[Math.max(0, Math.min(items.length - 1, index + step))]?.focus();
    });
  }

  function toggle(el) {
    const popover = el.querySelector('.ws-prompts-popover');
    if (!popover) return false;
    if (popover.matches(':popover-open')) { popover.hidePopover(); return true; }
    window.__libroCloseAllPopups?.();
    fill(el);
    place(el);
    popover.showPopover();
    popover.querySelector('.ws-prompt')?.focus();
    return true;
  }

  // The list covers the agent panel below the strip, across its full width.
  function place(el) {
    const popover = el.querySelector('.ws-prompts-popover');
    if (!popover) return;
    const strip = el.getBoundingClientRect();
    const panel = el.parentElement.getBoundingClientRect();
    popover.style.left = panel.left + 'px';
    popover.style.width = panel.width + 'px';
    popover.style.top = strip.bottom + 'px';
    popover.style.maxHeight = Math.max(120, panel.bottom - strip.bottom) + 'px';
  }

  // A new prompt may arrive while the list is open; expanded rows and focus stay.
  function fill(el) {
    const appID = el.dataset.agentPrompts;
    const prompts = lists.get(appID) || [];
    const list = el.querySelector('.ws-prompts-list');
    const open = new Set([...list.querySelectorAll('.ws-prompt[aria-expanded=true]')].map(item => item.dataset.index));
    const focused = list.contains(document.activeElement) ? document.activeElement.closest('.ws-prompts-item').querySelector('.ws-prompt').dataset.index : null;
    list.replaceChildren();
    prompts.forEach((text, index) => {
      const item = node('li', 'ws-prompts-item');
      const prompt = node('button', 'ws-prompt'); prompt.type = 'button'; prompt.dataset.index = index;
      prompt.append(node('span', 'ws-prompt-number', String(index + 1)), node('span', 'ws-prompt-text', text));
      const actions = node('div', 'ws-prompt-actions');
      const action = (label, glyph, run) => {
        const button = node('button', 'ws-prompt-action'); button.type = 'button';
        button.append(icon(glyph), label); button.onclick = run; return button;
      };
      actions.append(
        action('Copy', 'content_copy', () => copy(text)),
        action('Insert into agent', 'input', () => insert(el, text)),
      );
      const expand = expanded => { prompt.setAttribute('aria-expanded', String(expanded)); actions.hidden = !expanded; };
      expand(open.has(String(index)));
      prompt.onclick = () => expand(prompt.getAttribute('aria-expanded') !== 'true');
      item.append(prompt, actions);
      list.append(item);
    });
    if (focused !== null) list.querySelector('.ws-prompt[data-index="' + focused + '"]')?.focus();
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text); window.__libroShowToast?.('Prompt copied', '', 'success'); }
    catch { window.__libroShowToast?.('Could not copy the prompt', '', 'error'); }
  }

  // The agent receives the text as a paste without Enter, so it can be edited first.
  function insert(el, text) {
    const appID = el.dataset.agentPrompts;
    el.querySelector('.ws-prompts-popover').hidePopover();
    // Control characters other than tab and newline could end the bracketed paste.
    const clean = text.replace(/\r\n?/g, '\n').replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '');
    if (!window.__libroSendPageToolPrompt?.(clean, false, appID)) {
      window.__libroShowToast?.('The agent is not connected', '', 'error');
      return;
    }
    window.libroWorkspace?.select(appID);
    window.__libroFocusTerminalFrame?.(appID);
  }

  // The shortcut opens the list of the current agent panel.
  function open(appID) {
    const el = mounts(appID || '').find(mount => mount.getClientRects().length && mount.firstChild);
    return el ? toggle(el) : false;
  }

  window.libroPrompts = {set, open, mount: render};
})();
