(function () {
  const states = new Map();
  let sequence = 0;
  let activeProject;
  let activeWorkspace;
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
    s.send.disabled = s.busy || s.saving || s.reading > 0 || !s.draft.id || dirty(s);
    s.move.disabled = s.busy || s.saving || s.reading > 0 || !s.draft.id || dirty(s) || !s.target.value;
    s.target.disabled = s.busy || s.saving;
    s.remove.disabled = s.busy || s.saving || s.reading > 0 || !s.draft.id;
    s.el.querySelectorAll('.ws-notes-toolbar button,.ws-notes-search,.ws-note-row').forEach(el => { el.disabled = s.busy || s.saving || s.reading > 0; });
  }
  function changed(s) {
    clearTimeout(s.saveTimer);
    updateActions(s);
    status(s, dirty(s) ? 'Waiting to save…' : 'Saved');
    if (dirty(s)) s.saveTimer = setTimeout(() => autoSave(s), 600);
  }
  function autoSave(s) {
    clearTimeout(s.saveTimer);
    if (!s.draft || s.busy || s.saving || s.reading > 0 || !dirty(s)) return;
    Object.assign(s.draft, s.editor.serialize());
    if (!s.draft.id && !s.draft.body.trim() && !s.draft.images.length) return;
    s.savingNote = structuredClone(s.draft);
    // The server titles the note from its first line; image-only notes keep a fallback.
    const note = {...s.savingNote, title:s.draft.body.trim() ? '' : 'Untitled note'};
    if (new TextEncoder().encode(JSON.stringify(note)).length > 9 * 1024 * 1024) {
      status(s, 'This note is too large. Remove an image or shorten the text.'); return;
    }
    s.saving = true; updateActions(s); status(s, 'Saving…');
    request(s, 'save', {note});
  }
  function relativeTime(value) {
    const time = Date.parse(value);
    if (Number.isNaN(time)) return '';
    const seconds = Math.max(1, Math.round((Date.now() - time) / 1000));
    const units = [[31536000, 'y'], [2592000, 'mo'], [604800, 'w'], [86400, 'd'], [3600, 'h'], [60, 'm']];
    for (const [span, label] of units) if (seconds >= span) return Math.floor(seconds / span) + label + ' ago';
    return 'just now';
  }
  function shortID(note) { return '#' + note.id.slice(0, 6); }
  function canClose(s) {
    if (s.busy || s.saving || s.reading > 0) return false;
    if (s.draft && dirty(s)) { autoSave(s); status(s, 'Saving changes before closing.'); return false; }
    return true;
  }
  function collapse(s) {
    clearTimeout(s.saveTimer);
    const id = s.draft.id;
    s.draft = null; s.reading = 0;
    renderList(s);
    (Array.from(s.el.querySelectorAll('[data-note-id]')).find(row => row.dataset.noteId === id) || s.el.querySelector('.ws-notes-new')).focus();
  }
  function edit(s, note) {
    if (!canClose(s)) return;
    // Older notes kept the title apart from the body; show it as the first line.
    const body = note.title && !note.body.includes(note.title) ? '# ' + note.title + (note.body ? '\n\n' + note.body : '') : note.body;
    s.saved = {...structuredClone(note), body, images:note.images || []};
    s.draft = structuredClone(s.saved);
    renderList(s);
    (note.id ? s.detail.previousElementSibling : s.detail.querySelector('.ws-note-body')).focus();
  }
  function renderEditor(s) {
    const note = s.draft;
    s.reading = 0;
    const form = element('form', 'ws-note-editor');
    const body = element('div', 'ws-note-rich-editor');
    const hint = element('p', 'ws-note-hint', 'The first line is the title. Use Markdown shortcuts or the toolbar, and paste screenshots anywhere.');
    const description = element('div', 'ws-note-section');
    description.append(body, hint);
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    const actionsSection = element('div', 'ws-note-section');
    const actions = element('div', 'ws-note-actions');
    s.send = button('Send to agent', () => {
      s.busy = true; updateActions(s); status(s, 'Sending…');
      request(s, 'send', {noteID:s.draft.id});
    });
    s.remove = button('Delete note', () => {
      if (!window.confirm('Delete this note permanently?')) return;
      clearTimeout(s.saveTimer);
      s.busy = true; updateActions(s); status(s, 'Deleting…');
      request(s, 'delete', {noteID:s.draft.id});
      s.editor.setEditable(false);
    });
    s.target = element('select');
    s.target.setAttribute('aria-label', 'Move note to project');
    const placeholder = element('option', '', 'Choose project…'); placeholder.value = '';
    s.target.append(placeholder);
    for (const project of s.projects) {
      const option = element('option', '', project); option.value = project; s.target.append(option);
    }
    s.target.onchange = () => updateActions(s);
    s.move = button('Move note', () => {
      if (s.move.disabled) return;
      s.busy = true; updateActions(s); status(s, 'Moving…');
      request(s, 'move', {noteID:s.draft.id, target:s.target.value});
      s.editor.setEditable(false);
      form.querySelectorAll('input,textarea,select,button').forEach(el => { el.disabled = true; });
    });
    actions.append(s.send, s.target, s.move, s.status, s.remove);
    for (const [control, label, icon] of [[s.send, 'Send to agent', 'send'], [s.remove, 'Delete note', 'delete_outline'], [s.move, 'Move note', 'drive_file_move']]) {
      control.title = label;
      control.setAttribute('aria-label', label);
      const glyph = element('span', 'material-icons-round', icon);
      glyph.setAttribute('aria-hidden', 'true');
      control.replaceChildren(glyph);
      if (control === s.send) control.append(label);
      else control.classList.add('ws-note-icon-button');
    }
    s.send.classList.add('ws-notes-primary');
    s.remove.classList.add('ws-note-delete');
    actionsSection.append(actions);
    form.append(description, actionsSection);
    s.detail.append(form);
    s.editor = libroNoteEditor.create({
      element: body, note: s.draft,
      onChange: value => { Object.assign(s.draft, value); changed(s); },
      onBusy: value => { s.reading = Number(value); if (!value) changed(s); else updateActions(s); },
      onError: message => status(s, message),
    });
    form.onsubmit = event => { event.preventDefault(); autoSave(s); };
    s.editor.setEditable(!s.busy);
    updateActions(s);
  }
  function renderList(s) {
    s.editor?.destroy(); s.editor = null;
    s.el.replaceChildren();
    const heading = element('div', 'ws-notes-heading');
    heading.append(element('span', 'ws-notes-title', 'Notes'), element('span', 'ws-notes-project', s.project));
    const toolbar = element('div', 'ws-notes-toolbar');
    const search = element('input', 'ws-notes-search');
    search.type = 'search'; search.placeholder = 'Search notes…'; search.value = s.search; search.setAttribute('aria-label', 'Search notes');
    search.oninput = () => { s.search = search.value; listRows(s, list); };
    const actions = element('div', 'ws-notes-toolbar-actions');
    actions.append(search, button('New note', () => edit(s, {id:'', title:'', body:'', state:'new', images:[]}), 'ws-notes-button ws-notes-primary ws-notes-new'));
    toolbar.append(heading, actions);
    const list = element('div', 'ws-notes-list');
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    s.el.append(toolbar, list, s.status);
    listRows(s, list);
  }
  function listRows(s, list) {
    s.editor?.destroy(); s.editor = null;
    list.replaceChildren();
    const query = s.search.trim().toLowerCase();
    const notes = s.notes.filter(note => !query || (note.title + '\n' + note.body).toLowerCase().includes(query));
    if (s.draft && !notes.some(note => note.id === s.draft.id)) notes.unshift(s.draft);
    notes.forEach(note => {
      const expanded = s.draft?.id === note.id;
      const item = element('div', 'ws-note-item');
      const row = button('', () => {
        if (expanded) { if (canClose(s)) collapse(s); }
        else edit(s, note);
      }, 'ws-note-row');
      row.dataset.noteId = note.id;
      row.id = 'note-row-' + s.id + '-' + (note.id || 'new');
      const main = element('span', 'ws-note-row-main');
      main.append(element('span', 'ws-note-row-title-line', note.title || 'New note'),
        element('span', 'ws-note-row-meta', note.id ? shortID(note) + ' · updated ' + relativeTime(note.updated) : 'Unsaved note'));
      const chevron = element('span', 'material-icons-round ws-note-chevron', 'expand_more');
      chevron.setAttribute('aria-hidden', 'true');
      row.append(main, chevron);
      row.setAttribute('aria-expanded', String(expanded));
      const detail = element('div', 'ws-note-detail');
      detail.id = row.id + '-detail';
      detail.hidden = !expanded;
      detail.setAttribute('role', 'region');
      detail.setAttribute('aria-labelledby', row.id);
      row.setAttribute('aria-controls', detail.id);
      item.append(row, detail);
      list.append(item);
      if (expanded) s.detail = detail;
    });
    if (s.draft) renderEditor(s);
    if (!notes.length) {
      const empty = element('div', 'ws-notes-empty');
      const icon = element('span', 'material-icons-round ws-notes-empty-icon', query ? 'search_off' : 'sticky_note_2');
      icon.setAttribute('aria-hidden', 'true');
      empty.append(icon,
        element('p', 'ws-notes-empty-text', query ? 'No notes match your search.' : 'No notes yet. Add one to track a task.'));
      list.append(empty);
    }
  }
  function dispose(el) {
    const s = states.get(el.dataset.notes);
    if (!s || s.el !== el) return;
    clearTimeout(s.saveTimer);s.editor?.destroy();states.delete(s.id);
  }
  function init() {
    const activeGrid = Array.from(document.querySelectorAll('[data-workspace-project]')).find(el => el.parentElement.style.display !== 'none');
    const workspace = activeGrid?.dataset.workspaceProject || '';
    const switched = activeWorkspace !== workspace;
    activeWorkspace = workspace;
    activeProject = activeGrid?.dataset.noteProject || '';
    for (const [id, s] of states) if (!s.el.isConnected || s.el.closest('[data-workspace-project]')?.dataset.noteProject !== s.project) dispose(s.el);
    document.querySelectorAll('[data-notes]').forEach(el => {
      const project = el.closest('[data-workspace-project]')?.dataset.noteProject;
      if (project !== activeProject) return;
      const existing = states.get(el.dataset.notes);
      if (existing) {
        if (switched) request(existing, 'list');
        return;
      }
      const s = {el, id:el.dataset.notes, project, notes:[], projects:[], requests:new Map(), search:'', busy:false, reading:0};
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
    if (action === 'save') s.saving = false;
    else if (action !== 'list') s.busy = false;
    if (s.draft && action !== 'list' && action !== 'save') {
      s.editor.setEditable(true);
      s.detail.querySelectorAll('input,textarea,select,button').forEach(el => { el.disabled = false; });
      updateActions(s);
    }
    if (result.error) {
      if (s.draft) updateActions(s);
      status(s, result.error);
      if (action === 'save') {
        clearTimeout(s.saveTimer);
        s.status.append(' ', button('Retry', () => autoSave(s)));
      }
      if (action === 'list' && !s.draft) s.el.replaceChildren(s.status, button('Retry', () => { s.el.replaceChildren(s.status); status(s, 'Loading notes…'); request(s, 'list'); }));
      return;
    }
    if (action === 'list') { s.notes = result.notes; s.projects = result.projects || []; if (!s.draft) renderList(s); }
    if (action === 'save') {
      s.notes = [result.note, ...s.notes.filter(note => note.id !== result.note.id)];
      // Keep the live draft and editor: typing may continue while a save is in flight.
      s.saved = {...s.savingNote, id:result.note.id, updated:result.note.updated};
      s.draft.id = result.note.id; s.draft.updated = result.note.updated;
      const row = s.detail.previousElementSibling;
      row.dataset.noteId = result.note.id;
      row.querySelector('.ws-note-row-title-line').textContent = result.note.title;
      row.querySelector('.ws-note-row-meta').textContent = shortID(result.note) + ' · updated ' + relativeTime(result.note.updated);
      changed(s);
    }
    if (action === 'delete') {
      s.notes = s.notes.filter(note => note.id !== result.noteID);
      collapse(s); status(s, 'Note deleted');
    }
    if (action === 'move') {
      s.notes = s.notes.filter(note => note.id !== result.noteID);
      s.draft = null; renderList(s); status(s, 'Note moved');
      s.el.querySelector('.ws-notes-toolbar button')?.focus();
      for (const other of states.values()) {
        if (other !== s && other.project === activeProject) request(other, 'list');
      }
    }
    if (action === 'send') {
      const active = s.project === activeProject;
      const sent = active && window.__libroSendPageToolPrompt?.(result.prompt, true);
      status(s, sent ? 'Sent to agent' : 'Start or select an agent in this project, then try again.');
    }
  }
  async function control(command) {
    const response = await fetch('/notes/agent', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({sid:window.__libroWorkspaceSID, command})
    });
    if (!response.ok) throw new Error('Note request failed: HTTP ' + response.status);
    const reply = await response.json();
    if (reply.error) throw new Error(reply.error);
    if (['create', 'set_status', 'delete'].includes(command.action)) {
      for (const s of states.values()) {
        if (s.el.isConnected && s.project === activeProject) request(s, 'list');
      }
    }
    return reply.result;
  }
  window.libroNotes = {init, receive, control, dispose};
})();
