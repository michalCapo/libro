(function () {
  if (window.libroWorkspace) return;
  const root = document.getElementById('libro-workspace');
  const sid = window.__libroWorkspaceSID;
  let maximized = '';
  let keepBottomHidden = false;
  const dockStates = new Map();
  function dockState(grid) {
    const key = grid.dataset.workspaceProject;
    if (!dockStates.has(key)) dockStates.set(key, {right:'', hidden:new Set(), bottom:false});
    return dockStates.get(key);
  }
  let prefs = {};
  try { prefs = JSON.parse(localStorage.getItem('libro.workspace') || '{}') || {}; } catch (_) {}
  if (typeof prefs.projects !== 'boolean') prefs.projects = innerWidth > 760;
  const call = (action, data = {}) => {
    if (action === 'project.switch') acknowledgeProjectActivity(data.name);
    if (action === 'worktree.switch') acknowledgeProjectActivity(data.project + '/' + data.branch);
    return __ws.call(action, {sid, ...data});
  };
  const node = (tag, cls, text) => { const el = document.createElement(tag); el.className = cls; if (text) el.textContent = text; return el; };
  const button = (label, icon, action) => {
    const el = node('button', 'ws-button'); el.type = 'button'; el.title = label; el.setAttribute('aria-label', label);
    const glyph = node('i', 'material-icons-round', icon); glyph.setAttribute('aria-hidden', 'true'); el.append(glyph); el.onclick = action; return el;
  };
  document.addEventListener('click', event => {
    const trigger = event.target.closest('[data-browser-menu-trigger]');
    if (!trigger) return;
    event.preventDefault();
    const menu = document.getElementById(trigger.getAttribute('popovertarget'));
    if (!menu.togglePopover()) return;
    const rect = trigger.getBoundingClientRect();
    menu.style.left = Math.max(8, Math.min(rect.right - menu.offsetWidth, innerWidth - menu.offsetWidth - 8)) + 'px';
    menu.style.top = Math.max(8, Math.min(rect.bottom + 4, innerHeight - menu.offsetHeight - 8)) + 'px';
    menu.querySelector('button')?.focus();
  });
  document.addEventListener('keydown', event => {
    const menu = event.target.closest('.ws-browser-menu');
    if (!menu || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const items = [...menu.querySelectorAll('button')];
    const current = items.indexOf(document.activeElement);
    const index = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
    items[index]?.focus();
  });
  let sizePicker = null;
  let sizePickerTimer;
  function closeSizePicker() {
    clearTimeout(sizePickerTimer);
    if (sizePicker?.matches(':popover-open')) sizePicker.hidePopover();
    sizePicker = null;
  }
  function openSizePicker(trigger) {
    clearTimeout(sizePickerTimer);
    const picker = document.getElementById(trigger.getAttribute('aria-controls'));
    if (!picker || picker.matches(':popover-open')) return;
    closeSizePicker();
    sizePicker = picker;
    picker.showPopover();
    const rect = trigger.getBoundingClientRect();
    picker.style.left = Math.max(8, Math.min(rect.right - picker.offsetWidth, innerWidth - picker.offsetWidth - 8)) + 'px';
    picker.style.top = Math.min(rect.bottom, innerHeight - picker.offsetHeight - 8) + 'px';
    trigger.setAttribute('aria-expanded', 'true');
    picker.ontoggle = () => trigger.setAttribute('aria-expanded', String(picker.matches(':popover-open')));
  }
  document.addEventListener('pointerover', event => {
    const trigger = event.target.closest('[data-size-trigger]');
    if (trigger && event.pointerType !== 'touch') openSizePicker(trigger);
    if (event.target.closest('.ws-size-picker')) clearTimeout(sizePickerTimer);
  });
  document.addEventListener('pointerout', event => {
    if (!event.target.closest('[data-size-badges]')) return;
    if (event.relatedTarget?.closest('[data-size-badges]') === event.target.closest('[data-size-badges]')) return;
    sizePickerTimer = setTimeout(closeSizePicker, 180);
  });
  document.addEventListener('click', event => {
    const trigger = event.target.closest('[data-size-trigger]');
    if (trigger) { event.stopPropagation(); openSizePicker(trigger); }
  });
  document.addEventListener('keydown', event => {
    const trigger = event.target.closest('[data-size-trigger]');
    if (trigger && ['ArrowDown', 'Enter', ' '].includes(event.key)) {
      event.preventDefault();
      openSizePicker(trigger);
      sizePicker?.querySelector('[aria-pressed="true"]')?.focus();
    }
  });
  window.addEventListener('resize', closeSizePicker);
  document.addEventListener('scroll', event => {
    if (!event.target.closest?.('.ws-size-picker')) closeSizePicker();
  }, true);
  function save() { try { localStorage.setItem('libro.workspace', JSON.stringify(prefs)); } catch (_) {} }
  function activeGrid() { return [...document.querySelectorAll('[data-workspace-project]')].find(el => el.parentElement.style.display !== 'none'); }
  function frames(grid) { return [...grid.querySelectorAll(':scope > [data-app-id]')].sort((a, b) => Number(a.style.order) - Number(b.style.order)); }
  function isThread(grid = activeGrid()) { return grid?.dataset.thread === 'true'; }
  function openPlugin(id, dock, trigger) {
    if (isThread() && dock === 'center' && frames(activeGrid()).some(frame => frame.dataset.dock === 'center')) return;
    const plugin = window.__libroPlugins.find(candidate => candidate.id === id);
    if (!plugin || plugin.disabled || plugin.removed || trigger?.disabled) return;
    if (trigger) {
      trigger.disabled = true;
      trigger.setAttribute('aria-busy', 'true');
    }
    call('app.start', {
      type: plugin.type,
      command: plugin.command || '',
      url: plugin.url || '',
      name: plugin.name,
      plugin: plugin.id,
      dock: dock || plugin.dock,
      writable: true
    });
    if (trigger) setTimeout(() => {
      if (!trigger.isConnected) return;
      trigger.disabled = false;
      trigger.removeAttribute('aria-busy');
    }, 800);
  }
  function select(id, send = true) {
    if (!document.getElementById('workspace-settings').hidden) closeSettings();
    const frame = document.getElementById('frame-' + id);
    if (!frame) return;
    if (keepBottomHidden && frame.dataset.dock === 'bottom') return;
    const grid = frame.parentElement;
    const state = dockState(grid);
    state.hidden.delete(id);
    if (frame.dataset.dock === 'right') state.right = id;
    if (frame.dataset.dock === 'bottom') { state.bottom = true; state.bottomID = id; }
    window.__libroSelectedApp = id;
    if (maximized && maximized !== id) maximized = '';
    refresh();
    window.__libroScrollToApp(frame);
    if (send) call('app.select', {index:frames(grid).indexOf(frame)});
    requestAnimationFrame(() => {
      const terminal = frame.querySelector('[data-terminal-app]');
      if (terminal && window.__libroFitTerminalFrame) window.__libroFitTerminalFrame(terminal);
      if (window.__libroFocusAppByID) window.__libroFocusAppByID(id);
    });
  }
  window.__libroEnsurePageToolAgent = function(browserID, mode) {
    const browser = document.getElementById('frame-' + browserID);
    const grid = browser?.parentElement;
    if (!grid) return false;
    const agents = frames(grid).filter(frame => frame.dataset.dock === 'center' && frame.querySelector('[data-terminal-app]'));
    const agent = agents.find(frame => frame.dataset.appId === dockState(grid).agent) || agents[0];
    if (agent) { window.__libroActiveAgentID = agent.dataset.appId; return true; }
    launcher('center', () => {
      let attempts = 0;
      const resume = setInterval(() => {
        if (!browser.isConnected || activeGrid() !== grid || ++attempts > 150) { clearInterval(resume); return; }
        const agent = frames(grid).find(frame => frame.dataset.dock === 'center' && frame.querySelector('[data-terminal-app]'));
        if (!agent) return;
        clearInterval(resume);
        window.__libroActiveAgentID = agent.dataset.appId;
        select(browserID);
        window.__libroTogglePageTool?.(browserID, mode);
      }, 200);
    });
    return false;
  };
  function launcher(dock, onLaunch) {
    if (isThread() && dock === 'center' && frames(activeGrid()).some(frame => frame.dataset.dock === 'center')) return;
    if (dock === 'bottom') { bottom(); return; }
    let dialog = document.getElementById('workspace-plugin-dialog');
    if (dialog) dialog.remove();
    dialog = node('dialog', 'ws-plugin-dialog'); dialog.id = 'workspace-plugin-dialog'; dialog.setAttribute('aria-label', dock === 'center' ? 'New agent session' : 'Open an app');
    const searchBar = node('div', 'ws-command-search');
    const search = node('input', 'ws-palette-search'); search.placeholder = dock === 'center' ? 'Search agents…' : 'Search apps…'; search.setAttribute('aria-label', dock === 'center' ? 'Search agents' : 'Search apps'); search.autocomplete = 'off'; search.spellcheck = false;
    const dismiss = node('button', 'ws-command-dismiss'); dismiss.append(node('i', 'material-icons-round', 'close')); dismiss.firstChild.setAttribute('aria-hidden', 'true'); dismiss.type = 'button'; dismiss.setAttribute('aria-label', 'Close app launcher'); dismiss.onclick = () => dialog.close();
    searchBar.append(node('i', 'material-icons-round', 'search'), search, dismiss); dialog.append(searchBar);
    const list = window.__libroPlugins.filter(plugin => {
      if (plugin.disabled || plugin.removed) return false;
      if (dock === 'center') return plugin.dock === 'center' && plugin.type === 'terminal';
      if (dock === 'right') return plugin.dock !== 'center';
      return true;
    });
    const entries = node('div', 'ws-plugin-list');
    list.forEach(plugin => {
      const entry = node('button', 'ws-plugin-entry'); entry.type = 'button'; const icon = node('i', 'material-icons-round', plugin.type === 'url' ? 'language' : 'terminal'); icon.setAttribute('aria-hidden', 'true'); const copy = node('span', 'ws-plugin-copy'); copy.append(node('span', '', plugin.name), node('small', '', plugin.description || plugin.command || 'Browser app')); entry.append(icon, copy); applyToolIcon(entry, plugin);
      entry.onclick = () => { dialog.close(); openPlugin(plugin.id, dock, entry); onLaunch?.(); }; entries.append(entry);
    });
    dialog.append(entries);
    let active = 0;
    const visibleEntries = () => [...entries.children].filter(entry => !entry.hidden);
    const highlight = () => { visibleEntries().forEach((entry, index) => entry.dataset.selected = String(index === active)); };
    const noResults = node('p', 'ws-palette-empty', 'No matching apps'); noResults.hidden = true; dialog.append(noResults);
    search.oninput = () => {
      const query = search.value.trim().toLowerCase();
      [...entries.children].forEach(entry => entry.hidden = !entry.querySelector('.ws-plugin-copy > span').textContent.toLowerCase().includes(query));
      active = 0; noResults.hidden = visibleEntries().length > 0; highlight();
    };
    search.onkeydown = event => {
      const visible = visibleEntries();
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault(); active = Math.max(0, Math.min(visible.length - 1, active + (event.key === 'ArrowDown' ? 1 : -1))); highlight(); visible[active]?.scrollIntoView({block:'nearest'});
      } else if (event.key === 'Enter') { event.preventDefault(); visible[active]?.click(); }
    };
    highlight();
    dialog.append(node('div', 'ws-command-footer', '↑ ↓ Navigate · Enter open · Esc close'));
    root.append(dialog); dialog.showModal(); search.focus();
    dialog.addEventListener('click', e => { if (e.target === dialog) { const rect = dialog.getBoundingClientRect(); if (e.clientX < rect.left || e.clientX > rect.right || e.clientY < rect.top || e.clientY > rect.bottom) dialog.close(); } });
  }
  function empty(zone, grid) {
    const el = node('section', 'ws-empty'); el.dataset.zone = zone;
    const center = zone === 'center';
    const standalone = (window.__libroThreads || []).some(thread => thread.id === grid.dataset.workspaceProject);
    const heading = node(center ? 'h1' : 'h2', '', center ? (standalone ? 'What would you like to do in ' : 'What are we building on ') : zone === 'right' ? 'Tools for your workspace' : 'Your project terminal');
    if (center) heading.append(node('span', 'ws-project-accent', grid.dataset.projectLabel), document.createTextNode('?'));
    el.append(heading);
    el.append(node('p', '', center ? (standalone ? 'Explore an idea or work on your computer and servers. Agents start in your home folder.' : 'Start an agent in this project. Your tools and terminals stay close by.') : zone === 'right' ? 'Open a browser, repository tool, or terminal.' : 'Run commands without leaving your agent.'));
    const actions = node('div', 'ws-agent-actions');

    (center ? window.__libroPlugins.filter(p => p.dock === 'center' && p.type === 'terminal' && !p.disabled && !p.removed).slice(0, 3).map(p => p.id) : zone === 'right' ? ['browser', 'lazyrepo', 'terminal'] : ['terminal']).forEach(id => {
      const plugin = window.__libroPlugins.find(p => p.id === id);
      if (!plugin || plugin.disabled || plugin.removed) return;
      const launch = node('button', 'ws-launch', plugin.name);
      launch.type = 'button';
      launch.onclick = () => openPlugin(id, zone, launch);
      actions.append(launch);
    });
    const more = node('button', 'ws-launch', center ? 'Other agent…' : 'More…'); more.type = 'button'; more.onclick = () => launcher(zone); actions.append(more); el.append(actions); return el;
  }
  function projectSettings(name) {
    const project = (window.__libroProjects || []).find(p => (p.kind === 'worktree' ? p.name + '/' + p.branch : p.name) === name);
    if (!project) return;
    document.getElementById('project-command-dialog')?.remove();
    const dialog = node('dialog', 'ws-plugin-dialog ws-project-command-dialog'); dialog.id = 'project-command-dialog';
    dialog.setAttribute('aria-labelledby', 'project-command-title');
    const heading = node('h2', '', project.displayName || project.name); heading.id = 'project-command-title';
    const form = node('form', '');
    const label = node('label', '', 'Start command'); label.htmlFor = 'project-command-input';
    const input = node('input', 'ws-agent-command'); input.id = 'project-command-input'; input.value = project.command || ''; input.placeholder = 'air or bun src/dev.ts'; input.autocomplete = 'off'; input.spellcheck = false;
    const help = node('p', 'ws-settings-status', 'Runs in this project folder. ' + (toolKeys['run-project'] || 'Start / restart project') + ' starts or restarts it; ' + (toolKeys['stop-project'] || 'Stop project command') + ' stops it. You can also use Ctrl+C in the terminal.'); help.id = 'project-command-help'; input.setAttribute('aria-describedby', help.id);
    const actions = node('div', 'ws-agent-actions');
    const cancel = node('button', 'ws-launch', 'Cancel'); cancel.type = 'button'; cancel.onclick = () => dialog.close();
    const submit = node('button', 'ws-launch', 'Save'); submit.type = 'submit';
    actions.append(cancel, submit); form.append(label, input, help, actions);
    form.onsubmit = event => { event.preventDefault(); call('project.command.save', {name, command:input.value}); };
    dialog.append(heading, form); root.append(dialog); dialog.showModal(); input.focus();
  }
  function newThread() {
    call('thread.create', {});
  }
  function threadArchived() { refresh(); }
  function renderThreads() {
    const list = document.getElementById('workspace-thread-list'); if (!list) return;
    const threads = (window.__libroThreads || []).slice().reverse();
    const signature = JSON.stringify([threads, window.__libroActiveProject]);
    if (list.dataset.signature === signature) return;
    list.dataset.signature = signature; list.replaceChildren();
    const appendThread = thread => {
      const item = node('div', 'ws-project-item');
      item.dataset.archived = String(thread.archived);
      const row = node('button', 'ws-project-row ws-thread-row'); row.type = 'button'; row.title = thread.name; row.dataset.projectKey = thread.id; row.dataset.kind = 'thread';
      row.setAttribute('aria-current', String(thread.id === window.__libroActiveProject));
      const icon = node('i', 'material-icons-round', 'chat_bubble_outline'); icon.setAttribute('aria-hidden', 'true');
      row.append(icon, node('span', '', thread.name));
      row.onclick = () => { closeSettings(); if (innerWidth <= 760) { prefs.projects = false; save(); } call('project.switch', {name:thread.id}); };
      const archive = button(thread.archived ? 'Restore thread' : 'Archive thread', thread.archived ? 'unarchive' : 'archive', () => call('thread.archive', {id:thread.id, archived:!thread.archived}));
      archive.classList.add('ws-project-remove'); item.append(row, archive); list.append(item);
    };
    threads.filter(thread => !thread.archived).forEach(appendThread);
    if (!threads.length) {
      list.append(node('div', 'ws-no-threads', 'No threads to show'));
    }
    threads.filter(thread => thread.archived).slice(0, 10).forEach(appendThread);
  }
  function renderProjects() {
    const list = document.getElementById('workspace-project-list'); if (!list) return;
    const projects = window.__libroProjects || []; const signature = JSON.stringify(projects);
    if (list.dataset.signature === signature) return; list.dataset.signature = signature; list.replaceChildren();
    projects.forEach(project => {
      const item = node('div', 'ws-project-item'); item.dataset.kind = project.kind;
      const row = node('button', 'ws-project-row'); row.type = 'button'; row.dataset.kind = project.kind;
      row.dataset.projectKey = project.kind === 'worktree' ? project.name + '/' + project.branch : project.name;
      row.setAttribute('aria-current', String(!!project.isActive));
      const projectName = project.displayName || project.name;
      row.setAttribute('aria-label', projectName);
      row.title = projectName + (project.path ? '\n' + project.path : '');
      const icon = node('i', 'material-icons-round', project.kind === 'worktree' ? 'account_tree' : 'folder_open'); icon.setAttribute('aria-hidden', 'true');
      row.append(icon, node('span', '', project.displayName || project.name));
      row.onclick = () => { closeSettings(); if (innerWidth <= 760) { prefs.projects = false; save(); } if (project.kind === 'worktree') call('worktree.switch', {project:project.name, path:project.path, branch:project.branch}); else call('project.switch', {name:project.name}); };
      item.append(row);
      const settings = button('Settings for ' + projectName, 'settings', event => { event.stopPropagation(); projectSettings(row.dataset.projectKey); });
      settings.classList.add('ws-project-settings'); item.append(settings);
      if (project.kind !== 'worktree') {
        const label = project.displayName || project.name;
        const remove = button('Remove ' + label + ' from Libro', 'close', event => {
          event.stopPropagation();
          const run = () => call('project.remove', {name:project.name});
          const message = 'Remove project "' + label + '" from Libro?\n\nThis only removes it from the project list. The directory and its worktrees stay on disk.';
          if (window.__libroConfirmAction) window.__libroConfirmAction('Remove project?', message, run); else run();
        });
        remove.classList.add('ws-project-remove'); item.append(remove);
      }
      list.append(item);
    });
  }
  function renderProjectAgents() {
    const grids = new Map([...document.querySelectorAll('[data-workspace-project]')].map(grid => [grid.dataset.workspaceProject, grid]));
    document.querySelectorAll('.ws-project-row:not([data-kind=thread])').forEach(row => {
      const grid = grids.get(row.dataset.projectKey);
      const agents = grid ? frames(grid).filter(frame => frame.dataset.dock === 'center') : [];
      let tree = row.parentElement.nextElementSibling;
      if (!tree?.classList.contains('ws-project-agents')) tree = null;
      if (!agents.length) { tree?.remove(); return; }
      if (!tree) {
        tree = node('div', 'ws-project-agents');
        tree.setAttribute('role', 'group');
        tree.setAttribute('aria-label', row.querySelector('span').textContent + ' agents');
        row.parentElement.after(tree);
      }
      const entries = agents.map(frame => ({id:frame.dataset.appId, name:frame.dataset.appName || 'Agent', title:frame.dataset.taskTitle || '', selected:grid.dataset.workspaceProject === window.__libroActiveProject && frame.dataset.appId === window.__libroSelectedApp}));
      const signature = JSON.stringify(entries);
      if (tree.dataset.signature === signature) return;
      tree.dataset.signature = signature;
      const tabs = new Map([...tree.children].map(tab => [tab.dataset.agentId, tab]));
      entries.forEach((entry, index) => {
        const label = entry.title || entry.name + ' session ' + (index + 1);
        let tab = tabs.get(entry.id);
        if (!tab) {
          tab = node('button', 'ws-project-agent'); tab.type = 'button';
          tab.dataset.agentId = entry.id;
          const icon = node('i', 'material-icons-round', 'chat_bubble_outline'); icon.setAttribute('aria-hidden', 'true');
          tab.append(icon, node('span', '', label));
        }
        tabs.delete(entry.id);
        tab.setAttribute('aria-current', String(entry.selected));
        tab.title = entry.name + ': ' + label;
        const text = tab.querySelector('span');
        if (text.textContent !== label) text.textContent = label;
        tab.onclick = () => {
          closeSettings();
          if (innerWidth <= 760) { prefs.projects = false; save(); }
          if (grid.dataset.workspaceProject === window.__libroActiveProject) select(entry.id);
          else call('project.switch', {name:grid.dataset.workspaceProject, appId:entry.id});
        };
        // Keep the button mounted while terminal titles animate, including during a click.
        if (tree.children[index] !== tab) tree.insertBefore(tab, tree.children[index] || null);
      });
      tabs.forEach(tab => tab.remove());
    });
  }
  function renderProjectShortcuts() {
    const running = new Set([...document.querySelectorAll('[data-workspace-project]')]
      .filter(grid => frames(grid).length > 0).map(grid => grid.dataset.workspaceProject));
    let index = 0;
    document.querySelectorAll('.ws-project-row').forEach(row => {
      const thread = row.dataset.kind === 'thread';
      const available = thread ? row.parentElement.dataset.archived !== 'true' : running.has(row.dataset.projectKey);
      const number = available && index < 9 ? String(++index) : '';
      row.dataset.projectShortcut = number;
      let badge = row.querySelector('.ws-project-shortcut');
      if (!number) { badge?.remove(); row.removeAttribute('aria-keyshortcuts'); return; }
      if (!badge) { badge = node('kbd', 'ws-project-shortcut'); badge.setAttribute('aria-hidden', 'true'); row.append(badge); }
      badge.textContent = number;
      badge.title = 'Ctrl+' + number;
      row.setAttribute('aria-keyshortcuts', 'Control+' + number);
    });
  }
  let notificationAudio;
  function enableNotificationAudio() {
    if (prefs.notificationSound === false) return;
    try {
      notificationAudio ||= new AudioContext();
      if (notificationAudio.state === 'suspended') notificationAudio.resume().catch(() => {});
    } catch (_) {}
  }
  // Unlock audio during interaction so background completions can play immediately.
  document.addEventListener('pointerdown', enableNotificationAudio, true);
  document.addEventListener('keydown', enableNotificationAudio, true);
  function playDoneSound() {
    if (prefs.notificationSound === false || notificationAudio?.state !== 'running') return;
    const start = notificationAudio.currentTime;
    [660, 880].forEach((frequency, index) => {
      const tone = notificationAudio.createOscillator();
      const volume = notificationAudio.createGain();
      const at = start + index * 0.14;
      tone.frequency.value = frequency;
      volume.gain.setValueAtTime(0, at);
      volume.gain.linearRampToValueAtTime(0.12, at + 0.015);
      volume.gain.exponentialRampToValueAtTime(0.001, at + 0.3);
      tone.connect(volume); volume.connect(notificationAudio.destination);
      tone.onended = () => { tone.disconnect(); volume.disconnect(); };
      tone.start(at); tone.stop(at + 0.32);
    });
  }
  let previousAgentStatuses = {...window.__libroAgentStatuses};
  function notifyAgentDone() {
    const statuses = window.__libroAgentStatuses || {};
    for (const [id, status] of Object.entries(statuses)) {
      if (status === 'done' && previousAgentStatuses[id] === 'working') playDoneSound();
    }
    previousAgentStatuses = {...statuses};
  }
  window.addEventListener('libro-agent-status', notifyAgentDone);
  function saveNotificationSound(value) {
    const enabled = value === 'on';
    const status = document.getElementById('notification-sound-status');
    try {
      const updated = {...prefs, notificationSound:enabled};
      localStorage.setItem('libro.workspace', JSON.stringify(updated));
      prefs = updated;
      enableNotificationAudio();
      status.textContent = enabled ? 'Notification sound on.' : 'Notification sound off.';
    } catch (_) {
      document.getElementById('notification-sound').value = prefs.notificationSound === false ? 'off' : 'on';
      status.textContent = 'Could not save. Please try again.';
    }
  }
  const acknowledgedAgents = new Set();
  function acknowledgeProjectActivity(key) {
    const grid = [...document.querySelectorAll('[data-workspace-project]')].find(grid => grid.dataset.workspaceProject === key);
    if (!grid) return;
    let changed = false;
    frames(grid).forEach(frame => {
      const id = frame.dataset.appId;
      if (window.__libroAgentStatuses?.[id] === 'done' && !acknowledgedAgents.has(id)) {
        acknowledgedAgents.add(id);
        changed = true;
      }
    });
    if (changed) renderProjectActivity();
  }
  function acknowledgeProjectInteraction(event) {
    const thread = event.target.closest?.('.ws-project-agent, [data-app-id]');
    const id = thread?.dataset.agentId || thread?.dataset.appId;
    if (id) {
      if (window.__libroAgentStatuses?.[id] === 'done') {
        acknowledgedAgents.add(id);
        renderProjectActivity();
      }
      return;
    }
    const row = event.target.closest?.('.ws-project-row');
    const project = event.target.closest?.('.ws-project');
    const grid = project?.querySelector('[data-workspace-project]');
    const key = row?.dataset.projectKey || grid?.dataset.workspaceProject;
    if (key) acknowledgeProjectActivity(key);
    else if (event.type === 'keydown' && event.target === document.body) acknowledgeProjectActivity(window.__libroActiveProject);
  }
  ['pointerdown', 'keydown', 'input', 'wheel'].forEach(type => {
    document.addEventListener(type, acknowledgeProjectInteraction, {capture:true, passive:true});
  });
  function renderProjectActivity() {
    const statuses = window.__libroAgentStatuses || {};
    for (const id of acknowledgedAgents) {
      if (statuses[id] !== 'done') acknowledgedAgents.delete(id);
    }
    document.querySelectorAll('.ws-project-agent').forEach(tab => {
      const id = tab.dataset.agentId;
      const state = acknowledgedAgents.has(id) && statuses[id] === 'done' ? 'idle' : statuses[id];
      const status = state === 'working' ? 'Working' : state === 'done' ? 'Done' : '';
      tab.dataset.agentStatus = status ? state : '';
      tab.querySelector('i').textContent = state === 'working' ? 'sync' : state === 'done' ? 'check_circle_outline' : 'chat_bubble_outline';
      let badge = tab.querySelector('.ws-thread-status');
      if (status && !badge) {
        badge = node('small', 'ws-thread-status');
        tab.append(badge);
      }
      if (badge) {
        if (status) badge.textContent = status;
        else badge.remove();
      }
      tab.setAttribute('aria-label', tab.querySelector('span').textContent + (status ? ': ' + status : ''));
    });
    const projects = new Map();
    document.querySelectorAll('[data-workspace-project]').forEach(grid => {
      const agents = frames(grid).filter(frame => frame.dataset.dock === 'center');
      const states = agents.map(frame => acknowledgedAgents.has(frame.dataset.appId) && statuses[frame.dataset.appId] === 'done' ? 'idle' : statuses[frame.dataset.appId]);
      projects.set(grid.dataset.workspaceProject, states.includes('working') ? 'working' :
        states.includes('done') && states.every(state => state === 'done' || state === 'idle') ? 'done' : '');
    });
    document.querySelectorAll('.ws-project-row').forEach(row => {
      const state = projects.get(row.dataset.projectKey) || '';
      if (row.dataset.agentStatus === state) return;
      row.dataset.agentStatus = state;
      row.querySelector('i').textContent = state === 'working' ? 'sync' : state === 'done' ? 'check_circle_outline' : row.dataset.kind === 'worktree' ? 'account_tree' : row.dataset.kind === 'thread' ? 'chat_bubble_outline' : 'folder_open';
      const label = row.querySelector('span').textContent;
      row.setAttribute('aria-label', label + (state === 'working' ? ': agents working' : state === 'done' ? ': all agents done' : ''));
    });
  }
  window.addEventListener('libro-agent-status', renderProjectActivity);
  function renderProjectTerminals() {
    const running = new Set([...document.querySelectorAll('[data-workspace-project]')]
      .filter(grid => grid.querySelector('[data-dock="bottom"] [data-process-status="running"]'))
      .map(grid => grid.dataset.workspaceProject));
    document.querySelectorAll('.ws-project-row').forEach(row => {
      const active = running.has(row.dataset.projectKey);
      let icon = row.querySelector('.ws-project-terminal');
      if (!active) { icon?.remove(); return; }
      if (icon) return;
      icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      icon.classList.add('ws-project-terminal');
      icon.setAttribute('viewBox', '0 0 24 24');
      icon.setAttribute('role', 'img');
      icon.setAttribute('aria-label', 'Command running in bottom terminal');
      const title = document.createElementNS(icon.namespaceURI, 'title');
      title.textContent = 'Command running in bottom terminal';
      const path = document.createElementNS(icon.namespaceURI, 'path');
      path.setAttribute('d', 'm4 5 6 6-6 6m9 0h7');
      icon.append(title, path);
      row.insertBefore(icon, row.querySelector('.ws-project-shortcut'));
    });
  }
  window.addEventListener('libro-process-status', renderProjectTerminals);
  let queued = false;
  const observer = new MutationObserver(records => {
    if (records.some(record => (record.type === 'attributes' && record.target.matches('[data-app-id]')) || [...record.addedNodes, ...record.removedNodes].some(n => n.nodeType === 1 && (n.matches('[data-app-id], [data-workspace-project], .ws-project, #top-bar') || n.querySelector('[data-app-id], [data-workspace-project]'))))) schedule();
  });
  function schedule() { if (!queued) { queued = true; requestAnimationFrame(() => { queued = false; refresh(); }); } }
  function refresh() {
    observer.disconnect();
    root.dataset.projects = String(prefs.projects);
    root.dataset.thread = String(isThread());
    document.querySelectorAll('.ws-sidebar-action, .ws-sidebar-search').forEach(button => {
      const label = button.getAttribute('aria-label') || button.textContent.trim().replace(/^(settings|apps|tune)\s*/, '');
      button.setAttribute('aria-label', label); button.title = label;
    });
    renderToolRail();
    renderProjects();
    renderThreads();
    renderProjectAgents();
    renderProjectShortcuts();
    renderProjectActivity();
    renderProjectTerminals();
    window.libroFiles?.init();
    window.libroNotes?.init();
    document.querySelectorAll('[data-workspace-project]').forEach(grid => {
      // Replacing a project's DOM can reset its hidden styles. Always reconcile
      // visibility with server state so inactive welcome screens cannot reappear.
      if (typeof window.__libroActiveProject === 'string') {
        const active = grid.dataset.workspaceProject === window.__libroActiveProject;
        const project = grid.parentElement;
        project.style.display = active ? 'flex' : 'none';
        project.style.visibility = active ? 'visible' : 'hidden';
        project.style.pointerEvents = active ? '' : 'none';
        project.style.position = active ? '' : 'absolute';
        project.style.inset = active ? '' : '0';
        project.style.zIndex = active ? '' : '0';
        if (active) project.removeAttribute('aria-hidden');
        else project.setAttribute('aria-hidden', 'true');
      }
      const all = frames(grid);
      const full = all.find(frame => frame.dataset.appId === maximized);
      grid.dataset.maximized = full ? full.dataset.appId : '';
      all.forEach(frame => {
        frame.dataset.maximized = String(frame === full);
        frame.dataset.selected = String(frame.dataset.appId === window.__libroSelectedApp);
        frame.setAttribute('role', 'region');
        frame.setAttribute('aria-label', frame.dataset.appName || 'Terminal');
      });
      layoutDocks(grid, all, full);
    });
    observer.observe(document.getElementById('main-area'), {childList:true, subtree:true, attributes:true, attributeFilter:['style', 'data-dock', 'data-task-title', 'data-app-name']});
  }
  function renderToolRail() {
    const rail = document.getElementById('workspace-tool-buttons'); if (!rail) return;
    const list = window.__libroPlugins.filter(p => p.dock === 'right' && !p.disabled && !p.removed);
    const signature = JSON.stringify(list);
    if (rail.dataset.signature === signature) return;
    rail.dataset.signature = signature; rail.replaceChildren();
    const icons = {terminal:'terminal',browser:'language',files:'folder_open',notes:'description',nvim:'edit',lazyrepo:'account_tree',lazydata:'storage'};
    list.forEach(p => { const entry = button(p.name, icons[p.id] || (p.type === 'url' ? 'language' : 'terminal'), () => tool(p.id)); applyToolIcon(entry, p); entry.dataset.toolId = p.id; rail.append(entry); });
    updateToolHints();
  }
  function restoreAgentFocus(grid) {
    const agents = frames(grid).filter(frame => frame.dataset.dock === 'center');
    const agent = agents.find(frame => frame.dataset.appId === dockState(grid).agent) || agents[0];
    if (agent) select(agent.dataset.appId); else refresh();
  }
  function tool(id) {
    const grid = activeGrid(); if (!grid) return;
    const state = dockState(grid);
    const matches = frames(grid).filter(frame => frame.dataset.dock === 'right' && frame.dataset.plugin === id);
    const existing = matches.find(frame => frame.dataset.appId === window.__libroSelectedApp) ||
      matches.find(frame => frame.dataset.appId === state.right) || matches[0];
    if (!existing) { openPlugin(id, 'right'); return; }
    if (existing.dataset.dockVisible === 'true' && existing.dataset.appId === window.__libroSelectedApp) {
      state.hidden.add(existing.dataset.appId); restoreAgentFocus(grid);
    } else select(existing.dataset.appId);
  }
  function newBrowser() { openPlugin('browser', 'right'); }
  function navigateBrowser(delta) {
    const grid = activeGrid(); if (!grid) return;
    const browsers = frames(grid).filter(frame => frame.dataset.appType === 'url' && !['files', 'notes'].includes(frame.dataset.plugin));
    if (!browsers.length) return;
    const selected = browsers.findIndex(frame => frame.dataset.appId === window.__libroSelectedApp);
    const state = dockState(grid);
    const current = selected >= 0 ? selected : browsers.findIndex(frame => frame.dataset.appId === (state.browser || state.right));
    const next = current < 0 ? (delta > 0 ? 0 : browsers.length - 1) : (current + delta + browsers.length) % browsers.length;
    select(browsers[next].dataset.appId);
  }
  let toolKeys = window.__libroToolKeys || {};
  function syncWorkspaceShortcuts() {
    window.libroElectron?.setWorkspaceShortcuts?.(Object.values(toolKeys));
  }
  syncWorkspaceShortcuts();
  function shortcut(event) {
    const key = event.key === '+' ? '=' : event.key;
    if (!/^[a-z0-9=,.\[\]\-]$/i.test(key) || !(event.ctrlKey || event.altKey || event.metaKey)) return '';
    return (event.ctrlKey ? 'Ctrl+' : '') + (event.altKey ? 'Alt+' : '') + (event.shiftKey && event.key !== '+' ? 'Shift+' : '') + (event.metaKey ? 'Meta+' : '') + key.toUpperCase();
  }
  function zoom(action) {
    const method = {'zoom-in':'zoomIn', 'zoom-out':'zoomOut', 'zoom-reset':'zoomReset'}[action];
    if (!method) return;
    if (window.libroElectron?.[method]) { window.libroElectron[method](); return; }
    const current = Number(document.documentElement.style.zoom) || 1;
    document.documentElement.style.zoom = action === 'zoom-reset' ? 1 : Math.max(0.5, Math.min(2, current + (action === 'zoom-in' ? 0.1 : -0.1)));
    window.dispatchEvent(new Event('resize'));
  }
  function fillToolKeys(bindings) {
    document.querySelectorAll('[data-tool-key]').forEach(input => input.value = bindings[input.dataset.toolKey] || '');
  }
  function updateToolHints() {
    syncWorkspaceShortcuts();
    document.querySelectorAll('.ws-tool-rail button, .ws-sidebar button.ws-button').forEach(button => {
      const name = button.getAttribute('aria-label');
      const plugin = window.__libroPlugins.find(plugin => plugin.id === button.dataset.toolId);
      if (plugin) button.title = name + (toolKeys[plugin.id] ? ' (' + toolKeys[plugin.id] + ')' : '');
      else if (name === 'New thread') button.title = name + (toolKeys['new-thread'] ? ' (' + toolKeys['new-thread'] + ')' : '');
      else if (name === 'Toggle projects') button.title = name + (toolKeys['toggle-projects'] ? ' (' + toolKeys['toggle-projects'] + ')' : '');
    });
  }
  function saveToolKeys() {
    const bindings = {...toolKeys};
    document.querySelectorAll('#tool-key-form [data-tool-key]').forEach(input => bindings[input.dataset.toolKey] = input.value);
    document.querySelector('#tool-key-form [type=submit]').disabled = true;
    document.getElementById('tool-key-status').textContent = 'Saving…';
    call('settings.tool-keys', {bindings});
  }
  function resetToolKeys() { document.querySelectorAll('#tool-key-form [data-tool-key]').forEach(input => input.value = window.__libroDefaultToolKeys[input.dataset.toolKey] || ''); }
  function toolKeysSaved(bindings, message) {
    document.querySelector('#tool-key-form [type=submit]').disabled = false;
    document.getElementById('tool-key-status').textContent = message;
    if (bindings) { toolKeys = bindings; updateToolHints(); }
  }
  let lastCtrlA = 0;
  function hideTools() {
    const grid = activeGrid(); if (!grid) return;
    maximized = '';
    const state = dockState(grid);
    frames(grid).forEach(frame => { if (frame.dataset.dock === 'right') state.hidden.add(frame.dataset.appId); });
    state.bottom = false;
    restoreAgentFocus(grid);
  }
  window.addEventListener('keydown', event => {
    const input = event.target.closest?.('[data-tool-key]');
    if (input) {
      if (event.key === 'Tab') return;
      event.preventDefault(); event.stopImmediatePropagation();
      if (event.key === 'Backspace' || event.key === 'Delete') input.value = '';
      else if (shortcut(event)) input.value = shortcut(event);
      return;
    }
    const binding = shortcut(event);
    const zoomAction = binding && ['zoom-in', 'zoom-out', 'zoom-reset'].find(id => toolKeys[id] === binding);
    if (zoomAction) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) zoom(zoomAction);
      return;
    }
    if (binding && binding === toolKeys['settings']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat && (!document.getElementById('workspace-settings').hidden || !document.querySelector('dialog[open], #libro-confirm-popover'))) settings();
      return;
    }
    if (!document.getElementById('workspace-settings').hidden || document.querySelector('dialog[open], #libro-confirm-popover')) return;
    if (event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'a') {
      if (event.repeat) return;
      const now = performance.now();
      if (lastCtrlA && now - lastCtrlA < 450) {
        lastCtrlA = 0; event.preventDefault(); event.stopImmediatePropagation(); hideTools(); return;
      }
      lastCtrlA = now;
    } else if (!['Control', 'Shift', 'Alt', 'Meta'].includes(event.key)) lastCtrlA = 0;
    const number = /^[1-9]$/.test(event.key) ? event.key : /^Digit[1-9]$/.test(event.code || '') ? event.code.slice(-1) : '';
    if (event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && number) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) {
        renderProjectShortcuts();
        document.querySelector('.ws-project-row[data-project-shortcut="' + number + '"]')?.click();
      }
      return;
    }
    if (binding && (binding === toolKeys['panel-size-down'] || binding === toolKeys['panel-size-up'])) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) window.__libroResizeSelectedAppStep(binding === toolKeys['panel-size-down'] ? -1 : 1, sid);
      return;
    }
    if (binding && binding === toolKeys['panel-size-max']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat && window.__libroSelectedApp) call('app.resize.max.toggle', {maxPixel: window.__libroAppWidthMaxPixel()});
      return;
    }
    if (binding && binding === toolKeys['toggle-projects']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) { prefs.projects = !prefs.projects; save(); refresh(); }
      return;
    }
    if (binding && (binding === toolKeys['previous-agent'] || binding === toolKeys['next-agent'])) {
      const grid = activeGrid();
      const all = grid ? frames(grid) : [];
      const panels = [
        ...all.filter(frame => frame.dataset.dock === 'center'),
        ...all.filter(frame => frame.dataset.dock === 'right' && frame.dataset.dockVisible === 'true' && frame.dataset.toolOverlay !== 'true')
      ];
      if (panels.length < 2) return;
      event.preventDefault(); event.stopImmediatePropagation();
      if (event.repeat) return;
      const state = dockState(grid);
      const selected = panels.findIndex(frame => frame.dataset.appId === window.__libroSelectedApp);
      const current = selected >= 0 ? selected : Math.max(0, panels.findIndex(frame => frame.dataset.appId === state.agent));
      if (grid.querySelector('[data-tool-overlay=true][data-dock-visible=true]')) {
        frames(grid).filter(frame => frame.dataset.dock === 'right').forEach(frame => state.hidden.add(frame.dataset.appId));
      }
      const step = binding === toolKeys['previous-agent'] ? -1 : 1;
      select(panels[(current + step + panels.length) % panels.length].dataset.appId);
      return;
    }
    if (binding && binding === toolKeys['new-browser']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) newBrowser();
      return;
    }
    if (binding && (binding === toolKeys['previous-browser'] || binding === toolKeys['next-browser'])) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) navigateBrowser(binding === toolKeys['previous-browser'] ? -1 : 1);
      return;
    }
    if (binding && binding === toolKeys['new-thread']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) newThread();
      return;
    }
    if (binding && binding === toolKeys['new-agent']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) launcher('center');
      return;
    }
    if (binding && binding === toolKeys['project-picker']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) {
        if (window.__libroOpenProjectDialogSearch) window.__libroOpenProjectDialogSearch();
        else call('project.dialog.open');
      }
      return;
    }
    if (binding && (binding === toolKeys['run-project'] || binding === toolKeys['stop-project'])) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) call(binding === toolKeys['run-project'] ? 'project.command.run' : 'project.command.stop');
      return;
    }
    if (binding && binding === toolKeys['close-project']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) {
        const grid = activeGrid();
        if (!grid) return;
        call('project.close.check');
      }
      return;
    }
    if (binding && binding === toolKeys['close-panel']) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (event.repeat) return;
      const grid = activeGrid();
      const frame = grid && frames(grid).find(frame => frame.dataset.appId === window.__libroSelectedApp && frame.dataset.dockVisible === 'true');
      if (frame) call('app.close', {id:frame.dataset.appId});
      return;
    }
    if (binding === 'Ctrl+A') {
      const grid = activeGrid();
      const agents = grid && frames(grid).filter(frame => frame.dataset.dock === 'center');
      if (!grid) return;
      event.preventDefault(); event.stopImmediatePropagation();
      if (event.repeat) return;
      if (!agents.length) { launcher('center'); return; }
      const state = dockState(grid);
      if (grid.querySelector('[data-tool-overlay=true][data-dock-visible=true]')) {
        frames(grid).filter(frame => frame.dataset.dock === 'right').forEach(frame => state.hidden.add(frame.dataset.appId));
      }
      const agent = agents.find(frame => frame.dataset.appId === state.agent) || agents[0];
      select(agent.dataset.appId);
      return;
    }
    if (binding === 'Ctrl+D' && event.target.closest?.('[data-terminal]')) return;
    const id = binding && Object.keys(toolKeys).find(id => toolKeys[id] === binding);
    if (!id) return;
    event.preventDefault(); event.stopImmediatePropagation();
    if (!event.repeat) tool(id);
  }, true);
  window.addEventListener('keydown', event => {
    if (!event.ctrlKey || event.metaKey || event.altKey || event.shiftKey ||
        (event.code !== 'Backquote' && event.key !== '`')) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (!event.repeat) bottom();
  }, true);
  function terminalExited(id) {
    const frame = document.getElementById('frame-' + id);
    if (!frame || frame.dataset.dock !== 'bottom') return;
    const state = dockState(frame.parentElement);
    if (state.bottomID === id || !state.bottomID) state.bottom = false;
    call('app.close', {id});
    refresh();
  }
  function restartProject(update) {
    const grid = activeGrid();
    const selected = window.__libroSelectedApp;
    keepBottomHidden = !!grid && !dockState(grid).bottom;
    try {
      update();
    } finally {
      if (keepBottomHidden) {
        dockState(grid).bottom = false;
        window.__libroSelectedApp = document.getElementById('frame-' + selected) ? selected : '';
      }
      refresh();
      keepBottomHidden = false;
    }
  }
  function bottom() {
    const grid = activeGrid(); if (!grid) return;
    const state = dockState(grid);
    const terminals = frames(grid).filter(frame => frame.dataset.dock === 'bottom');
    const terminal = terminals.find(frame => frame.dataset.appId === state.bottomID) || terminals[0];
    if (!terminal) { openPlugin('terminal', 'bottom'); return; }
    state.bottom = !state.bottom || terminal.dataset.appId !== window.__libroSelectedApp;
    if (state.bottom) select(terminal.dataset.appId); else restoreAgentFocus(grid);
  }
  function layoutDocks(grid, all, full) {
    const state = dockState(grid);
    const selected = all.find(frame => frame.dataset.appId === window.__libroSelectedApp);
    // Server-created panels must become visible before terminal hydration.
    all.forEach(frame => {
      if (!frame.dataset.dockSeen) {
        frame.dataset.dockSeen = 'true';
        if (frame.dataset.dock === 'bottom' && !keepBottomHidden) state.bottom = true;
        if (frame.dataset.dock === 'right') state.right = frame.dataset.appId;
      }
    });
    if (selected && grid.dataset.lastSelected !== selected.dataset.appId) {
        state.hidden.delete(selected.dataset.appId);
        if (selected.dataset.dock === 'right') state.right = selected.dataset.appId;
        // Restoring selection on a project switch must preserve bottom visibility.
        // Explicit selection opens the terminal in select().
    }
    if (selected?.dataset.appType === 'url' && !['files', 'notes'].includes(selected.dataset.plugin)) state.browser = selected.dataset.appId;
    if (selected?.dataset.dock === 'center') state.agent = selected.dataset.appId;
    grid.dataset.lastSelected = selected?.dataset.appId || '';
    const width = frame => frame.style.width.endsWith('%') ? grid.clientWidth : parseFloat(frame.style.width) || frame.offsetWidth;
    const center = all.filter(frame => frame.dataset.dock === 'center');
    const tools = all.filter(frame => frame.dataset.dock === 'right' && !state.hidden.has(frame.dataset.appId));
    if (grid.parentElement && grid.parentElement.style.display !== 'none' && grid.parentElement.getAttribute('aria-hidden') !== 'true') {
      const rememberedAgent = center.find(frame => frame.dataset.appId === state.agent) || center[0];
      window.__libroActiveAgentID = rememberedAgent?.dataset.appId || '';
    }
    const right = [tools.find(frame => frame.dataset.appId === state.right) || tools.at(-1)].filter(Boolean);
    const terminals = all.filter(frame => frame.dataset.dock === 'bottom');
    if (selected?.dataset.dock === 'bottom') state.bottomID = selected.dataset.appId;
    const terminal = terminals.find(frame => frame.dataset.appId === state.bottomID) || terminals[0];
    const bottomVisible = terminal && state.bottom && !full;
    const overlay = !full && right.length > 0 && (center.length ? center.reduce((sum, frame) => sum + width(frame), 0) : 320) + width(right[0]) > grid.clientWidth;
    const visible = full ? [full] : [...center, ...right];
    const columns = (overlay ? center : visible).map(frame => full ? grid.clientWidth + 'px' : width(frame) + 'px');
    if (center.length === 1 && !full) columns[0] = 'minmax(' + width(center[0]) + 'px, 1fr)';
    if (!center.length && !full) columns.unshift('minmax(0, 1fr)');
    grid.style.gridTemplateColumns = columns.join(' ') || 'minmax(0, 1fr)';
    grid.style.gridTemplateRows = bottomVisible ? 'minmax(120px, 1fr) minmax(120px, 25.2875%)' : 'minmax(0, 1fr)';
    all.forEach(frame => {
      const index = visible.indexOf(frame);
      const show = index >= 0 || (frame === terminal && bottomVisible);
      frame.dataset.dockVisible = String(!!show);
      frame.inert = !show;
      const isOverlay = overlay && frame === right[0];
      frame.dataset.toolOverlay = String(isOverlay);
      frame.style.left = isOverlay ? Math.max(0, grid.clientWidth - width(frame)) + grid.scrollLeft + 'px' : '';
      frame.style.justifySelf = center.length === 1 && frame === center[0] && !full && !overlay ? 'center' : 'start';
      frame.style.gridRow = frame === terminal && !full ? '2' : '1';
      frame.style.gridColumn = frame === terminal && !full ? '1 / -1' : String(index + 1 + (!center.length && !full ? 1 : 0));
      if (isOverlay) frame.style.gridColumn = '1 / -1';
      if (frame === terminal && !frame.querySelector('[data-hide-bottom]')) {
        const hide = button('Hide terminal (keep running)', 'expand_more', bottom);
        hide.dataset.hideBottom = ''; frame.querySelector('[data-app-toolbar]').append(hide);
      }
    });
    const placeholder = grid.querySelector(':scope > .ws-empty');
    if (center.length || full) placeholder?.remove();
    else if (!placeholder) grid.append(empty('center', grid));
    let tabs = grid.parentElement.querySelector('.ws-tool-tabs');
    if (!tabs) { tabs = node('div', 'ws-tool-tabs'); tabs.setAttribute('aria-label', 'Agents and tools'); grid.before(tabs); }
    const signature = [...center, ...all.filter(frame => frame.dataset.dock === 'right'), ...(terminals.length > 1 ? terminals : [])].map(frame => [frame.dataset.appId, frame.dataset.appName, frame.dataset.selected, frame.querySelector('[data-size-trigger]')?.textContent || 'MD', frame.dataset.dock]);
    if (tabs.dataset.signature !== JSON.stringify(signature)) {
      tabs.dataset.signature = JSON.stringify(signature); tabs.replaceChildren();
      signature.forEach(([id, name, shown, size, dock]) => {
        const group = node('div', 'ws-agent-tab ws-tool-tab-group'); group.dataset.selected = shown; group.dataset.tabDock = dock;
        const tab = node('button', 'ws-tool-tab', name); tab.type = 'button'; tab.setAttribute('aria-pressed', shown);
        tab.onclick = () => {
          if (dock === 'center' && grid.querySelector('[data-tool-overlay=true][data-dock-visible=true]')) {
            all.filter(frame => frame.dataset.dock === 'right').forEach(frame => state.hidden.add(frame.dataset.appId));
          }
          if (dock === 'right' && shown === 'true') {
            state.hidden.add(id);
            if (id === window.__libroSelectedApp) restoreAgentFocus(grid); else refresh();
          }
          else select(id);
        };
        const sizes = node('div', 'ws-tool-sizes'); sizes.dataset.sizeBadges = '';
        const trigger = node('button', 'ws-size-trigger', size); trigger.type = 'button'; trigger.dataset.sizeTrigger = '';
        trigger.setAttribute('aria-label', name + ' panel size: ' + size); trigger.setAttribute('aria-expanded', 'false'); trigger.setAttribute('aria-controls', 'tool-sizes-' + id);
        const picker = node('div', 'ws-size-picker'); picker.id = 'tool-sizes-' + id; picker.setAttribute('popover', 'auto'); picker.setAttribute('role', 'group'); picker.setAttribute('aria-label', name + ' panel size');
        document.getElementById('frame-' + id).querySelectorAll('[data-resize-width]').forEach(original => {
          const option = node('button', '', original.textContent); option.type = 'button';
          option.dataset.resizeWidth = original.dataset.resizeWidth; option.setAttribute('aria-label', original.getAttribute('aria-label')); option.setAttribute('aria-pressed', original.getAttribute('aria-pressed'));
          option.onclick = () => { picker.hidePopover(); window.__libroResizeApp(id, option.dataset.resizeWidth, sid); };
          picker.append(option);
        });
        sizes.append(trigger, picker); group.append(tab); if (dock !== 'bottom') group.append(sizes); tabs.append(group);
      });
    }
    tabs.hidden = signature.length === 0;
  }
  function toggle(zone) {
    if (zone !== 'projects') return;
    prefs.projects = !prefs.projects; save(); refresh();
  }
  function maximize(id) {
    maximized = maximized === id ? '' : id;
    refresh();
    window.__libroCenterSelectedApp();
  }
  function navigate(delta) {
    const grid = activeGrid(); if (!grid) return;
    const apps = frames(grid);
    const index = apps.findIndex(frame => frame.dataset.appId === window.__libroSelectedApp);
    const next = apps[Math.max(0, Math.min(apps.length - 1, index + delta))];
    if (next) select(next.dataset.appId);
  }
  let savedWidth = 'md', savedToolWidth = 'lg';
  let settingsFocus;
  function settings() {
    if (!document.getElementById('workspace-settings').hidden) { closeSettings(); return; }
    settingsFocus = document.activeElement;
    call('settings.open');
  }
  function themePreference() {
    try {
      const mode = localStorage.getItem('theme');
      return mode === 'dark' || mode === 'light' ? mode : 'system';
    } catch (_) {
      return 'system';
    }
  }
  function saveTheme(mode) {
    if (!['system', 'light', 'dark'].includes(mode)) return;
    const status = document.getElementById('workspace-theme-status');
    try {
      window.setTheme(mode);
      status.textContent = '';
    } catch (_) {
      document.getElementById('workspace-theme').value = themePreference();
      status.textContent = 'Could not save theme. Please try again.';
    }
  }
  let savedThreadAgent = '';
  function fillThreadAgents() {
    const select = document.getElementById('default-thread-agent');
    select.replaceChildren(new Option('Choose manually', ''));
    (window.__libroPlugins || []).filter(p => p.dock === 'center' && p.type === 'terminal' && !p.disabled && !p.removed).forEach(p => select.add(new Option(p.name, p.id)));
    select.value = [...select.options].some(option => option.value === savedThreadAgent) ? savedThreadAgent : '';
  }
  function saveThreadAgent(agent) {
    document.getElementById('default-thread-agent').disabled = true;
    document.getElementById('default-thread-agent-status').textContent = 'Saving…';
    call('settings.thread-agent', {agent});
  }
  function threadAgentSaved(ok, agent) {
    savedThreadAgent = agent;
    fillThreadAgents();
    document.getElementById('default-thread-agent').disabled = false;
    document.getElementById('default-thread-agent-status').textContent = ok ? 'Saved.' : 'Could not save. Choose an enabled agent and try again.';
  }
  function saveBrowserControl(enabled) {
    document.getElementById('browser-control-enabled').disabled = true;
    document.getElementById('browser-control-enabled-status').textContent = 'Saving…';
    call('settings.browser-control', {enabled: !!enabled});
  }
  function browserControlSaved(ok, enabled) {
    const select = document.getElementById('browser-control-enabled');
    select.disabled = false;
    select.value = enabled ? 'on' : 'off';
    document.getElementById('browser-control-enabled-status').textContent = ok ? 'Saved.' : 'Could not save. Try again.';
    if (window.__libroApplyBrowserControlSetting) window.__libroApplyBrowserControlSetting(enabled);
  }
  function savePageTools(enabled) {
    const select = document.getElementById('page-tools-autoexecute');
    const status = document.getElementById('page-tools-autoexecute-status');
    if (select) select.disabled = true;
    if (status) status.textContent = 'Saving…';
    call('settings.page-tools', {autoexecute: !!enabled});
  }
  function pageToolsSaved(ok, enabled) {
    window.__libroPageToolsAutoExecute = !!enabled;
    const select = document.getElementById('page-tools-autoexecute');
    const status = document.getElementById('page-tools-autoexecute-status');
    if (select) { select.disabled = false; select.value = enabled ? 'on' : 'off'; }
    if (status) status.textContent = ok ? 'Saved.' : 'Could not save. Try again.';
  }
  function addAgentEnvironment(name = '', saved = false) {
    const row = node('div', 'ws-settings-row ws-agent-command-row ws-environment-row');
    const key = node('input', 'ws-agent-command');
    key.dataset.environmentName = ''; key.value = name; key.placeholder = 'VARIABLE_NAME'; key.required = true;
    key.autocomplete = 'off'; key.spellcheck = false; key.setAttribute('aria-label', 'Environment variable name');
    const value = node('input', 'ws-agent-command');
    value.dataset.environmentValue = ''; value.type = 'password'; value.placeholder = saved ? 'Saved value' : 'Value';
    value.autocomplete = 'new-password'; value.setAttribute('aria-label', name ? name + ' value' : 'Environment variable value');
    if (!saved) value.required = true;
    row.dataset.originalName = saved ? name : '';
    row.append(key, value, button('Remove variable', 'delete_outline', () => row.remove()));
    document.getElementById('agent-environment-rows').append(row);
    if (!saved) key.focus();
    return row;
  }
  function fillAgentEnvironment(names = []) {
    document.getElementById('agent-environment-rows').replaceChildren();
    names.forEach(name => addAgentEnvironment(name, true));
  }
  function saveAgentEnvironment(form) {
    const entries = [...form.querySelectorAll('.ws-environment-row')].map(row => ({
      name: row.querySelector('[data-environment-name]').value,
      value: row.querySelector('[data-environment-value]').value,
      originalName: row.dataset.originalName || '',
    }));
    form.querySelector('[type=submit]').disabled = true;
    form.querySelector('[role=status]').textContent = 'Saving…';
    call('settings.agent-environment', {entries});
  }
  function agentEnvironmentSaved(ok, message, names) {
    const form = document.getElementById('agent-environment-form');
    form.querySelector('[type=submit]').disabled = false;
    form.querySelector('[role=status]').textContent = message;
    if (ok) fillAgentEnvironment(names);
  }
  function showSettings(width, commands = {}, bindings = toolKeys, toolWidth = 'lg', threadAgent = '', pageToolsAutoExecute = false, environment = [], browserControlEnabled = true) {
    window.__libroPageToolsAutoExecute = !!pageToolsAutoExecute;
    document.getElementById('browser-control-enabled').value = browserControlEnabled ? 'on' : 'off';
    document.getElementById('browser-control-enabled-status').textContent = '';
    savedThreadAgent = threadAgent;
    fillThreadAgents();
    document.getElementById('default-thread-agent-status').textContent = '';
    document.getElementById('notification-sound').value = prefs.notificationSound === false ? 'off' : 'on';
    document.getElementById('notification-sound-status').textContent = '';
    document.getElementById('page-tools-autoexecute').value = pageToolsAutoExecute ? 'on' : 'off';
    document.getElementById('page-tools-autoexecute-status').textContent = '';
    document.getElementById('workspace-theme').value = themePreference();
    document.getElementById('workspace-theme-status').textContent = '';
    fillAgentEnvironment(environment);
    document.querySelector('#agent-environment-form [role=status]').textContent = '';
    toolKeys = bindings; fillToolKeys(bindings); updateToolHints();
    document.getElementById('tool-key-status').textContent = '';
    document.getElementById('agent-command-rows').replaceChildren();
    document.getElementById('tool-command-rows').replaceChildren();
    window.__libroPlugins.filter(p => p.dock === 'right' && ['terminal', 'url'].includes(p.type) && !['terminal', 'browser', 'files', 'notes'].includes(p.id) && !p.removed).forEach(p => addAgentRow(p, p.type === 'url' ? p.url : p.command, true));
    removedAgents = Object.fromEntries(window.__libroPlugins.filter(p => p.removed && p.dock === 'center' && p.type === 'terminal').map(p => [p.id, true]));
    window.__libroPlugins.filter(p => !p.removed && p.dock === 'center' && p.type === 'terminal').forEach(p => addAgentRow(p, commands[p.id] || p.command));
    fillAutolaunchAgents();
    document.getElementById('autolaunch-agent').value = window.__libroPlugins.find(p => p.autolaunch && !p.disabled && !p.removed)?.id || '';
    document.querySelector('#agent-commands-form [role=status]').textContent = '';
    savedWidth = width;
    savedToolWidth = toolWidth;
    document.getElementById('default-panel-width').value = width;
    document.getElementById('default-tool-panel-width').value = toolWidth;
    document.getElementById('workspace-settings-status').textContent = '';
    document.getElementById('workspace-settings').hidden = false;
    document.querySelectorAll('.ws-project').forEach(project => project.inert = true);
    document.getElementById('workspace-settings-title').focus();
  }
  function closeSettings() {
    document.getElementById('workspace-settings').hidden = true;
    document.querySelectorAll('.ws-project').forEach(project => project.inert = false);
    if (settingsFocus?.isConnected) settingsFocus.focus();
  }
  function saveSettings(width, tool = false) {
    document.getElementById(tool ? 'default-tool-panel-width' : 'default-panel-width').disabled = true;
    document.getElementById('workspace-settings-status').textContent = 'Saving…';
    call('settings.width', {width, tool});
  }
  let removedAgents = {};
  let draggedAgentRow = null;
  function addAgentRow(plugin, command, toolRow = false) {
    const row = node('div', 'ws-settings-row ws-agent-command-row ws-agent-config-row');
    row.dataset.toolType = plugin.type || 'terminal';
    row.dataset.agentId = plugin.id; row.dataset.custom = String(!!plugin.custom);
    const name = node('input', 'ws-agent-command');
    name.value = plugin.name; name.placeholder = toolRow ? (plugin.type === 'url' ? 'Website name' : 'Tool name') : 'Agent name'; name.required = true; name.setAttribute('aria-label', toolRow ? 'Tool name' : 'Agent name'); name.dataset.agentName = '';
    const input = node('input', 'ws-agent-command'); input.id = 'agent-command-' + plugin.id; input.dataset.agentCommand = plugin.id;
    input.value = command || ''; input.placeholder = 'CLI command'; input.required = true; input.spellcheck = false; input.setAttribute('aria-label', plugin.name ? plugin.name + ' command' : 'Custom agent command');
    if (toolRow && plugin.type === 'url') { input.type = 'url'; input.placeholder = 'https://example.com'; input.setAttribute('aria-label', 'Website URL'); }
    const toggle = node('label', 'ws-agent-enabled');
    const checkbox = node('input', ''); checkbox.type = 'checkbox'; checkbox.checked = !plugin.disabled; checkbox.dataset.agentEnabled = '';
    checkbox.setAttribute('aria-label', 'Enable ' + (plugin.name || 'custom agent'));
    toggle.append(checkbox);
    const remove = button(toolRow ? 'Remove tool' : 'Remove agent', 'delete_outline', () => {
      if (checkbox.checked) return;
      if (toolRow) { if (!window.__libroPlugins.some(p => p.id === plugin.id)) { row.remove(); return; } row.dataset.removed = 'true'; row.hidden = true; row.querySelectorAll('input').forEach(input => input.required = false); return; }
      if (window.__libroPlugins.some(p => p.id === plugin.id)) removedAgents[plugin.id] = true;
      row.remove();
      updateAgentOrderButtons();
    });
    remove.hidden = checkbox.checked;
    checkbox.onchange = () => {
      remove.hidden = checkbox.checked;
      if (!toolRow) fillAutolaunchAgents();
    };
    if (!toolRow) name.oninput = fillAutolaunchAgents;
    row.prepend(toggle, name, input); row.append(remove); document.getElementById(toolRow ? 'tool-command-rows' : 'agent-command-rows').append(row);
    if (toolRow) {
      const field = node('div', 'ws-tool-shortcut');
      const key = node('input', 'ws-agent-command');
      key.dataset.toolKey = plugin.id; key.readOnly = true; key.placeholder = 'Press shortcut';
      key.value = toolKeys[plugin.id] || '';
      key.setAttribute('aria-label', (plugin.name || 'Custom tool') + ' shortcut');
      key.setAttribute('aria-describedby', 'tool-shortcut-help');
      field.append(key, button('Clear shortcut', 'close', () => { key.value = ''; key.focus(); }));
      row.insertBefore(field, remove);
    }
    if (!toolRow) {
      const controls = node('div', 'ws-agent-order');
      const handle = button('Drag to reorder agent', 'drag_indicator', () => {});
      handle.classList.add('ws-agent-drag');
      handle.draggable = true;
      handle.ondragstart = event => {
        draggedAgentRow = row;
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('text/plain', plugin.id);
        row.classList.add('ws-agent-dragging');
      };
      handle.ondragend = () => {
        draggedAgentRow = null;
        row.classList.remove('ws-agent-dragging');
        row.parentElement?.querySelectorAll('[data-agent-drop]').forEach(target => delete target.dataset.agentDrop);
      };
      row.ondragover = event => {
        if (!draggedAgentRow || draggedAgentRow === row || draggedAgentRow.parentElement !== row.parentElement) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'move';
        row.parentElement.querySelectorAll('[data-agent-drop]').forEach(target => delete target.dataset.agentDrop);
        const bounds = row.getBoundingClientRect();
        row.dataset.agentDrop = event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after';
      };
      row.ondragleave = event => {
        if (!row.contains(event.relatedTarget)) delete row.dataset.agentDrop;
      };
      row.ondrop = event => {
        if (!draggedAgentRow || draggedAgentRow === row || draggedAgentRow.parentElement !== row.parentElement) return;
        event.preventDefault();
        const bounds = row.getBoundingClientRect();
        if (event.clientY < bounds.top + bounds.height / 2) row.before(draggedAgentRow); else row.after(draggedAgentRow);
        delete row.dataset.agentDrop;
        updateAgentOrderButtons();
      };
      controls.append(handle);
      [-1, 1].forEach(direction => {
        const move = button(direction < 0 ? 'Move agent up' : 'Move agent down', direction < 0 ? 'arrow_upward' : 'arrow_downward', () => {
          const sibling = direction < 0 ? row.previousElementSibling : row.nextElementSibling;
          if (!sibling) return;
          if (direction < 0) sibling.before(row); else sibling.after(row);
          updateAgentOrderButtons();
          move.focus();
        });
        move.dataset.agentMove = String(direction);
        controls.append(move);
      });
      row.append(controls);
      updateAgentOrderButtons();
    }
    return row;
  }
  function fillAutolaunchAgents() {
    const select = document.getElementById('autolaunch-agent');
    const selected = select.value;
    select.replaceChildren(new Option('Off', ''));
    document.querySelectorAll('#agent-command-rows [data-agent-id]').forEach(row => {
      if (row.querySelector('[data-agent-enabled]').checked) {
        select.add(new Option(row.querySelector('[data-agent-name]').value.trim() || 'Custom agent', row.dataset.agentId));
      }
    });
    select.value = [...select.options].some(option => option.value === selected) ? selected : '';
  }
  function updateAgentOrderButtons() {
    fillAutolaunchAgents();
    const rows = [...document.getElementById('agent-command-rows').children];
    rows.forEach((row, i) => row.querySelectorAll('[data-agent-move]').forEach(move => {
      move.disabled = Number(move.dataset.agentMove) < 0 ? i === 0 : i === rows.length - 1;
    }));
  }
  function applyToolIcon(entry, plugin) {
    if (!plugin.icon?.startsWith('data:image/')) return;
    const image = node('img', 'ws-tool-icon');
    image.alt = ''; image.style.display = 'none';
    image.onerror = () => image.remove();
    image.onload = () => { const fallback = entry.querySelector('i'); if (fallback) fallback.remove(); image.style.display = ''; };
    entry.prepend(image); image.src = plugin.icon;
  }
  function addCustomTool(type = 'terminal') {
    addAgentRow({id:'custom-tool-' + crypto.randomUUID(), name:'', custom:true, type}, '', true).querySelector('[data-agent-name]').focus();
  }
  function saveTools(form) {
    const tools = window.__libroPlugins.filter(p => p.dock === 'right' && p.removed);
    form.querySelectorAll('[data-agent-id]').forEach(row => tools.push({id:row.dataset.agentId, name:row.querySelector('[data-agent-name]').value.trim(), [row.dataset.toolType === 'url' ? 'url' : 'command']:row.querySelector('[data-agent-command]').value, type:row.dataset.toolType, dock:'right', custom:row.dataset.custom === 'true', disabled:!row.querySelector('[data-agent-enabled]').checked, removed:row.dataset.removed === 'true'}));
    form.querySelector('[type=submit]').disabled = true;
    form.querySelector('[role=status]').textContent = 'Saving…';
    const bindings = {...toolKeys};
    form.querySelectorAll('[data-tool-key]').forEach(input => bindings[input.dataset.toolKey] = input.closest('[data-agent-id]').dataset.removed === 'true' ? '' : input.value);
    call('settings.tools', {tools, bindings});
  }
  function toolsSaved(plugins, message, bindings) {
    const form = document.getElementById('tool-commands-form');
    form.querySelector('[type=submit]').disabled = false; form.querySelector('[role=status]').textContent = message;
    if (plugins) { window.__libroPlugins = plugins; if (bindings) { toolKeys = bindings; updateToolHints(); } refresh(); }
  }
  function addCustomAgent() {
    addAgentRow({id:'custom-' + crypto.randomUUID(), name:'', custom:true}, '').querySelector('input').focus();
  }
  function saveAgentCommand(form) {
    const commands = {}, disabled = {}, custom = [], names = {}, order = [];
    form.querySelectorAll('[data-agent-id]').forEach(row => {
      const id = row.dataset.agentId, command = row.querySelector('[data-agent-command]').value;
      order.push(id);
      names[id] = row.querySelector('[data-agent-name]').value.trim();
      commands[id] = command; disabled[id] = !row.querySelector('[data-agent-enabled]').checked;
      if (row.dataset.custom === 'true') custom.push({id, name:row.querySelector('[data-agent-name]').value.trim(), command, type:'terminal', dock:'center', custom:true});
    });
    form.querySelector('[type=submit]').disabled = true;
    form.querySelector('[role=status]').textContent = 'Saving…';
    Object.keys(removedAgents).forEach(id => disabled[id] = true);
    call('settings.agent-command', {commands, disabled, custom, names, order, removed:removedAgents, autolaunch:document.getElementById('autolaunch-agent').value});
  }
  function agentCommandSaved(message, plugins) {
    const form = document.getElementById('agent-commands-form');
    form.querySelector('[type=submit]').disabled = false;
    form.querySelector('[role=status]').textContent = message;
    if (plugins) {
      window.__libroPlugins = plugins;
      fillThreadAgents();
      document.querySelectorAll('.ws-grid > .ws-empty').forEach(el => el.remove());
      refresh();
    }
  }
  function settingsSaved(ok, tool = false) {
    const select = document.getElementById(tool ? 'default-tool-panel-width' : 'default-panel-width');
    if (ok) {
      if (tool) savedToolWidth = select.value;
      else savedWidth = select.value;
      refresh();
    } else select.value = tool ? savedToolWidth : savedWidth;
    select.disabled = false;
    document.getElementById('workspace-settings-status').textContent = ok ? 'Saved. New ' + (tool ? 'tool' : 'agent') + ' panels will use this width.' : 'Could not save. Please try again.';
  }
  window.libroWorkspace = {saveBrowserControl, browserControlSaved, saveThreadAgent, threadAgentSaved, newThread, threadArchived,newBrowser, navigateBrowser, restartProject, projectSettings, saveNotificationSound, saveTheme, savePageTools, pageToolsSaved, saveAgentEnvironment, agentEnvironmentSaved, addAgentEnvironment, saveTools, toolsSaved, addCustomTool, zoom, shortcutFor:id => toolKeys[id] || '', select, refresh, launcher, toggle, maximize, navigate, settings, showSettings, closeSettings, saveSettings, settingsSaved, saveToolKeys, resetToolKeys, toolKeysSaved, saveAgentCommand, agentCommandSaved, addCustomAgent, tool, bottom, terminalExited};
  // Scroll the existing strip; never reparent running terminals or webviews.
  window.__libroScrollToApp = frame => {
    if (!frame?.dataset.appId) return;
    const strip = frame.parentElement;
    refresh();
    if (frame.dataset.dockVisible !== 'true' || frame.dataset.toolOverlay === 'true') return;
    const viewport = strip.getBoundingClientRect();
    const panel = frame.getBoundingClientRect();
    if (panel.left < viewport.left || panel.width > strip.clientWidth) strip.scrollLeft += panel.left - viewport.left;
    else if (panel.right > viewport.left + strip.clientWidth) strip.scrollLeft += panel.right - viewport.left - strip.clientWidth;
  };
  document.getElementById('main-area').addEventListener('scroll', event => {
    const grid = event.target;
    if (!grid.matches('[data-workspace-project]')) return;
    const overlay = grid.querySelector('[data-tool-overlay=true]');
    if (overlay) overlay.style.left = Math.max(0, grid.clientWidth - overlay.offsetWidth) + grid.scrollLeft + 'px';
  }, true);
  new ResizeObserver(schedule).observe(document.getElementById('main-area'));
  window.addEventListener('resize', schedule);
  root.addEventListener('pointerdown', event => {
    const frame = event.target.closest('[data-app-id]');
    if (frame?.querySelector('[data-terminal-app]') && !event.target.closest('[data-app-toolbar], button, a, input, select, [contenteditable=true]')) {
      requestAnimationFrame(() => {
        if (window.__libroSelectedApp === frame.dataset.appId) window.__libroFocusTerminalFrame?.(frame.dataset.appId);
      });
    }
    if (frame?.dataset.dock === 'center' && frame.parentElement.querySelector('[data-tool-overlay=true][data-dock-visible=true]')) {
      const grid = frame.parentElement;
      const state = dockState(grid);
      frames(grid).filter(panel => panel.dataset.dock === 'right').forEach(panel => state.hidden.add(panel.dataset.appId));
      window.__libroSelectedApp = frame.dataset.appId;
      refresh();
      call('app.select', {index:frames(grid).indexOf(frame), focus:false});
      return;
    }
    if (frame && frame.dataset.appId !== window.__libroSelectedApp) {
      // Select the panel without replacing the clicked control's focus.
      window.__libroSelectedApp = frame.dataset.appId;
      refresh();
      call('app.select', {index:frames(frame.parentElement).indexOf(frame), focus:false});
    }
  }, true);
  updateToolHints();
  refresh();
  if (window.__libroSelectedApp) select(window.__libroSelectedApp, false);
})();
