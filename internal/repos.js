(function () {
  const states = new Map();
  let sequence = 0;
  // Panels remount on thread switches; fetch remotes again only after a while,
  // and never while another panel's fetch is still running.
  let lastFetch = 0;
  let remoteFetch = false;
  // Release target commands can reach slow servers, so remounted panels reuse
  // the last answer for a while instead of running them again.
  let releaseCache = null;
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
  // A change runs one Git action first; the answer is the status after it.
  function request(s, fetchRemotes, change) {
    const token = String(++sequence);
    s.request = token; s.fetching = fetchRemotes; s.changing = change?.change || ''; s.busy = true; render(s);
    if (fetchRemotes) remoteFetch = true;
    fetch('/repos/action', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({sid:window.__libroWorkspaceSID, id:s.id, request:token, fetch:fetchRemotes, ...change})
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
    const fetched = s.fetching, changed = s.changing;
    s.busy = false; s.fetching = false; s.changing = ''; s.error = result.error || '';
    if (!result.error) { s.repos = result.repos; s.checked = new Date(); }
    if (changed) {
      const failed = result.changeError || result.error || '';
      if (s.change) s.change.error = failed; else s.error = failed;
      if (!failed) s.change = null;
    }
    if (result.agent) askAgent(s, result.agent);
    render(s);
    // Show local state at once, then fetch so behind counts are current.
    if (!result.error && !fetched && !s.fetched && !remoteFetch && Date.now() - lastFetch > 5 * 60 * 1000) { s.fetched = true; request(s, true); }
  }
  // Releases load on their own, so slow target commands never hold up the Git status.
  function releaseRequest(s, body = {}) {
    const token = String(++sequence);
    s.releaseToken = token; s.releasesBusy = true; s.saving = Boolean(body.save); render(s);
    fetch('/releases/action', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body:JSON.stringify({sid:window.__libroWorkspaceSID, id:s.id, request:token, ...body})
    }).then(response => {
      if (!response.ok) throw new Error('Could not read release targets. Try again.');
      return response.json();
    }).then(receiveReleases).catch(error => receiveReleases({id:s.id, request:token, path:body.path || '', error:error.message}));
  }
  function receiveReleases(result) {
    const s = states.get(result.id);
    if (!s || !s.el.isConnected || result.request !== s.releaseToken) return;
    s.releasesBusy = false;
    if (s.editing && result.path === s.editing.path) {
      s.editing.error = result.error || '';
      if (!result.error) s.editing = null;
    } else s.releaseError = result.error || '';
    if (!result.error) {
      // A save rereads only its own project.
      s.releases = result.path ? (s.releases || []).map(release => release.path === result.path ? result.releases[0] : release) : result.releases;
      releaseCache = {releases:s.releases, checked:Date.now()};
    }
    render(s);
  }
  function refresh(s) {
    if (!s.busy) request(s, false);
    if (!s.releasesBusy) releaseRequest(s);
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
  function releaseAttention(release) {
    return Boolean(release && (release.error || release.targets.some(target => target.error || !target.known || target.behind || target.ahead)));
  }
  function badge(text, kind) { return element('span', 'ws-repos-badge ws-repos-' + kind, text); }
  // Branch actions run in the worktree that has the branch checked out.
  function branchWorktree(repo, branch) {
    return repo.worktrees.find(worktree => worktree.branch === branch.name);
  }
  function pushBranch(s, repo, branch) {
    if (branch.ahead) act(s, {change:'push', worktree:(branchWorktree(repo, branch) || repo.worktrees[0]).path, branch:branch.name});
  }
  function pullBranch(s, repo, branch) {
    if (branch.behind) act(s, {change:'pull', worktree:(branchWorktree(repo, branch) || repo.worktrees[0]).path, branch:branch.name});
  }
  function startBranchChange(s, repo, branch, kind) {
    const worktree = branchWorktree(repo, branch);
    const options = [...new Set(repo.branches.flatMap(other => [other.name, other.upstream]).filter(name => name && name !== branch.name))];
    const main = repo.worktrees[0]?.branch;
    const onto = branch.behind && branch.upstream ? branch.upstream : options.includes(main) ? main : options[0];
    if (!worktree || !onto) return;
    s.change = {kind, repo:repo.path, branch:branch.name, worktree:worktree.path, options, onto, error:'', focus:'onto'};
    s.cursor = 'b:' + repo.path + ':' + branch.name;
    render(s);
  }
  function small(label, glyph, action, disabled) {
    const el = button(label, glyph, action); el.classList.add('ws-repos-small'); el.tabIndex = -1; el.disabled = disabled;
    return el;
  }
  function branchRow(s, repo, branch) {
    const row = item(element('li', 'ws-repos-branch'), 'b:' + repo.path + ':' + branch.name);
    row.dataset.branch = branch.name;
    const name = element('span', 'ws-repos-branch-name', branch.name);
    const target = element('span', 'ws-repos-muted', branch.upstream ? '→ ' + branch.upstream : 'not on any remote');
    const counts = element('span', 'ws-repos-counts');
    if (branch.ahead) counts.append(badge('↑ ' + plural(branch.ahead, 'commit') + ' to push', 'push'));
    if (branch.behind) counts.append(badge('↓ ' + plural(branch.behind, 'commit') + ' to pull', 'pull'));
    if (branch.gone) counts.append(badge('remote branch deleted', 'problem'));
    const actions = element('span', 'ws-repos-row-actions');
    const busy = Boolean(s.changing);
    if (branch.behind) actions.append(small('Pull', 'download', () => pullBranch(s, repo, branch), busy));
    if (branch.ahead) actions.append(small('Push', 'upload', () => pushBranch(s, repo, branch), busy));
    if (branchWorktree(repo, branch)) actions.append(small('Merge', 'call_merge', () => startBranchChange(s, repo, branch, 'merge'), busy),
      small('Rebase', 'low_priority', () => startBranchChange(s, repo, branch, 'rebase'), busy));
    row.append(name, target, counts, actions);
    return row;
  }
  // Commit and discard act on the checked files, or on all files when none is checked.
  function picked(s, worktree) {
    const set = s.selected.get(worktree.path) || new Set();
    const result = new Set(worktree.changes.map(change => change.path).filter(path => set.has(path)));
    s.selected.set(worktree.path, result);
    return result;
  }
  function toggleFile(s, worktree, path) {
    const set = picked(s, worktree);
    if (path === undefined) {
      if (set.size === worktree.changes.length) set.clear(); else worktree.changes.forEach(change => set.add(change.path));
    } else if (!set.delete(path)) set.add(path);
    render(s);
  }
  function startFileChange(s, worktree, kind) {
    const paths = [...picked(s, worktree)];
    const count = paths.length || worktree.changes.length + worktree.more;
    if (!count && kind !== 'abort') return;
    s.change = {kind, worktree:worktree.path, operation:worktree.operation, paths, count, message:'', error:'', focus:kind === 'commit' ? 'message' : 'confirm'};
    render(s);
  }
  function act(s, change) {
    if (!s.changing) request(s, false, change);
  }
  // The agent of the folder's thread or project gets the request, like a failed thread merge.
  function askAgent(s, request) {
    if (!request.appID || !window.__libroSendPageToolPrompt?.(request.prompt, true, request.appID)) {
      s.error = 'Start an agent in this folder, then try again.'; render(s); return;
    }
    window.__libroShowToast?.('Conflicts sent to the agent', '', 'success');
    window.libroWorkspace?.openProject(request.workspace);
  }
  function submitChange(s) {
    const change = s.change;
    if (s.changing) return;
    if (change.kind === 'commit' && !change.message.trim()) { change.error = 'Write a commit message'; change.focus = 'message'; render(s); return; }
    request(s, false, {change:change.kind, worktree:change.worktree, paths:change.paths || [], all:!change.paths?.length, message:change.message, branch:change.branch || '', onto:change.onto || ''});
  }
  function cancelChange(s) {
    s.change = null; render(s);
    select(s, s.cursor);
  }
  function changeForm(s, change) {
    const form = element('form', 'ws-releases-form ws-repos-change');
    form.onsubmit = event => { event.preventDefault(); submitChange(s); };
    const files = (change.paths?.length ? '' : 'all ') + plural(change.count, 'file');
    const labels = {commit:'Commit ' + plural(change.count, 'file'), discard:'Discard', abort:'Abort ' + change.operation, merge:'Merge', rebase:'Rebase'};
    if (change.kind === 'commit') {
      const input = element('input', 'ws-releases-input ws-releases-name'); input.value = change.message; input.dataset.field = 'message';
      input.placeholder = 'Commit message for ' + files; input.setAttribute('aria-label', 'Commit message');
      input.oninput = () => { change.message = input.value; };
      form.append(input);
    } else if (change.kind === 'merge' || change.kind === 'rebase') {
      const line = element('label', 'ws-repos-change-line', change.kind === 'merge' ? 'Merge into ' + change.branch + ' from ' : 'Rebase ' + change.branch + ' onto ');
      const choose = element('select', 'ws-releases-input'); choose.dataset.field = 'onto';
      change.options.forEach(name => { const option = element('option', '', name); option.value = name; choose.append(option); });
      choose.value = change.onto;
      choose.onchange = () => { change.onto = choose.value; };
      line.append(choose);
      form.append(line);
      if (change.kind === 'rebase') form.append(element('p', 'ws-repos-muted', 'This rewrites the commits of ' + change.branch + '.'));
    } else if (change.kind === 'discard') form.append(element('p', '', 'Discard changes in ' + files + '? New files are deleted. This cannot be undone.'));
    else form.append(element('p', '', 'Abort the ' + change.operation + '? Conflict fixes made so far are lost.'));
    if (change.error) form.append(element('p', 'ws-repos-error', change.error));
    const actions = element('div', 'ws-repos-actions ws-releases-form-actions');
    const submit = button(labels[change.kind], {commit:'check', discard:'undo', abort:'close', merge:'call_merge', rebase:'low_priority'}[change.kind], null);
    submit.type = 'submit'; submit.disabled = Boolean(s.changing);
    if (change.kind === 'discard' || change.kind === 'abort') { submit.classList.add('ws-repos-danger'); submit.dataset.field = 'confirm'; }
    actions.append(button('Cancel', 'close', () => cancelChange(s)), submit);
    form.append(actions);
    return form;
  }
  function worktreeChanges(s, repo, worktree) {
    const count = worktree.changes.length + worktree.more;
    const set = picked(s, worktree);
    const details = element('details', 'ws-repos-changes');
    details.dataset.worktree = worktree.path;
    details.open = s.open.has(worktree.path) ? s.open.get(worktree.path) : count <= 8;
    details.ontoggle = () => s.open.set(worktree.path, details.open);
    const head = item(element('summary', ''), 'w:' + worktree.path);
    const where = worktree.path === repo.path ? '' : ' · ' + folder(worktree.path);
    if (count) head.append(badge(plural(count, 'uncommitted file'), 'changed'));
    if (worktree.operation) head.append(badge(worktree.operation + ' stopped on conflicts', 'problem'));
    head.append(element('span', 'ws-repos-muted', ' on ' + (worktree.branch || 'detached HEAD') + where));
    const change = s.change?.worktree === worktree.path && !s.change.branch ? s.change : null;
    let bar;
    if (change) bar = changeForm(s, change);
    else {
      const busy = Boolean(s.changing);
      bar = element('div', 'ws-repos-file-actions');
      if (worktree.changes.length) {
        const all = element('label', 'ws-repos-toggle');
        const box = element('input', ''); box.type = 'checkbox'; box.checked = set.size === worktree.changes.length; box.indeterminate = set.size > 0 && !box.checked;
        box.onchange = () => toggleFile(s, worktree);
        all.append(box, element('span', '', 'Select all'));
        bar.append(all);
      }
      const what = set.size ? plural(set.size, 'file') : 'all';
      if (count) bar.append(small('Commit ' + what, 'check', () => startFileChange(s, worktree, 'commit'), busy),
        small('Discard ' + what, 'undo', () => startFileChange(s, worktree, 'discard'), busy));
      if (worktree.operation) bar.append(small('Continue ' + worktree.operation, 'play_arrow', () => act(s, {change:'continue', worktree:worktree.path}), busy),
        small('Resolve with agent', 'smart_toy', () => act(s, {change:'agent', worktree:worktree.path}), busy),
        small('Abort ' + worktree.operation, 'close', () => startFileChange(s, worktree, 'abort'), busy));
    }
    const list = element('ul', 'ws-repos-files');
    worktree.changes.forEach(change => {
      const row = item(element('li', ''), 'f:' + worktree.path + ':' + change.path);
      row.dataset.path = change.path;
      const box = element('input', ''); box.type = 'checkbox'; box.tabIndex = -1; box.checked = set.has(change.path);
      box.setAttribute('aria-label', 'Select ' + change.path);
      box.onchange = () => toggleFile(s, worktree, change.path);
      row.append(box, element('span', 'ws-repos-code', change.code === '??' ? 'new' : change.code), element('span', 'ws-repos-path', change.path));
      list.append(row);
    });
    if (worktree.more) list.append(element('li', 'ws-repos-muted', 'and ' + plural(worktree.more, 'more file') + ' (commit or discard all to include them)'));
    details.append(head, bar, list);
    return details;
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
    form.append(element('p', 'ws-repos-muted', 'Add places this project runs, like Installed, Stage or Production. Each command runs in the project folder and prints the version there: a tag like v1.2.0, a version like 1.2.0, or a commit. For example: libro --version, ssh prod cat /srv/app/VERSION, or curl -s https://stage.example.com/version'));
    if (edit.error) form.append(element('p', 'ws-repos-error', edit.error));
    const actions = element('div', 'ws-repos-actions ws-releases-form-actions');
    const add = button('Add target', 'add', () => { edit.targets.push({name:'', command:''}); edit.focus = 'name:' + (edit.targets.length - 1); render(s); });
    add.disabled = edit.targets.length >= 10;
    const cancel = button('Cancel', 'close', () => cancelEdit(s));
    const submit = button('Save', 'check', null); submit.type = 'submit'; submit.disabled = s.releasesBusy;
    actions.append(add, cancel, submit);
    form.append(actions);
    return form;
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
    if (s.releasesBusy) return;
    releaseRequest(s, {path:s.editing.path, save:true, targets:s.editing.targets.filter(target => target.name.trim() || target.command.trim())});
  }
  function project(s, repo, counts, release) {
    const card = element('section', 'ws-repos-project');
    card.dataset.attention = String(needsAttention(counts) || releaseAttention(release));
    const head = item(element('div', 'ws-repos-project-head'), 'p:' + repo.path);
    head.dataset.project = repo.name;
    const title = element('div', 'ws-repos-project-title');
    title.append(element('span', 'ws-repos-name', repo.label), element('span', 'ws-repos-muted ws-repos-folder', repo.path));
    const badges = element('div', 'ws-repos-counts');
    if (counts.changed) badges.append(badge(plural(counts.changed, 'file') + ' changed', 'changed'));
    if (counts.push) badges.append(badge(plural(counts.push, 'branch', 'branches') + ' to push', 'push'));
    if (counts.pull) badges.append(badge(plural(counts.pull, 'branch', 'branches') + ' to pull', 'pull'));
    const behind = release ? release.targets.filter(target => target.behind).length : 0;
    if (behind) badges.append(badge(plural(behind, 'target') + ' behind', 'changed'));
    if (!repo.remote && !repo.error) badges.append(badge('no remote', 'muted'));
    if (!needsAttention(counts) && !releaseAttention(release) && (repo.remote || release?.targets.length)) badges.append(badge('up to date', 'ok'));
    head.append(title, badges);
    if (release) {
      const edit = element('button', 'ws-button'); edit.type = 'button'; edit.title = edit.ariaLabel = 'Edit release targets (e)'; edit.append(icon('rocket_launch'));
      edit.onclick = () => startEdit(s, release);
      head.append(edit);
    }
    card.append(head);
    if (repo.error) card.append(element('p', 'ws-repos-error', repo.error));
    if (repo.fetchError) card.append(element('p', 'ws-repos-error', 'Fetch failed: ' + repo.fetchError));
    repo.worktrees.forEach(worktree => {
      if (worktree.error) card.append(element('p', 'ws-repos-error', worktree.path + ': ' + worktree.error));
      else if (worktree.changes.length || worktree.more || worktree.operation) card.append(worktreeChanges(s, repo, worktree));
    });
    // Checked-out branches stay listed so they can be merged or rebased.
    const branches = repo.branches.filter(branch => branch.ahead || branch.behind || branch.gone || branchWorktree(repo, branch));
    if (branches.length) {
      const list = element('ul', 'ws-repos-branches');
      branches.forEach(branch => list.append(branchRow(s, repo, branch)));
      card.append(list);
    }
    if (s.change?.repo === repo.path) card.append(changeForm(s, s.change));
    if (release?.error) card.append(element('p', 'ws-repos-error', release.error));
    if (s.editing?.path === repo.path) card.append(editForm(s));
    else if (release?.targets.length) {
      const list = element('ul', 'ws-repos-branches');
      release.targets.forEach((target, index) => list.append(targetRow(s, release, target, index)));
      card.append(list);
    }
    if (repo.remote) card.append(element('div', 'ws-repos-muted ws-repos-fetched', repo.fetched ? 'Remote checked ' + relativeTime(repo.fetched) : 'Never fetched'));
    return card;
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
    const repos = s.repos || [];
    const releases = new Map((s.releases || []).map(release => [release.path, release]));
    const rows = repos.map(repo => ({repo, counts:summary(repo), release:releases.get(repo.path)}));
    const attention = rows.filter(row => needsAttention(row.counts) || releaseAttention(row.release));
    heading.append(element('span', 'ws-repos-title', 'Repositories'),
      element('span', 'ws-repos-muted', s.repos ? attention.length + ' of ' + repos.length + ' need attention' : ''));
    const actions = element('div', 'ws-repos-actions');
    const all = element('label', 'ws-repos-toggle');
    const box = element('input', ''); box.type = 'checkbox'; box.checked = s.showAll;
    box.onchange = () => { s.showAll = box.checked; render(s); };
    all.append(box, element('span', '', 'Show all'));
    const refreshButton = button('Refresh', 'refresh', () => refresh(s));
    const fetchButton = button('Fetch', 'cloud_download', () => request(s, true));
    refreshButton.disabled = s.busy && s.releasesBusy;
    fetchButton.disabled = s.busy;
    const keys = element('button', 'ws-button'); keys.type = 'button'; keys.title = keys.ariaLabel = 'Keyboard shortcuts';
    keys.setAttribute('aria-expanded', String(s.help)); keys.append(icon('keyboard'));
    keys.onclick = () => { s.help = !s.help; render(s); };
    actions.append(all, refreshButton, fetchButton, keys);
    toolbar.append(heading, actions);
    const status = element('div', 'ws-repos-status'); status.setAttribute('role', 'status');
    if (s.error || s.releaseError) status.textContent = [s.error, s.releaseError].filter(Boolean).join(' ');
    else if (s.changing) status.textContent = {commit:'Committing…', discard:'Discarding…', push:'Pushing…', pull:'Pulling…', merge:'Merging…', rebase:'Rebasing…', continue:'Continuing…', abort:'Aborting…', agent:'Asking the agent…'}[s.changing];
    else if (s.busy) status.textContent = s.fetching ? 'Fetching remotes…' : 'Checking repositories…';
    else if (s.releasesBusy) status.textContent = s.saving ? 'Saving release targets…' : 'Checking release targets…';
    else if (s.checked) status.textContent = 'Checked at ' + s.checked.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'});
    s.el.append(toolbar, status);
    if (s.help) s.el.append(element('div', 'ws-repos-keys', 'j/k move · gg/G first/last · {/} project · l/Enter open files or missing commits · h close · x/Space check file · c commit · d discard · p pull · P push · M merge · R rebase · e edit release targets · o open project · r refresh · f fetch · a show all · ? keys'));
    const list = element('div', 'ws-repos-list');
    rows.sort((a, b) => attention.includes(b) - attention.includes(a) || a.repo.label.localeCompare(b.repo.label));
    rows.filter(row => s.showAll || attention.includes(row) || s.editing?.path === row.repo.path).forEach(row => list.append(project(s, row.repo, row.counts, row.release)));
    if (s.repos && !list.children.length) {
      const empty = element('div', 'ws-repos-empty');
      empty.append(icon(repos.length ? 'task_alt' : 'folder_off'),
        element('p', 'ws-repos-muted', repos.length ? 'Everything is committed, pushed and released.' : 'No Git projects yet. Add a project to see it here.'));
      list.append(empty);
    }
    s.el.append(list);
    s.el.scrollTop = scroll;
    // A closed group hides its files; keep the cursor on the group instead.
    s.el.querySelectorAll('details').forEach(details => details.addEventListener('toggle', () => {
      const summary = details.querySelector('summary');
      select(s, !details.open && details.querySelector('[data-repos-item="' + CSS.escape(s.cursor) + '"]') ? summary.dataset.reposItem : s.cursor, false, s.el.contains(document.activeElement));
    }));
    const target = s.editing?.focus || s.change?.focus || field?.name;
    const input = target && s.el.querySelector('[data-field="' + CSS.escape(target) + '"]');
    if (input) {
      if (s.editing) s.editing.focus = '';
      if (s.change) s.change.focus = '';
      select(s, s.cursor, false, false);
      input.focus({preventScroll:true});
      if (field?.name === target && field.start != null) input.setSelectionRange(field.start, field.end);
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
    const form = event.target.closest('form');
    if (form) {
      if (event.key === 'Escape') { if (form.classList.contains('ws-repos-change')) cancelChange(s); else cancelEdit(s); event.preventDefault(); event.stopPropagation(); }
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
    const repo = card && s.repos?.find(entry => 'p:' + entry.path === card.dataset.reposItem);
    const worktree = details && repo?.worktrees.find(entry => entry.path === details.dataset.worktree);
    const branch = current?.dataset.branch !== undefined && repo?.branches.find(entry => entry.name === current.dataset.branch);
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
    } else if ((key === 'x' || key === ' ') && worktree && current.tagName !== 'SUMMARY') toggleFile(s, worktree, current.dataset.path);
    else if (key === 'x' && worktree) toggleFile(s, worktree);
    else if ((key === 'c' || key === 'd') && worktree && !s.changing) startFileChange(s, worktree, key === 'c' ? 'commit' : 'discard');
    else if (key === 'p' && branch) pullBranch(s, repo, branch);
    else if (key === 'P' && branch) pushBranch(s, repo, branch);
    else if ((key === 'M' || key === 'R') && branch && !s.changing) startBranchChange(s, repo, branch, key === 'M' ? 'merge' : 'rebase');
    else if (key === 'e' && card) {
      const release = s.releases?.find(release => 'p:' + release.path === card.dataset.reposItem);
      if (release) startEdit(s, release);
    } else if (key === 'o') {
      if (card) window.libroWorkspace?.openProject(card.dataset.project);
    } else if (key === 'r') refresh(s);
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
      const s = {el, id:el.dataset.repos, repos:null, releases:null, showAll:false, busy:false, changing:'', change:null, selected:new Map(), releasesBusy:false, fetched:false, open:new Map(), cursor:'', help:false, editing:null};
      states.set(s.id, s);
      el.tabIndex = 0;
      el.setAttribute('role', 'region');
      el.setAttribute('aria-label', 'Repositories. Press ? for keyboard shortcuts.');
      el.onkeydown = event => keydown(s, event);
      el.onclick = event => { const row = event.target.closest('[data-repos-item]'); if (row) select(s, row.dataset.reposItem, false); };
      request(s, false);
      if (releaseCache && Date.now() - releaseCache.checked < 5 * 60 * 1000) s.releases = releaseCache.releases;
      else releaseRequest(s);
    });
  }
  window.libroRepos = {init, dispose, refresh:() => { for (const s of states.values()) if (s.el.isConnected) refresh(s); }};
})();
