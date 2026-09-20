(function () {
  const states = new Map();
  let sequence = 0;
  const element = (tag, cls, text) => {
    const el = document.createElement(tag); el.className = cls || '';
    if (text !== undefined) el.textContent = text;
    return el;
  };
  function button(text, action, cls = 'ws-notes-button') {
    const el = element('button', cls, text); el.type = 'button'; el.onclick = action; return el;
  }
  function request(s, action, data = {}) {
    const token = String(++sequence);
    s.requests.set(token, action);
    fetch('/notes/action', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({sid:window.__libroWorkspaceSID, id:s.id, project:s.project, request:token, action, ...data})
    }).then(response => {
      if (!response.ok) throw new Error('Could not ' + action + ' note. Try again or reduce its size.');
      return response.json();
    }).then(receive).catch(error => receive({id:s.id, request:token, error:error.message}));
    return token;
  }
  function status(s, text) { s.status.textContent = text; }
  function dirty(s) { return JSON.stringify(s.draft) !== JSON.stringify(s.saved); }
  function updateActions(s) {
    s.save.disabled = s.busy || s.reading > 0 || !s.draft.title.trim() || !dirty(s);
    s.send.disabled = s.busy || s.reading > 0 || !s.draft.id || dirty(s);
    s.cancel.disabled = s.busy;
  }
  function changed(s) { updateActions(s); status(s, dirty(s) ? 'Unsaved changes' : ''); }
  function preview(s) {
    clearTimeout(s.timer);
    s.previewRequest = '';
    s.timer = setTimeout(() => { s.previewRequest = request(s, 'preview', {body:s.draft.body}); }, 150);
  }
  function relativeTime(value) {
    const time = Date.parse(value);
    if (Number.isNaN(time)) return '';
    const seconds = Math.max(1, Math.round((Date.now() - time) / 1000));
    const units = [[31536000, 'y'], [2592000, 'mo'], [604800, 'w'], [86400, 'd'], [3600, 'h'], [60, 'm']];
    for (const [span, label] of units) if (seconds >= span) return Math.floor(seconds / span) + label + ' ago';
    return 'just now';
  }
  function badge(state) {
    const el = element('span', 'ws-note-badge ws-note-badge-' + state);
    el.append(element('span', 'ws-note-badge-dot'), state === 'new' ? 'Open' : 'Archived');
    return el;
  }
  function shortID(note) { return '#' + note.id.slice(0, 6); }
  function showImages(s) {
    s.images.replaceChildren();
    s.draft.images.forEach((attachment, index) => {
      const figure = element('figure', 'ws-note-image');
      const img = element('img'); img.src = attachment.data; img.alt = 'Pasted image ' + (index + 1);
      figure.append(img, button('Remove image ' + (index + 1), () => {
        s.draft.images.splice(index, 1); showImages(s); changed(s);
      }));
      s.images.append(figure);
    });
  }
  function edit(s, note) {
    s.saved = structuredClone(note);
    s.draft = structuredClone(note);
    s.saved.images ||= []; s.draft.images ||= [];
    s.el.replaceChildren();
    const form = element('form', 'ws-note-editor');
    const back = button('‹ Issues', () => s.cancel.click(), 'ws-note-back');
    const header = element('div', 'ws-note-editor-header');
    const title = element('input', 'ws-note-title'); title.value = note.title; title.maxLength = 200; title.required = true;
    title.placeholder = 'Issue title'; title.setAttribute('aria-label', 'Title');
    header.append(back, title, badge(s.draft.state));
    const meta = element('div', 'ws-note-editor-meta');
    const idSpan = element('span', 'ws-note-id', note.id ? shortID(note) : 'New issue');
    const dateSpan = element('span', 'ws-note-date');
    if (note.updated) dateSpan.textContent = 'Last edited ' + relativeTime(note.updated);
    meta.append(idSpan, dateSpan);
    const stateRow = element('div', 'ws-note-state-row');
    stateRow.append(element('span', 'ws-note-state-label', 'State'));
    const chips = element('div', 'ws-note-state-chips'); chips.setAttribute('role', 'radiogroup'); chips.setAttribute('aria-label', 'State');
    const chipButtons = {};
    for (const [value, label] of [['new', 'Open'], ['archived', 'Archived']]) {
      const chip = button(label, () => {
        s.draft.state = value;
        for (const [k, b] of Object.entries(chipButtons)) {
          b.classList.toggle('ws-note-chip-active', k === value);
          b.setAttribute('aria-checked', String(k === value));
        }
        header.querySelector('.ws-note-badge').replaceWith(badge(value));
        changed(s);
      }, 'ws-note-chip' + (value === note.state ? ' ws-note-chip-active' : ''));
      chip.setAttribute('role', 'radio');
      chip.setAttribute('aria-checked', String(value === note.state));
      chipButtons[value] = chip;
      chips.append(chip);
    }
    stateRow.append(chips);
    const body = element('textarea', 'ws-note-body'); body.value = note.body;
    body.placeholder = 'Describe the task…'; body.setAttribute('aria-label', 'Note');
    body.rows = 10;
    const hint = element('p', 'ws-note-hint', 'Markdown supported: headings, lists, **bold**, *italic*, and `code`. Paste images here.');
    const previewWrap = element('div', 'ws-note-preview-wrap');
    previewWrap.append(element('div', 'ws-note-preview-title', 'Preview'));
    s.preview = element('div', 'ws-note-preview'); s.preview.setAttribute('aria-label', 'Live Markdown preview');
    previewWrap.append(s.preview);
    s.images = element('div', 'ws-note-images');
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    const actions = element('div', 'ws-note-actions');
    s.save = button('Save issue', () => form.requestSubmit(), 'ws-notes-button ws-notes-primary');
    s.cancel = button('Cancel', () => { clearTimeout(s.timer); const id = s.draft.id; s.draft = null; renderList(s); (Array.from(s.el.querySelectorAll('[data-note-id]')).find(row => row.dataset.noteId === id) || s.el.querySelector('.ws-notes-toolbar button')).focus(); });
    s.send = button('Send to agent', () => {
      s.busy = true; updateActions(s); status(s, 'Sending…');
      request(s, 'send', {noteID:s.draft.id});
    });
    actions.append(s.save, s.send, s.cancel);
    form.append(header, meta, stateRow, body, hint, previewWrap, s.images, actions, s.status);
    s.el.append(form);
    title.oninput = () => { s.draft.title = title.value; changed(s); };
    body.oninput = () => { s.draft.body = body.value; changed(s); preview(s); };
    body.onpaste = async event => {
      const items = Array.from(event.clipboardData?.items || []).filter(item => item.type.startsWith('image/'));
      if (!items.length) return;
      event.preventDefault();
      const draft = s.draft;
      s.reading++; updateActions(s);
      try {
        for (const item of items) {
          const file = item.getAsFile();
          if (!file || !['image/png', 'image/jpeg', 'image/gif'].includes(file.type)) throw new Error('Paste a PNG, JPEG, or GIF image.');
          if (file.size > 4 * 1024 * 1024) throw new Error('Each image must be 4 MB or smaller.');
          const data = await new Promise((resolve, reject) => {
            const reader = new FileReader(); reader.onload = () => resolve(reader.result); reader.onerror = () => reject(new Error('Could not read clipboard image.')); reader.readAsDataURL(file);
          });
          if (s.draft !== draft) return;
          if (draft.images.reduce((size, image) => size + atob(image.data.split(',')[1]).length, file.size) > 4 * 1024 * 1024) throw new Error('Images must total 4 MB or less.');
          draft.images.push({data}); showImages(s); changed(s);
        }
      } catch (error) { if (s.draft === draft) status(s, error.message); }
      finally { s.reading--; if (s.draft) updateActions(s); }
    };
    form.onsubmit = event => {
      event.preventDefault(); if (s.save.disabled) return;
      if (new TextEncoder().encode(JSON.stringify(s.draft)).length > 9 * 1024 * 1024) { status(s, 'This note is too large. Remove an image or shorten the text.'); return; }
      s.busy = true; updateActions(s); status(s, 'Saving…');
      request(s, 'save', {note:s.draft});
      form.querySelectorAll('input,textarea,select,button').forEach(el => { el.disabled = true; });
    };
    showImages(s); updateActions(s); preview(s); title.focus();
  }
  function renderList(s) {
    s.el.replaceChildren();
    const header = element('div', 'ws-notes-header');
    const heading = element('div', 'ws-notes-heading');
    const open = s.notes.filter(note => note.state === 'new').length;
    const archived = s.notes.length - open;
    heading.append(element('span', 'ws-notes-title', 'Issues'), element('span', 'ws-notes-count', open + ' Open · ' + archived + ' Archived'));
    header.append(heading, button('New issue', () => edit(s, {id:'', title:'', body:'', state:'new', images:[]}), 'ws-notes-button ws-notes-primary'));
    const toolbar = element('div', 'ws-notes-toolbar');
    const tabs = element('div', 'ws-notes-tabs');
    for (const [value, label] of [['new', 'Open'], ['archived', 'Archived'], ['all', 'All']]) {
      const tab = button(label, () => { s.filter = value; s.search = s.el.querySelector('.ws-notes-search').value; renderList(s); }, 'ws-notes-tab' + (s.filter === value ? ' ws-notes-tab-active' : ''));
      tab.setAttribute('aria-pressed', String(s.filter === value));
      tabs.append(tab);
    }
    const search = element('input', 'ws-notes-search');
    search.type = 'search'; search.placeholder = 'Search issues…'; search.value = s.search; search.setAttribute('aria-label', 'Search issues');
    search.oninput = () => { s.search = search.value; listRows(s, list); };
    toolbar.append(tabs, search);
    const list = element('div', 'ws-notes-list');
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    s.el.append(header, toolbar, list, s.status);
    listRows(s, list);
  }
  function listRows(s, list) {
    list.replaceChildren();
    const query = s.search.trim().toLowerCase();
    const notes = s.notes.filter(note => (s.filter === 'all' || note.state === s.filter) && (!query || note.title.toLowerCase().includes(query)));
    notes.forEach(note => {
      const row = button('', () => edit(s, note), 'ws-note-row');
      row.dataset.noteId = note.id;
      const main = element('span', 'ws-note-row-main');
      main.append(element('span', 'ws-note-row-title-line', note.title),
        element('span', 'ws-note-row-meta', shortID(note) + ' · updated ' + relativeTime(note.updated)));
      row.append(badge(note.state), main, element('span', 'ws-note-chevron', '›'));
      list.append(row);
    });
    if (!notes.length) {
      const empty = element('div', 'ws-notes-empty');
      empty.append(element('div', 'ws-notes-empty-icon', '✓'),
        element('p', 'ws-notes-empty-text', query ? 'No issues match your search.' : s.filter === 'archived' ? 'No archived issues.' : 'No open issues. Good job, or add one to track a task.'));
      list.append(empty);
    }
  }
  function init() {
    for (const [id, s] of states) if (!s.el.isConnected) { clearTimeout(s.timer); states.delete(id); }
    document.querySelectorAll('[data-notes]').forEach(el => {
      const project = el.closest('[data-workspace-project]')?.dataset.workspaceProject;
      if (project !== window.__libroActiveProject || states.has(el.dataset.notes)) return;
      const s = {el, id:el.dataset.notes, project, notes:[], requests:new Map(), filter:'new', search:'', busy:false, reading:0};
      states.set(s.id, s);
      s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
      s.el.replaceChildren(s.status); status(s, 'Loading issues…'); request(s, 'list');
    });
  }
  function receive(result) {
    const s = states.get(result.id);
    if (!s || !s.el.isConnected) return;
    const action = s.requests.get(result.request); s.requests.delete(result.request);
    if (!action) return;
    if (action === 'preview') {
      if (result.request !== s.previewRequest || !s.draft) return;
      if (!result.error) {
        const template = document.createElement('template');
        template.innerHTML = result.html;
        // Notes never fetch remote images or navigate the workspace from their preview.
        template.content.querySelectorAll('img').forEach(image => image.replaceWith(document.createTextNode(image.alt || '[image]')));
        template.content.querySelectorAll('a').forEach(link => { link.removeAttribute('href'); });
        s.preview.replaceChildren(template.content);
      }
      return;
    }
    s.busy = false;
    if (s.draft) {
      s.el.querySelectorAll('input,textarea,select,button').forEach(el => { el.disabled = false; });
      updateActions(s);
    }
    if (result.error) {
      status(s, result.error);
      if (action === 'list') s.el.replaceChildren(s.status, button('Retry', () => { s.el.replaceChildren(s.status); status(s, 'Loading issues…'); request(s, 'list'); }));
      return;
    }
    if (action === 'list') { s.notes = result.notes; if (!s.draft) renderList(s); }
    if (action === 'save') {
      s.notes = [result.note, ...s.notes.filter(note => note.id !== result.note.id)];
      edit(s, result.note); status(s, 'Saved');
    }
    if (action === 'send') {
      const active = s.project === window.__libroActiveProject;
      const sent = active && window.__libroSendPageToolPrompt?.(result.prompt, true);
      status(s, sent ? 'Sent to agent' : 'Start or select an agent in this project, then try again.');
    }
  }
  window.libroNotes = {init, receive};
})();
