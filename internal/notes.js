(function () {
  const states = new Map();
  let sequence = 0;
  const element = (tag, cls, text) => {
    const el = document.createElement(tag); el.className = cls || '';
    if (text !== undefined) el.textContent = text;
    return el;
  };
  function button(text, action) {
    const el = element('button', 'ws-notes-button', text); el.type = 'button'; el.onclick = action; return el;
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
    const titleLabel = element('label', 'ws-note-label', 'Title');
    const title = element('input', 'ws-note-title'); title.value = note.title; title.maxLength = 200; title.required = true;
    titleLabel.append(title);
    const stateLabel = element('label', 'ws-note-label', 'State');
    const select = element('select');
    select.add(new Option('New', 'new')); select.add(new Option('Archived', 'archived')); select.value = note.state;
    stateLabel.append(select);
    const bodyLabel = element('label', 'ws-note-label', 'Note');
    const body = element('textarea', 'ws-note-body'); body.value = note.body;
    body.placeholder = 'Describe the task…'; bodyLabel.append(body);
    const hint = element('p', 'ws-note-hint', 'Markdown supported: headings, lists, **bold**, *italic*, and `code`. Paste images here.');
    s.preview = element('div', 'ws-note-preview'); s.preview.setAttribute('aria-label', 'Live Markdown preview');
    s.images = element('div', 'ws-note-images');
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    const actions = element('div', 'ws-note-actions');
    s.save = button('Save note', () => form.requestSubmit());
    s.cancel = button('Cancel', () => { clearTimeout(s.timer); const id = s.draft.id; s.draft = null; renderList(s); (Array.from(s.el.querySelectorAll('[data-note-id]')).find(row => row.dataset.noteId === id) || s.el.querySelector('.ws-notes-toolbar button')).focus(); });
    s.send = button('Send to agent', () => {
      s.busy = true; updateActions(s); status(s, 'Sending…');
      request(s, 'send', {noteID:s.draft.id});
    });
    actions.append(s.save, s.cancel, s.send);
    form.append(titleLabel, stateLabel, bodyLabel, hint, s.preview, s.images, actions, s.status);
    s.el.append(form);
    title.oninput = () => { s.draft.title = title.value; changed(s); };
    select.onchange = () => { s.draft.state = select.value; changed(s); };
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
    const toolbar = element('div', 'ws-notes-toolbar');
    const filter = element('select'); filter.setAttribute('aria-label', 'Filter notes by state');
    ['new', 'archived', 'all'].forEach(value => filter.add(new Option(value[0].toUpperCase() + value.slice(1), value)));
    filter.value = s.filter;
    filter.onchange = () => { s.filter = filter.value; renderList(s); s.el.querySelector('.ws-notes-toolbar select').focus(); };
    toolbar.append(filter, button('Add note', () => edit(s, {id:'', title:'', body:'', state:'new', images:[]})));
    const list = element('div', 'ws-notes-list');
    const notes = s.notes.filter(note => s.filter === 'all' || note.state === s.filter);
    notes.forEach(note => {
      const row = button('', () => edit(s, note)); row.className = 'ws-note-row'; row.dataset.noteId = note.id;
      row.append(element('span', 'ws-note-row-title', note.title), element('span', 'ws-note-state', note.state === 'new' ? 'New' : 'Archived'));
      list.append(row);
    });
    if (!notes.length) list.append(element('p', 'ws-notes-empty', s.filter === 'archived' ? 'No archived notes.' : 'No notes yet. Add a note to track a task.'));
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    s.el.append(toolbar, list, s.status);
  }
  function init() {
    for (const [id, s] of states) if (!s.el.isConnected) { clearTimeout(s.timer); states.delete(id); }
    document.querySelectorAll('[data-notes]').forEach(el => {
      const project = el.closest('[data-workspace-project]')?.dataset.workspaceProject;
      if (project !== window.__libroActiveProject || states.has(el.dataset.notes)) return;
      const s = {el, id:el.dataset.notes, project, notes:[], requests:new Map(), filter:'new', busy:false, reading:0};
      states.set(s.id, s);
      s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
      s.el.replaceChildren(s.status); status(s, 'Loading notes…'); request(s, 'list');
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
      if (action === 'list') s.el.replaceChildren(s.status, button('Retry', () => { s.el.replaceChildren(s.status); status(s, 'Loading notes…'); request(s, 'list'); }));
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
