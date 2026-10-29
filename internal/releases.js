(function () {
  const states = new Map();
  let sequence = 0;
  // Target commands can reach slow servers, so panels that remount on thread
  // switches reuse the last answer for a while instead of running them again.
  let cache = null;
  const element = (tag, cls, text) => {
    const el = document.createElement(tag); el.className = cls || '';
    if (text !== undefined) el.textContent = text;
    return el;
  };
  function icon(name) {
    const el = element('span', 'material-icons-round', name); el.setAttribute('aria-hidden', 'true'); return el;
  }
  function button(label, glyph, action) {
    const el = element('button', 'ws-repos-button'); el.type = 'button'; el.title = label; el.onclick = action;
    el.append(icon(glyph), label);
    return el;
  }
  function request(s, body = {}) {
    const token = String(++sequence);
    s.request = token; s.busy = true; s.saving = Boolean(body.save); render(s);
    fetch('/releases/action', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({sid:window.__libroWorkspaceSID, id:s.id, request:token, ...body})
    }).then(response => {
      if (!response.ok) throw new Error('Could not read releases. Try again.');
      return response.json();
    }).then(receive).catch(error => receive({id:s.id, request:token, path:body.path || '', error:error.message}));
  }
  function receive(result) {
    const s = states.get(result.id);
    if (!s || !s.el.isConnected || result.request !== s.request) return;
    s.busy = false;
    if (s.editing && result.path === s.editing.path) {
      s.editing.error = result.error || '';
      if (!result.error) s.editing = null;
    } else s.error = result.error || '';
    if (!result.error) {
      // A save rereads only its own project.
      s.releases = result.path ? (s.releases || []).map(release => release.path === result.path ? result.releases[0] : release) : result.releases;
      s.checked = new Date();
      cache = {releases:s.releases, checked:s.checked};
    }
    render(s);
  }
  function plural(count, word, many = word + 's') { return count + ' ' + (count === 1 ? word : many); }
  function folder(path) { return path.split(/[\\/]/).pop(); }
  // Rows j/k move between; the key keeps the cursor on the same row across refreshes.
  function item(el, key) { el.dataset.reposItem = key; return el; }
  function badge(text, kind) { return element('span', 'ws-repos-badge ws-repos-' + kind, text); }
  function needsAttention(release) {
    return Boolean(release.error || release.targets.some(target => target.error || !target.known || target.behind || target.ahead));
  }
  function targetRow(s, release, target, index) {
    const key = release.path + ':' + index;
    const missing = target.missing.length > 0;
    const row = item(element(missing ? 'summary' : 'li', missing ? '' : 'ws-repos-branch'), (missing ? 'm:' : 't:') + key);
    const line = missing ? element('span', 'ws-repos-branch ws-releases-inline') : row;
    const counts = element('span', 'ws-repos-counts');
    let version = target.version || target.output;
    if (target.error) { counts.append(badge('not available', 'problem')); version = target.error; }
    else if (!target.known) counts.append(badge('unknown version', 'muted'));
    else {
      if (target.behind) counts.append(badge(plural(target.behind, 'commit') + ' missing', 'changed'));
      if (target.ahead) counts.append(badge(plural(target.ahead, 'commit') + ' not on ' + (release.branch || 'this branch'), 'pull'));
      if (!target.behind && !target.ahead) counts.append(badge('latest', 'ok'));
    }
    line.append(element('span', 'ws-repos-branch-name', target.name), element('span', 'ws-repos-muted ws-releases-version', version), counts);
    row.title = target.command;
    if (!missing) return row;
    row.append(line);
    const details = element('details', 'ws-repos-changes');
    details.open = s.open.get(key) || false;
    details.ontoggle = () => s.open.set(key, details.open);
    const list = element('ul', 'ws-repos-files');
    target.missing.forEach((subject, i) => list.append(item(element('li', '', subject), 'c:' + key + ':' + i)));
    if (target.behind > target.missing.length) list.append(element('li', 'ws-repos-muted', 'and ' + plural(target.behind - target.missing.length, 'more commit')));
    details.append(row, list);
    const wrapper = element('li', '');
    wrapper.append(details);
    return wrapper;
  }
  function editForm(s) {
    const edit = s.editing;
    const form = element('form', 'ws-releases-form');
    form.onsubmit = event => { event.preventDefault(); save(s); };
    edit.targets.forEach((target, index) => {
      const row = element('div', 'ws-releases-row');
      ['name', 'command'].forEach(field => {
        const input = element('input', 'ws-releases-input ws-releases-' + field);
        input.value = target[field]; input.dataset.field = field + ':' + index;
        input.placeholder = field === 'name' ? ['Installed', 'Stage', 'Production'][index] || 'Name' : 'Command that prints the version';
        input.setAttribute('aria-label', (field === 'name' ? 'Target name ' : 'Version command ') + (index + 1));
        input.oninput = () => { target[field] = input.value; };
        row.append(input);
      });
      const remove = element('button', 'ws-button'); remove.type = 'button'; remove.title = remove.ariaLabel = 'Remove target'; remove.append(icon('close'));
      remove.onclick = () => { edit.targets.splice(index, 1); render(s); };
      row.append(remove);
      form.append(row);
    });
    form.append(element('p', 'ws-repos-muted', 'Each command runs in the project folder and prints the version there: a tag like v1.2.0, a version like 1.2.0, or a commit. For example: libro --version, ssh prod cat /srv/app/VERSION, or curl -s https://stage.example.com/version'));
    if (edit.error) form.append(element('p', 'ws-repos-error', edit.error));
    const actions = element('div', 'ws-repos-actions ws-releases-form-actions');
    const add = button('Add target', 'add', () => { edit.targets.push({name:'', command:''}); edit.focus = 'name:' + (edit.targets.length - 1); render(s); });
    add.disabled = edit.targets.length >= 10;
    const cancel = button('Cancel', 'close', () => cancelEdit(s));
    const submit = button('Save', 'check', null); submit.type = 'submit'; submit.disabled = s.busy;
    actions.append(add, cancel, submit);
    form.append(actions);
    return form;
  }
  function project(s, release) {
    const card = element('section', 'ws-repos-project');
    card.dataset.attention = String(needsAttention(release));
    const head = item(element('div', 'ws-repos-project-head'), 'p:' + release.path);
    head.dataset.project = release.name;
    const title = element('div', 'ws-repos-project-title');
    const latest = release.tag ? release.tag + (release.sinceTag ? ' + ' + plural(release.sinceTag, 'commit') : '') : release.head;
    title.append(element('span', 'ws-repos-name', folder(release.path)), element('span', 'ws-repos-muted ws-repos-folder', [release.branch, latest].filter(Boolean).join(' · ')));
    const badges = element('div', 'ws-repos-counts');
    const behind = release.targets.filter(target => target.behind).length;
    if (behind) badges.append(badge(plural(behind, 'target') + ' behind', 'changed'));
    if (!release.targets.length && !release.error) badges.append(badge('no targets', 'muted'));
    else if (!needsAttention(release)) badges.append(badge('all latest', 'ok'));
    const edit = element('button', 'ws-button'); edit.type = 'button'; edit.title = edit.ariaLabel = 'Edit targets (e)'; edit.append(icon('edit'));
    edit.onclick = () => startEdit(s, release);
    head.append(title, badges, edit);
    card.append(head);
    if (release.error) card.append(element('p', 'ws-repos-error', release.error));
    if (s.editing?.path === release.path) card.append(editForm(s));
    else if (release.targets.length) {
      const list = element('ul', 'ws-repos-branches');
      release.targets.forEach((target, index) => list.append(targetRow(s, release, target, index)));
      card.append(list);
    }
    return card;
  }
  function startEdit(s, release) {
    const targets = release.targets.map(target => ({name:target.name, command:target.command}));
    if (!targets.length) targets.push({name:'', command:''});
    s.editing = {path:release.path, targets, error:'', focus:'name:0'};
    s.cursor = 'p:' + release.path;
    render(s);
  }
  function cancelEdit(s) {
    s.editing = null; render(s);
    select(s, s.cursor);
  }
  function save(s) {
    if (s.busy) return;
    request(s, {path:s.editing.path, save:true, targets:s.editing.targets.filter(target => target.name.trim() || target.command.trim())});
  }
  function render(s) {
    const scroll = s.el.scrollTop;
    // Controls are rebuilt, so keep keyboard focus, and the caret in a field.
    const active = document.activeElement;
    const focused = s.el.contains(active);
    const field = focused && active.dataset.field ? {name:active.dataset.field, start:active.selectionStart, end:active.selectionEnd} : null;
    s.el.replaceChildren();
    const toolbar = element('div', 'ws-repos-toolbar');
    const heading = element('div', 'ws-repos-heading');
    const releases = s.releases || [];
    const configured = releases.filter(release => release.targets.length);
    const attention = releases.filter(needsAttention);
    heading.append(element('span', 'ws-repos-title', 'Releases'),
      element('span', 'ws-repos-muted', s.releases ? attention.length + ' of ' + configured.length + ' need attention' : ''));
    const actions = element('div', 'ws-repos-actions');
    const all = element('label', 'ws-repos-toggle');
    const box = element('input', ''); box.type = 'checkbox'; box.checked = s.showAll;
    box.onchange = () => { s.showAll = box.checked; render(s); };
    all.append(box, element('span', '', 'Show all'));
    const refresh = button('Refresh', 'refresh', () => request(s));
    refresh.disabled = s.busy;
    const keys = element('button', 'ws-button'); keys.type = 'button'; keys.title = keys.ariaLabel = 'Keyboard shortcuts';
    keys.setAttribute('aria-expanded', String(s.help)); keys.append(icon('keyboard'));
    keys.onclick = () => { s.help = !s.help; render(s); };
    actions.append(all, refresh, keys);
    toolbar.append(heading, actions);
    const status = element('div', 'ws-repos-status'); status.setAttribute('role', 'status');
    if (s.error) status.textContent = s.error;
    else if (s.busy) status.textContent = s.saving ? 'Saving targets…' : 'Checking targets…';
    else if (s.checked) status.textContent = 'Checked at ' + s.checked.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'});
    s.el.append(toolbar, status);
    if (s.help) s.el.append(element('div', 'ws-repos-keys', 'j/k move · gg/G first/last · {/} project · l/Enter open missing commits · h close · e edit targets · o open project · r refresh · a show all · ? keys'));
    const list = element('div', 'ws-repos-list');
    // Until a target exists, list every project so one can be added.
    const showAll = s.showAll || !configured.length;
    releases.slice().sort((a, b) => needsAttention(b) - needsAttention(a) || Boolean(b.targets.length) - Boolean(a.targets.length) || folder(a.path).localeCompare(folder(b.path)))
      .filter(release => showAll || release.targets.length || s.editing?.path === release.path)
      .forEach(release => list.append(project(s, release)));
    if (s.releases && !configured.length) s.el.append(element('p', 'ws-repos-muted ws-releases-intro', 'Add targets to a project, like Installed, Stage or Production. Each one runs a command that prints the version there, and Libro shows the commits it is missing.'));
    if (s.releases && !list.children.length) {
      const empty = element('div', 'ws-repos-empty');
      empty.append(icon('folder_off'), element('p', 'ws-repos-muted', 'No Git projects yet. Add a project to see it here.'));
      list.append(empty);
    }
    s.el.append(list);
    s.el.scrollTop = scroll;
    s.el.querySelectorAll('details').forEach(details => details.addEventListener('toggle', () => {
      const summary = details.querySelector('summary');
      select(s, !details.open && details.querySelector('[data-repos-item="' + CSS.escape(s.cursor) + '"]') ? summary.dataset.reposItem : s.cursor, false, s.el.contains(document.activeElement));
    }));
    const target = s.editing?.focus || field?.name;
    const input = target && s.el.querySelector('[data-field="' + CSS.escape(target) + '"]');
    if (input) {
      if (s.editing) s.editing.focus = '';
      select(s, s.cursor, false, false);
      input.focus({preventScroll:true});
      if (field?.name === target) input.setSelectionRange(field.start, field.end);
    } else select(s, s.cursor, false, focused);
  }
  function items(s) {
    return Array.from(s.el.querySelectorAll('[data-repos-item]')).filter(el => el.tagName === 'SUMMARY' || !el.closest('details:not([open])'));
  }
  // The cursor row holds the real focus, so screen readers announce it.
  function select(s, key, scroll = true, focus = scroll) {
    const rows = items(s);
    const current = rows.find(row => row.dataset.reposItem === key) || rows[0];
    rows.forEach(row => { row.tabIndex = row === current ? 0 : -1; row.classList.toggle('is-current', row === current); });
    s.cursor = current?.dataset.reposItem || '';
    s.el.tabIndex = current ? -1 : 0;
    if (current && scroll) current.scrollIntoView({block:'nearest'});
    if (focus) (current || s.el).focus({preventScroll:true});
  }
  function move(s, rows, index) {
    if (rows.length) select(s, rows[Math.max(0, Math.min(rows.length - 1, index))].dataset.reposItem);
  }
  function keydown(s, event) {
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    // Typing in the target form stays in the form; Escape leaves it.
    if (event.target.closest('form')) {
      if (event.key === 'Escape') { cancelEdit(s); event.preventDefault(); event.stopPropagation(); }
      return;
    }
    const control = event.target.closest('button,input,summary');
    if (control && control !== s.el && (event.key === 'Enter' || event.key === ' ')) return;
    const rows = items(s);
    const index = rows.findIndex(row => row.dataset.reposItem === s.cursor);
    const current = rows[index];
    const details = current?.closest('details');
    const key = event.key;
    const card = current?.closest('.ws-repos-project')?.querySelector('[data-project]');
    if (key === 'g') { if (Date.now() - (s.lastG || 0) < 500) move(s, rows, 0); s.lastG = Date.now(); }
    else if (key === 'G' || key === 'End') move(s, rows, rows.length - 1);
    else if (key === 'Home') move(s, rows, 0);
    else if (key === 'j' || key === 'ArrowDown') move(s, rows, index + 1);
    else if (key === 'k' || key === 'ArrowUp') move(s, rows, index - 1);
    else if (key === '}' || key === '{') {
      const heads = rows.map((row, i) => [row, i]).filter(([row]) => row.dataset.reposItem.startsWith('p:'));
      const target = key === '}' ? heads.find(([, i]) => i > index) : heads.reverse().find(([, i]) => i < index);
      if (target) move(s, rows, target[1]);
    } else if ((key === 'l' || key === 'ArrowRight' || key === 'Enter') && current?.tagName === 'SUMMARY') {
      details.open = key === 'Enter' ? !details.open : true;
    } else if (key === 'h' || key === 'ArrowLeft') {
      if (details?.open) { details.open = false; select(s, details.querySelector('summary').dataset.reposItem); }
      else select(s, card?.dataset.reposItem);
    } else if (key === 'e' && card) {
      const release = s.releases?.find(release => 'p:' + release.path === card.dataset.reposItem);
      if (release) startEdit(s, release);
    } else if (key === 'o') {
      if (card) window.libroWorkspace?.openProject(card.dataset.project);
    } else if (key === 'r' && !s.busy) request(s);
    else if (key === 'a') { s.showAll = !s.showAll; render(s); }
    else if (key === '?') { s.help = !s.help; render(s); }
    else return;
    event.preventDefault(); event.stopPropagation();
  }
  function dispose(el) {
    const s = states.get(el.dataset.releases);
    if (s && s.el === el) states.delete(s.id);
  }
  function init() {
    for (const s of states.values()) if (!s.el.isConnected) dispose(s.el);
    document.querySelectorAll('[data-releases]').forEach(el => {
      if (states.get(el.dataset.releases)?.el === el) return;
      const s = {el, id:el.dataset.releases, releases:null, showAll:false, busy:false, open:new Map(), cursor:'', help:false, editing:null};
      states.set(s.id, s);
      el.tabIndex = 0;
      el.setAttribute('role', 'region');
      el.setAttribute('aria-label', 'Releases. Press ? for keyboard shortcuts.');
      el.onkeydown = event => keydown(s, event);
      el.onclick = event => { const row = event.target.closest('[data-repos-item]'); if (row) select(s, row.dataset.reposItem, false); };
      if (cache && Date.now() - cache.checked < 5 * 60 * 1000) { s.releases = cache.releases; s.checked = cache.checked; render(s); }
      else request(s);
    });
  }
  window.libroReleases = {init, receive, dispose};
})();
