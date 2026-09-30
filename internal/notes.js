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
    s.save.disabled = s.busy || s.reading > 0 || !s.draft.title.trim() || !dirty(s);
    s.send.disabled = s.busy || s.reading > 0 || !s.draft.id || dirty(s);
    s.cancel.disabled = s.busy || s.reading > 0;
    s.move.disabled = s.busy || s.reading > 0 || !s.draft.id || dirty(s) || !s.target.value;
    s.target.disabled = s.busy;
    s.el.querySelectorAll('.ws-notes-toolbar button,.ws-notes-search,.ws-note-row').forEach(el => { el.disabled = s.busy || s.reading > 0; });
  }
  function changed(s) { updateActions(s); status(s, dirty(s) ? 'Unsaved changes' : ''); }
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
  function canClose(s) {
    if (s.busy || s.reading > 0) return false;
    if (s.draft && dirty(s)) { status(s, 'Save or cancel your changes before closing this note.'); return false; }
    return true;
  }
  function collapse(s) {
    const id = s.draft.id;
    s.draft = null;
    renderList(s);
    (Array.from(s.el.querySelectorAll('[data-note-id]')).find(row => row.dataset.noteId === id) || s.el.querySelector('.ws-notes-new')).focus();
  }
  function edit(s, note) {
    if (!canClose(s)) return;
    s.saved = structuredClone(note);
    s.draft = structuredClone(note);
    s.saved.images ||= []; s.draft.images ||= [];
    renderList(s);
    (note.id ? s.detail.previousElementSibling : s.detail.querySelector('.ws-note-title')).focus();
  }
  function renderEditor(s) {
    const note = s.draft;
    s.reading = 0;
    const form = element('form', 'ws-note-editor');
    const header = element('div', 'ws-note-editor-header');
    const stateToggle = button('', () => {
      s.draft.state = s.draft.state === 'new' ? 'archived' : 'new';
      stateToggle.classList.toggle('ws-note-state-toggle-archived', s.draft.state === 'archived');
      stateToggle.setAttribute('aria-label', s.draft.state === 'new' ? 'Archive note' : 'Reopen note');
      stateToggle.title = stateToggle.getAttribute('aria-label');
      changed(s);
    }, 'ws-note-state-toggle' + (note.state === 'archived' ? ' ws-note-state-toggle-archived' : ''));
    stateToggle.setAttribute('aria-label', note.state === 'new' ? 'Archive note' : 'Reopen note');
    stateToggle.title = stateToggle.getAttribute('aria-label');
    const title = element('input', 'ws-note-title'); title.value = note.title; title.maxLength = 200; title.required = true;
    title.placeholder = 'Note title'; title.setAttribute('aria-label', 'Title');
    header.append(stateToggle, title);
    const body = element('div', 'ws-note-rich-editor');
    const hint = element('p', 'ws-note-hint', 'Use Markdown shortcuts or the toolbar. Paste screenshots anywhere in the text.');
    const description = element('section', 'ws-note-section');
    const descriptionContent = element('div', 'ws-note-section-content');
    description.append(element('span', 'material-icons-round ws-note-section-icon', 'subject'), descriptionContent);
    descriptionContent.append(element('h2', 'ws-note-section-title', 'Description'), body, hint);
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    const actionsSection = element('section', 'ws-note-section');
    const actionsContent = element('div', 'ws-note-section-content');
    actionsSection.append(element('span', 'material-icons-round ws-note-section-icon', 'bolt'), actionsContent);
    const actions = element('div', 'ws-note-actions');
    s.save = button('Save note', () => form.requestSubmit(), 'ws-notes-button ws-notes-primary');
    s.cancel = button('Cancel', () => collapse(s));
    s.send = button('Send to agent', () => {
      s.busy = true; updateActions(s); status(s, 'Sending…');
      request(s, 'send', {noteID:s.draft.id});
    });
    actions.append(s.save, s.send, s.cancel);
    const moveActions = element('div', 'ws-note-actions');
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
    moveActions.append(s.target, s.move);
    actionsContent.append(element('h2', 'ws-note-section-title', 'Actions'), actions, moveActions, s.status);
    form.append(header, description, actionsSection);
    s.detail.append(form);
    title.oninput = () => { s.draft.title = title.value; changed(s); };
    s.editor = libroNoteEditor.create({
      element: body, note: s.draft,
      onChange: value => { Object.assign(s.draft, value); changed(s); },
      onBusy: value => { s.reading = Number(value); updateActions(s); },
      onError: message => status(s, message),
    });
    form.onsubmit = event => {
      event.preventDefault(); if (s.save.disabled) return;
      Object.assign(s.draft, s.editor.serialize());
      if (new TextEncoder().encode(JSON.stringify(s.draft)).length > 9 * 1024 * 1024) { status(s, 'This note is too large. Remove an image or shorten the text.'); return; }
      s.busy = true; updateActions(s); status(s, 'Saving…');
      request(s, 'save', {note:s.draft});
      s.editor.setEditable(false);
      form.querySelectorAll('input,textarea,select,button').forEach(el => { el.disabled = true; });
    };
    s.editor.setEditable(!s.busy);
    updateActions(s);
  }
  function renderList(s) {
    s.editor?.destroy(); s.editor = null;
    s.el.replaceChildren();
    const heading = element('div', 'ws-notes-heading');
    const open = s.notes.filter(note => note.state === 'new').length;
    const archived = s.notes.length - open;
    heading.append(element('span', 'ws-notes-title', 'Notes · ' + s.project));
    const toolbar = element('div', 'ws-notes-toolbar');
    const tabs = element('div', 'ws-notes-tabs');
    for (const [value, label, count] of [['new', 'Open', open], ['archived', 'Archived', archived], ['all', 'All', s.notes.length]]) {
      const tab = button(label, () => { s.filter = value; s.search = s.el.querySelector('.ws-notes-search').value; renderList(s); }, 'ws-notes-tab' + (s.filter === value ? ' ws-notes-tab-active' : ''));
      tab.append(' ', element('span', 'ws-notes-tab-count', String(count)));
      tab.setAttribute('aria-pressed', String(s.filter === value));
      tabs.append(tab);
    }
    const search = element('input', 'ws-notes-search');
    search.type = 'search'; search.placeholder = 'Search notes…'; search.value = s.search; search.setAttribute('aria-label', 'Search notes');
    search.oninput = () => { s.search = search.value; listRows(s, list); };
    const actions = element('div', 'ws-notes-toolbar-actions');
    actions.append(search, button('New note', () => edit(s, {id:'', title:'', body:'', state:'new', images:[]}), 'ws-notes-button ws-notes-primary ws-notes-new'));
    toolbar.append(heading, tabs, actions);
    const list = element('div', 'ws-notes-list');
    s.status = element('div', 'ws-notes-status'); s.status.setAttribute('role', 'status');
    s.el.append(toolbar, list, s.status);
    listRows(s, list);
  }
  function listRows(s, list) {
    s.editor?.destroy(); s.editor = null;
    list.replaceChildren();
    const query = s.search.trim().toLowerCase();
    const notes = s.notes.filter(note => (s.filter === 'all' || note.state === s.filter) && (!query || note.title.toLowerCase().includes(query)));
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
      row.append(badge(note.state), main, chevron);
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
      empty.append(element('div', 'ws-notes-empty-icon', '✓'),
        element('p', 'ws-notes-empty-text', query ? 'No notes match your search.' : s.filter === 'archived' ? 'No archived notes.' : 'No open notes. Good job, or add one to track a task.'));
      list.append(empty);
    }
  }
  function init() {
    const activeGrid = Array.from(document.querySelectorAll('[data-workspace-project]')).find(el => el.parentElement.style.display !== 'none');
    const workspace = activeGrid?.dataset.workspaceProject || '';
    const switched = activeWorkspace !== workspace;
    activeWorkspace = workspace;
    activeProject = activeGrid?.dataset.noteProject || '';
    for (const [id, s] of states) if (!s.el.isConnected || s.el.closest('[data-workspace-project]')?.dataset.noteProject !== s.project) { s.editor?.destroy(); states.delete(id); }
    document.querySelectorAll('[data-notes]').forEach(el => {
      const project = el.closest('[data-workspace-project]')?.dataset.noteProject;
      if (project !== activeProject) return;
      const existing = states.get(el.dataset.notes);
      if (existing) {
        if (switched) request(existing, 'list');
        return;
      }
      const s = {el, id:el.dataset.notes, project, notes:[], projects:[], requests:new Map(), filter:'new', search:'', busy:false, reading:0};
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
    if (action !== 'list') s.busy = false;
    if (s.draft && action !== 'list') {
      s.editor.setEditable(true);
      s.detail.querySelectorAll('input,textarea,select,button').forEach(el => { el.disabled = false; });
      updateActions(s);
    }
    if (result.error) {
      status(s, result.error);
      if (action === 'list' && !s.draft) s.el.replaceChildren(s.status, button('Retry', () => { s.el.replaceChildren(s.status); status(s, 'Loading notes…'); request(s, 'list'); }));
      return;
    }
    if (action === 'list') { s.notes = result.notes; s.projects = result.projects || []; if (!s.draft) renderList(s); }
    if (action === 'save') {
      s.notes = [result.note, ...s.notes.filter(note => note.id !== result.note.id)];
      s.saved = structuredClone(result.note); s.draft = structuredClone(result.note);
      renderList(s); status(s, 'Saved');
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
  window.libroNotes = {init, receive, control};
})();
