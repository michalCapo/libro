(function () {
  const states = new Map();
  let sequence = 0;
  // Panels remount on thread switches; fetch remotes again only after a while,
  // and never while another panel's fetch is still running.
  let lastFetch = 0;
  let remoteFetch = false;
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
  function request(s, fetchRemotes) {
    const token = String(++sequence);
    s.request = token; s.fetching = fetchRemotes; s.busy = true; render(s);
    if (fetchRemotes) remoteFetch = true;
    fetch('/repos/action', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({sid:window.__libroWorkspaceSID, id:s.id, request:token, fetch:fetchRemotes})
    }).then(response => {
      if (!response.ok) throw new Error('Could not read repositories. Try again.');
      return response.json();
    }).then(receive).catch(error => receive({id:s.id, request:token, error:error.message})).finally(() => {
      if (fetchRemotes) { remoteFetch = false; lastFetch = Date.now(); }
    });
  }
  function receive(result) {
    const s = states.get(result.id);
    if (!s || !s.el.isConnected || result.request !== s.request) return;
    const fetched = s.fetching;
    s.busy = false; s.fetching = false; s.error = result.error || '';
    if (!result.error) { s.repos = result.repos; s.checked = new Date(); }
    render(s);
    // Show local state at once, then fetch so behind counts are current.
    if (!result.error && !fetched && !s.fetched && !remoteFetch && Date.now() - lastFetch > 5 * 60 * 1000) { s.fetched = true; request(s, true); }
  }
  function relativeTime(value) {
    const time = Date.parse(value);
    if (Number.isNaN(time)) return '';
    const seconds = Math.max(1, Math.round((Date.now() - time) / 1000));
    const units = [[31536000, 'y'], [2592000, 'mo'], [604800, 'w'], [86400, 'd'], [3600, 'h'], [60, 'm']];
    for (const [span, label] of units) if (seconds >= span) return Math.floor(seconds / span) + label + ' ago';
    return 'just now';
  }
  function plural(count, word, many = word + 's') { return count + ' ' + (count === 1 ? word : many); }
  function folder(path) { return path.split(/[\\/]/).pop(); }
  // Rows j/k move between; the key keeps the cursor on the same row across refreshes.
  function item(el, key) { el.dataset.reposItem = key; return el; }
  function pendingBranches(repo) {
    return repo.branches.filter(branch => branch.ahead || branch.behind || branch.gone);
  }
  function summary(repo) {
    const changed = repo.worktrees.reduce((sum, worktree) => sum + worktree.changes.length + worktree.more, 0);
    const branches = pendingBranches(repo);
    return {
      changed,
      push: branches.filter(branch => branch.ahead).length,
      pull: branches.filter(branch => branch.behind).length,
      gone: branches.filter(branch => branch.gone).length,
      problem: Boolean(repo.error || repo.fetchError || repo.worktrees.some(worktree => worktree.error)),
    };
  }
  function needsAttention(counts) { return Boolean(counts.changed || counts.push || counts.pull || counts.gone || counts.problem); }
  function badge(text, kind) { return element('span', 'ws-repos-badge ws-repos-' + kind, text); }
  function branchRow(repo, branch) {
    const row = item(element('li', 'ws-repos-branch'), 'b:' + repo.path + ':' + branch.name);
    const name = element('span', 'ws-repos-branch-name', branch.name);
    const target = element('span', 'ws-repos-muted', branch.upstream ? '→ ' + branch.upstream : 'not on any remote');
    const counts = element('span', 'ws-repos-counts');
    if (branch.ahead) counts.append(badge('↑ ' + plural(branch.ahead, 'commit') + ' to push', 'push'));
    if (branch.behind) counts.append(badge('↓ ' + plural(branch.behind, 'commit') + ' to pull', 'pull'));
    if (branch.gone) counts.append(badge('remote branch deleted', 'problem'));
    row.append(name, target, counts);
    return row;
  }
  function worktreeChanges(s, repo, worktree) {
    const count = worktree.changes.length + worktree.more;
    const details = element('details', 'ws-repos-changes');
    details.open = s.open.has(worktree.path) ? s.open.get(worktree.path) : count <= 8;
    details.ontoggle = () => s.open.set(worktree.path, details.open);
    const head = item(element('summary', ''), 'w:' + worktree.path);
    const where = worktree.path === repo.path ? '' : ' · ' + folder(worktree.path);
    head.append(badge(plural(count, 'uncommitted file'), 'changed'), element('span', 'ws-repos-muted', ' on ' + (worktree.branch || 'detached HEAD') + where));
    const list = element('ul', 'ws-repos-files');
    worktree.changes.forEach(change => {
      const row = item(element('li', ''), 'f:' + worktree.path + ':' + change.path);
      row.append(element('span', 'ws-repos-code', change.code === '??' ? 'new' : change.code), element('span', 'ws-repos-path', change.path));
      list.append(row);
    });
    if (worktree.more) list.append(element('li', 'ws-repos-muted', 'and ' + plural(worktree.more, 'more file')));
    details.append(head, list);
    return details;
  }
  function project(s, repo, counts) {
    const card = element('section', 'ws-repos-project');
    card.dataset.attention = String(needsAttention(counts));
    const head = item(element('div', 'ws-repos-project-head'), 'p:' + repo.path);
    head.dataset.project = repo.name;
    const title = element('div', 'ws-repos-project-title');
    title.append(element('span', 'ws-repos-name', folder(repo.path)), element('span', 'ws-repos-muted ws-repos-folder', repo.path));
    const badges = element('div', 'ws-repos-counts');
    if (counts.changed) badges.append(badge(plural(counts.changed, 'file') + ' changed', 'changed'));
    if (counts.push) badges.append(badge(plural(counts.push, 'branch', 'branches') + ' to push', 'push'));
    if (counts.pull) badges.append(badge(plural(counts.pull, 'branch', 'branches') + ' to pull', 'pull'));
    if (!repo.remote && !repo.error) badges.append(badge('no remote', 'muted'));
    if (!needsAttention(counts) && repo.remote) badges.append(badge('up to date', 'ok'));
    head.append(title, badges);
    card.append(head);
    if (repo.error) card.append(element('p', 'ws-repos-error', repo.error));
    if (repo.fetchError) card.append(element('p', 'ws-repos-error', 'Fetch failed: ' + repo.fetchError));
    repo.worktrees.forEach(worktree => {
      if (worktree.error) card.append(element('p', 'ws-repos-error', worktree.path + ': ' + worktree.error));
      else if (worktree.changes.length || worktree.more) card.append(worktreeChanges(s, repo, worktree));
    });
    const branches = pendingBranches(repo);
    if (branches.length) {
      const list = element('ul', 'ws-repos-branches');
      branches.forEach(branch => list.append(branchRow(repo, branch)));
      card.append(list);
    }
    if (repo.remote) card.append(element('div', 'ws-repos-muted ws-repos-fetched', repo.fetched ? 'Remote checked ' + relativeTime(repo.fetched) : 'Never fetched'));
    return card;
  }
  function render(s) {
    const scroll = s.el.scrollTop;
    // Controls are rebuilt, so keep keyboard focus in the panel.
    const focused = s.el.contains(document.activeElement);
    s.el.replaceChildren();
    const toolbar = element('div', 'ws-repos-toolbar');
    const heading = element('div', 'ws-repos-heading');
    const repos = s.repos || [];
    const rows = repos.map(repo => ({repo, counts:summary(repo)}));
    const attention = rows.filter(row => needsAttention(row.counts));
    heading.append(element('span', 'ws-repos-title', 'Repositories'),
      element('span', 'ws-repos-muted', s.repos ? attention.length + ' of ' + repos.length + ' need attention' : ''));
    const actions = element('div', 'ws-repos-actions');
    const all = element('label', 'ws-repos-toggle');
    const box = element('input', ''); box.type = 'checkbox'; box.checked = s.showAll;
    box.onchange = () => { s.showAll = box.checked; render(s); };
    all.append(box, element('span', '', 'Show all'));
    const refresh = button('Refresh', 'refresh', () => request(s, false));
    const fetchButton = button('Fetch', 'cloud_download', () => request(s, true));
    refresh.disabled = fetchButton.disabled = s.busy;
    const keys = element('button', 'ws-button'); keys.type = 'button'; keys.title = keys.ariaLabel = 'Keyboard shortcuts';
    keys.setAttribute('aria-expanded', String(s.help)); keys.append(icon('keyboard'));
    keys.onclick = () => { s.help = !s.help; render(s); };
    actions.append(all, refresh, fetchButton, keys);
    toolbar.append(heading, actions);
    const status = element('div', 'ws-repos-status'); status.setAttribute('role', 'status');
    if (s.error) status.textContent = s.error;
    else if (s.busy) status.textContent = s.fetching ? 'Fetching remotes…' : 'Checking repositories…';
    else if (s.checked) status.textContent = 'Checked at ' + s.checked.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'});
    s.el.append(toolbar, status);
    if (s.help) s.el.append(element('div', 'ws-repos-keys', 'j/k move · gg/G first/last · {/} project · l/Enter open files · h close · o open project · r refresh · f fetch · a show all · ? keys'));
    const list = element('div', 'ws-repos-list');
    rows.sort((a, b) => needsAttention(b.counts) - needsAttention(a.counts) || folder(a.repo.path).localeCompare(folder(b.repo.path)));
    rows.filter(row => s.showAll || needsAttention(row.counts)).forEach(row => list.append(project(s, row.repo, row.counts)));
    if (s.repos && !list.children.length) {
      const empty = element('div', 'ws-repos-empty');
      empty.append(icon(repos.length ? 'task_alt' : 'folder_off'),
        element('p', 'ws-repos-muted', repos.length ? 'Everything is committed and pushed.' : 'No Git projects yet. Add a project to see it here.'));
      list.append(empty);
    }
    s.el.append(list);
    s.el.scrollTop = scroll;
    // A closed group hides its files; keep the cursor on the group instead.
    s.el.querySelectorAll('details').forEach(details => details.addEventListener('toggle', () => {
      const summary = details.querySelector('summary');
      select(s, !details.open && details.querySelector('[data-repos-item="' + CSS.escape(s.cursor) + '"]') ? summary.dataset.reposItem : s.cursor, false, s.el.contains(document.activeElement));
    }));
    select(s, s.cursor, false, focused);
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
    const control = event.target.closest('button,input,summary');
    if (control && control !== s.el && (event.key === 'Enter' || event.key === ' ')) return;
    const rows = items(s);
    const index = rows.findIndex(row => row.dataset.reposItem === s.cursor);
    const current = rows[index];
    const details = current?.closest('details');
    const key = event.key;
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
      else select(s, current?.closest('.ws-repos-project')?.querySelector('[data-repos-item^="p:"]')?.dataset.reposItem);
    } else if (key === 'o') {
      const name = current?.closest('.ws-repos-project')?.querySelector('[data-project]')?.dataset.project;
      if (name) window.libroWorkspace?.openProject(name);
    } else if (key === 'r' && !s.busy) request(s, false);
    else if (key === 'f' && !s.busy) request(s, true);
    else if (key === 'a') { s.showAll = !s.showAll; render(s); }
    else if (key === '?') { s.help = !s.help; render(s); }
    else return;
    event.preventDefault(); event.stopPropagation();
  }
  function dispose(el) {
    const s = states.get(el.dataset.repos);
    if (s && s.el === el) states.delete(s.id);
  }
  function init() {
    for (const s of states.values()) if (!s.el.isConnected) dispose(s.el);
    document.querySelectorAll('[data-repos]').forEach(el => {
      if (states.get(el.dataset.repos)?.el === el) return;
      const s = {el, id:el.dataset.repos, repos:null, showAll:false, busy:false, fetched:false, open:new Map(), cursor:'', help:false};
      states.set(s.id, s);
      el.tabIndex = 0;
      el.setAttribute('role', 'region');
      el.setAttribute('aria-label', 'Repositories. Press ? for keyboard shortcuts.');
      el.onkeydown = event => keydown(s, event);
      el.onclick = event => { const row = event.target.closest('[data-repos-item]'); if (row) select(s, row.dataset.reposItem, false); };
      request(s, false);
    });
  }
  window.libroRepos = {init, receive, dispose};
})();
