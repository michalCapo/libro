(function () {
  if (window.libroWorkspace) return;
  const root = document.getElementById('libro-workspace');
  const sid = window.__libroWorkspaceSID;
  let maximized = '';
  const dockStates = new Map();
  function dockState(grid) {
    const key = grid.dataset.workspaceProject;
    if (!dockStates.has(key)) dockStates.set(key, {right:'', hidden:new Set(), bottom:false});
    return dockStates.get(key);
  }
  let prefs = {};
  try { prefs = JSON.parse(localStorage.getItem('libro.workspace') || '{}') || {}; } catch (_) {}
  if (typeof prefs.projects !== 'boolean') prefs.projects = innerWidth > 760;
  const call = (action, data = {}) => __ws.call(action, {sid, ...data});
  const node = (tag, cls, text) => { const el = document.createElement(tag); el.className = cls; if (text) el.textContent = text; return el; };
  const button = (label, icon, action) => {
    const el = node('button', 'ws-button'); el.type = 'button'; el.title = label; el.setAttribute('aria-label', label);
    const glyph = node('i', 'material-icons-round', icon); glyph.setAttribute('aria-hidden', 'true'); el.append(glyph); el.onclick = action; return el;
  };
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
  function openPlugin(id, dock, trigger) {
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
    const grid = frame.parentElement;
    const state = dockState(grid);
    state.hidden.delete(id);
    if (frame.dataset.dock === 'right') state.right = id;
    if (frame.dataset.dock === 'bottom') state.bottom = true;
    window.__libroSelectedApp = id;
    if (maximized && maximized !== id) maximized = '';
    refresh();
    window.__libroScrollToApp(frame);
    if (send) call('app.select', {index:frames(grid).indexOf(frame)});
    requestAnimationFrame(() => {
      const terminal = frame.querySelector('[data-terminal-app]');
      if (terminal && window.__libroFitTerminalFrame) window.__libroFitTerminalFrame(terminal);
      if (window.__libroFocusAppByID) window.__libroFocusAppByID(id);
      frame.querySelector('.ws-file-tree')?.focus();
    });
  }
  function launcher(dock) {
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
      const entry = node('button', 'ws-plugin-entry'); entry.type = 'button'; const icon = node('i', 'material-icons-round', plugin.type === 'url' ? 'language' : 'terminal'); icon.setAttribute('aria-hidden', 'true'); const copy = node('span', 'ws-plugin-copy'); copy.append(node('span', '', plugin.name), node('small', '', plugin.description || plugin.command || 'Browser app')); entry.append(icon, copy);
      entry.onclick = () => { dialog.close(); openPlugin(plugin.id, dock, entry); }; entries.append(entry);
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
    const heading = node(center ? 'h1' : 'h2', '', center ? 'What are we building on ' : zone === 'right' ? 'Tools for your workspace' : 'Your project terminal');
    if (center) heading.append(node('span', 'ws-project-accent', grid.dataset.projectLabel), document.createTextNode('?'));
    el.append(heading);
    el.append(node('p', '', center ? 'Start an agent in this project. Your tools and terminals stay close by.' : zone === 'right' ? 'Open a browser, repository tool, or terminal.' : 'Run commands without leaving your agent.'));
    const actions = node('div', 'ws-agent-actions');

    (center ? ['codex', 'pi', 'claude'] : zone === 'right' ? ['browser', 'lazyrepo', 'terminal'] : ['terminal']).forEach(id => {
      const plugin = window.__libroPlugins.find(p => p.id === id);
      if (!plugin || plugin.disabled || plugin.removed) return;
      const launch = node('button', 'ws-launch', plugin.name);
      launch.type = 'button';
      launch.onclick = () => openPlugin(id, zone, launch);
      actions.append(launch);
    });
    const more = node('button', 'ws-launch', center ? 'Other agent…' : 'More…'); more.type = 'button'; more.onclick = () => launcher(zone); actions.append(more); el.append(actions); return el;
  }
  function renderProjects() {
    const list = document.getElementById('workspace-project-list'); if (!list) return;
    const projects = window.__libroProjects || []; const signature = JSON.stringify(projects);
    if (list.dataset.signature === signature) return; list.dataset.signature = signature; list.replaceChildren();
    projects.forEach(project => {
      const item = node('div', 'ws-project-item'); item.dataset.kind = project.kind;
      const row = node('button', 'ws-project-row'); row.type = 'button'; row.dataset.kind = project.kind;
      row.setAttribute('aria-current', String(!!project.isActive));
      const projectName = project.displayName || project.name;
      row.setAttribute('aria-label', projectName);
      row.title = projectName + (project.path ? '\n' + project.path : '');
      const icon = node('i', 'material-icons-round', project.kind === 'worktree' ? 'account_tree' : 'folder_open'); icon.setAttribute('aria-hidden', 'true');
      row.append(icon, node('span', '', project.displayName || project.name));
      row.onclick = () => { closeSettings(); if (innerWidth <= 760) { prefs.projects = false; save(); } if (project.kind === 'worktree') call('worktree.switch', {project:project.name, path:project.path, branch:project.branch}); else call('project.switch', {name:project.name}); };
      item.append(row);
      if (project.kind !== 'worktree' && project.name !== 'home') {
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
  let queued = false;
  const observer = new MutationObserver(records => {
    if (records.some(record => (record.type === 'attributes' && record.target.matches('[data-app-id]')) || [...record.addedNodes, ...record.removedNodes].some(n => n.nodeType === 1 && (n.matches('[data-app-id], [data-workspace-project], .ws-project, #top-bar') || n.querySelector('[data-app-id], [data-workspace-project]'))))) schedule();
  });
  function schedule() { if (!queued) { queued = true; requestAnimationFrame(() => { queued = false; refresh(); }); } }
  function refresh() {
    observer.disconnect();
    root.dataset.projects = String(prefs.projects);
    document.querySelectorAll('.ws-sidebar-action, .ws-sidebar-search').forEach(button => {
      const label = button.getAttribute('aria-label') || button.textContent.trim().replace(/^(settings|apps|tune)\s*/, '');
      button.setAttribute('aria-label', label); button.title = label;
    });
    renderToolRail();
    renderProjects();
    window.libroFiles?.init();
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
    observer.observe(document.getElementById('main-area'), {childList:true, subtree:true, attributes:true, attributeFilter:['style', 'data-dock']});
  }
  function renderToolRail() {
    const rail = document.getElementById('workspace-tool-buttons'); if (!rail) return;
    const list = window.__libroPlugins.filter(p => p.dock === 'right' && !p.disabled && !p.removed);
    const signature = JSON.stringify(list);
    if (rail.dataset.signature === signature) return;
    rail.dataset.signature = signature; rail.replaceChildren();
    const icons = {terminal:'terminal',browser:'language',files:'folder_open',nvim:'edit',lazyrepo:'account_tree',lazydata:'storage'};
    list.forEach(p => { const entry = button(p.name, icons[p.id] || 'terminal', () => tool(p.id)); entry.dataset.toolId = p.id; rail.append(entry); });
    updateToolHints();
  }
  function tool(id) {
    const grid = activeGrid(); if (!grid) return;
    const state = dockState(grid);
    const existing = frames(grid).find(frame => frame.dataset.dock === 'right' && frame.dataset.plugin === id);
    if (!existing) { openPlugin(id, 'right'); return; }
    if (existing.dataset.dockVisible === 'true') {
      state.hidden.add(existing.dataset.appId); refresh();
    } else select(existing.dataset.appId);
  }
  let toolKeys = window.__libroToolKeys || {};
  function shortcut(event) {
    const key = event.key === '+' ? '=' : event.key;
    if (!/^[a-z0-9=\-]$/i.test(key) || !(event.ctrlKey || event.altKey || event.metaKey)) return '';
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
    document.querySelectorAll('.ws-tool-rail button').forEach(button => {
      const name = button.getAttribute('aria-label');
      const plugin = window.__libroPlugins.find(plugin => plugin.id === button.dataset.toolId);
      if (plugin) button.title = name + (toolKeys[plugin.id] ? ' (' + toolKeys[plugin.id] + ')' : '');
    });
  }
  function saveToolKeys() {
    const bindings = {};
    document.querySelectorAll('[data-tool-key]').forEach(input => bindings[input.dataset.toolKey] = input.value);
    document.querySelector('#tool-key-form [type=submit]').disabled = true;
    document.getElementById('tool-key-status').textContent = 'Saving…';
    call('settings.tool-keys', {bindings});
  }
  function resetToolKeys() { fillToolKeys(window.__libroDefaultToolKeys); }
  function toolKeysSaved(bindings, message) {
    document.querySelector('#tool-key-form [type=submit]').disabled = false;
    document.getElementById('tool-key-status').textContent = message;
    if (bindings) { toolKeys = bindings; updateToolHints(); }
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
    if (!document.getElementById('workspace-settings').hidden || document.querySelector('dialog[open]')) return;
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
    dockState(frame.parentElement).bottom = false;
    call('app.close', {id});
    refresh();
  }
  function bottom() {
    const grid = activeGrid(); if (!grid) return;
    const state = dockState(grid);
    const terminal = frames(grid).find(frame => frame.dataset.dock === 'bottom');
    if (!terminal) { openPlugin('terminal', 'bottom'); return; }
    state.bottom = !state.bottom;
    if (state.bottom) select(terminal.dataset.appId); else refresh();
  }
  function layoutDocks(grid, all, full) {
    const state = dockState(grid);
    const selected = all.find(frame => frame.dataset.appId === window.__libroSelectedApp);
    // Server-created panels must become visible before terminal hydration.
    all.forEach(frame => {
      if (!frame.dataset.dockSeen) {
        frame.dataset.dockSeen = 'true';
        if (frame.dataset.dock === 'bottom') state.bottom = true;
        if (frame.dataset.dock === 'right') state.right = frame.dataset.appId;
      }
    });
    if (selected && grid.dataset.lastSelected !== selected.dataset.appId) {
        state.hidden.delete(selected.dataset.appId);
        if (selected.dataset.dock === 'right') state.right = selected.dataset.appId;
        if (selected.dataset.dock === 'bottom') state.bottom = true;
    }
    if (selected?.dataset.dock === 'center') state.agent = selected.dataset.appId;
    grid.dataset.lastSelected = selected?.dataset.appId || '';
    const width = frame => frame.style.width.endsWith('%') ? grid.clientWidth : parseFloat(frame.style.width) || frame.offsetWidth;
    const center = all.filter(frame => frame.dataset.dock === 'center');
    const tools = all.filter(frame => frame.dataset.dock === 'right' && !state.hidden.has(frame.dataset.appId));
    const right = [tools.find(frame => frame.dataset.appId === state.right) || tools.at(-1)].filter(Boolean);
    const terminal = all.find(frame => frame.dataset.dock === 'bottom');
    const bottomVisible = terminal && state.bottom && !full;
    const overlay = !full && right.length > 0 && center.reduce((sum, frame) => sum + width(frame), 0) + width(right[0]) > grid.clientWidth;
    const visible = full ? [full] : [...center, ...right];
    const columns = (overlay ? center : visible).map(frame => full ? grid.clientWidth + 'px' : width(frame) + 'px');
    if (center.length === 1 && !full) columns[0] = 'minmax(' + width(center[0]) + 'px, 1fr)';
    if (!center.length && !full) columns.unshift(right.length && !overlay ? '320px' : 'minmax(0, 1fr)');
    grid.style.gridTemplateColumns = columns.join(' ') || 'minmax(0, 1fr)';
    grid.style.gridTemplateRows = bottomVisible ? 'minmax(120px, 1fr) minmax(120px, 35%)' : 'minmax(0, 1fr)';
    all.forEach(frame => {
      const index = visible.indexOf(frame);
      const show = index >= 0 || (frame === terminal && bottomVisible);
      frame.dataset.dockVisible = String(!!show);
      frame.inert = !show;
      const isOverlay = overlay && frame === right[0];
      frame.dataset.toolOverlay = String(isOverlay);
      frame.style.left = isOverlay ? Math.max(0, grid.clientWidth - width(frame)) + grid.scrollLeft + 'px' : '';
      frame.style.justifySelf = center.length === 1 && frame === center[0] && !full ? 'center' : 'start';
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
    const signature = [...center, ...all.filter(frame => frame.dataset.dock === 'right')].map(frame => [frame.dataset.appId, frame.dataset.appName, frame.dataset.dock === 'center' ? frame.dataset.selected : frame.dataset.dockVisible, frame.querySelector('[data-size-trigger]')?.textContent || 'MD', frame.dataset.dock]);
    if (tabs.dataset.signature !== JSON.stringify(signature)) {
      tabs.dataset.signature = JSON.stringify(signature); tabs.replaceChildren();
      signature.forEach(([id, name, shown, size, dock]) => {
        const group = node('div', 'ws-agent-tab ws-tool-tab-group'); group.dataset.selected = shown; group.dataset.tabDock = dock;
        const tab = node('button', 'ws-tool-tab', name); tab.type = 'button'; tab.setAttribute('aria-pressed', shown);
        tab.onclick = () => {
          if (dock === 'center' && grid.querySelector('[data-tool-overlay=true][data-dock-visible=true]')) {
            all.filter(frame => frame.dataset.dock === 'right').forEach(frame => state.hidden.add(frame.dataset.appId));
          }
          if (dock === 'right' && shown === 'true') { state.hidden.add(id); refresh(); }
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
        sizes.append(trigger, picker); group.append(tab, sizes); tabs.append(group);
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
  let savedWidth = 'md';
  let settingsFocus;
  function settings() { settingsFocus = document.activeElement; call('settings.open'); }
  function showSettings(width, commands = {}, bindings = toolKeys) {
    toolKeys = bindings; fillToolKeys(bindings); updateToolHints();
    document.getElementById('tool-key-status').textContent = '';
    document.getElementById('agent-command-rows').replaceChildren();
    document.getElementById('tool-command-rows').replaceChildren();
    window.__libroPlugins.filter(p => p.dock === 'right' && p.type === 'terminal' && p.id !== 'terminal' && !p.removed).forEach(p => addAgentRow(p, p.command, true));
    removedAgents = Object.fromEntries(window.__libroPlugins.filter(p => p.removed).map(p => [p.id, true]));
    window.__libroPlugins.filter(p => !p.removed && p.dock === 'center' && p.type === 'terminal').forEach(p => addAgentRow(p, commands[p.id] || p.command));
    document.querySelector('#agent-commands-form [role=status]').textContent = '';
    savedWidth = width;
    document.getElementById('default-panel-width').value = width;
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
  function saveSettings(width) {
    document.getElementById('default-panel-width').disabled = true;
    document.getElementById('workspace-settings-status').textContent = 'Saving…';
    call('settings.width', {width});
  }
  let removedAgents = {};
  function addAgentRow(plugin, command, toolRow = false) {
    const row = node('div', 'ws-settings-row ws-agent-command-row ws-agent-config-row');
    row.dataset.agentId = plugin.id; row.dataset.custom = String(!!plugin.custom);
    const name = node('input', 'ws-agent-command');
    name.value = plugin.name; name.placeholder = 'Agent name'; name.required = true; name.setAttribute('aria-label', toolRow ? 'Tool name' : 'Agent name'); name.dataset.agentName = '';
    const input = node('input', 'ws-agent-command'); input.id = 'agent-command-' + plugin.id; input.dataset.agentCommand = plugin.id;
    input.value = command || ''; input.placeholder = 'CLI command'; input.required = true; input.spellcheck = false; input.setAttribute('aria-label', plugin.name ? plugin.name + ' command' : 'Custom agent command');
    const toggle = node('label', 'ws-agent-enabled');
    const checkbox = node('input', ''); checkbox.type = 'checkbox'; checkbox.checked = !plugin.disabled; checkbox.dataset.agentEnabled = '';
    checkbox.setAttribute('aria-label', 'Enable ' + (plugin.name || 'custom agent'));
    toggle.append(checkbox);
    const remove = button(toolRow ? 'Remove tool' : 'Remove agent', 'delete_outline', () => {
      if (checkbox.checked) return;
      if (toolRow) { if (!window.__libroPlugins.some(p => p.id === plugin.id)) { row.remove(); return; } row.dataset.removed = 'true'; row.hidden = true; row.querySelectorAll('input').forEach(input => input.required = false); return; }
      if (window.__libroPlugins.some(p => p.id === plugin.id)) removedAgents[plugin.id] = true;
      row.remove();
    });
    remove.hidden = checkbox.checked;
    checkbox.onchange = () => remove.hidden = checkbox.checked;
    row.append(toggle, name, input, remove); document.getElementById(toolRow ? 'tool-command-rows' : 'agent-command-rows').append(row);
    return row;
  }
  function addCustomTool() {
    addAgentRow({id:'custom-tool-' + crypto.randomUUID(), name:'', custom:true}, '', true).querySelector('[data-agent-name]').focus();
  }
  function saveTools(form) {
    const tools = window.__libroPlugins.filter(p => p.dock === 'right' && p.removed);
    form.querySelectorAll('[data-agent-id]').forEach(row => tools.push({id:row.dataset.agentId, name:row.querySelector('[data-agent-name]').value.trim(), command:row.querySelector('[data-agent-command]').value, type:'terminal', dock:'right', custom:row.dataset.custom === 'true', disabled:!row.querySelector('[data-agent-enabled]').checked, removed:row.dataset.removed === 'true'}));
    form.querySelector('[type=submit]').disabled = true;
    form.querySelector('[role=status]').textContent = 'Saving…';
    call('settings.tools', {tools});
  }
  function toolsSaved(plugins, message) {
    const form = document.getElementById('tool-commands-form');
    form.querySelector('[type=submit]').disabled = false; form.querySelector('[role=status]').textContent = message;
    if (plugins) { window.__libroPlugins = plugins; refresh(); }
  }
  function addCustomAgent() {
    addAgentRow({id:'custom-' + crypto.randomUUID(), name:'', custom:true}, '').querySelector('input').focus();
  }
  function saveAgentCommand(form) {
    const commands = {}, disabled = {}, custom = [], names = {};
    form.querySelectorAll('[data-agent-id]').forEach(row => {
      const id = row.dataset.agentId, command = row.querySelector('[data-agent-command]').value;
      names[id] = row.querySelector('[data-agent-name]').value.trim();
      commands[id] = command; disabled[id] = !row.querySelector('[data-agent-enabled]').checked;
      if (row.dataset.custom === 'true') custom.push({id, name:row.querySelector('[data-agent-name]').value.trim(), command, type:'terminal', dock:'center', custom:true});
    });
    form.querySelector('[type=submit]').disabled = true;
    form.querySelector('[role=status]').textContent = 'Saving…';
    Object.keys(removedAgents).forEach(id => disabled[id] = true);
    call('settings.agent-command', {commands, disabled, custom, names, removed:removedAgents});
  }
  function agentCommandSaved(message, plugins) {
    const form = document.getElementById('agent-commands-form');
    form.querySelector('[type=submit]').disabled = false;
    form.querySelector('[role=status]').textContent = message;
    if (plugins) {
      window.__libroPlugins = plugins;
      document.querySelectorAll('.ws-grid > .ws-empty').forEach(el => el.remove());
      refresh();
    }
  }
  function settingsSaved(ok) {
    const select = document.getElementById('default-panel-width');
    if (ok) savedWidth = select.value; else select.value = savedWidth;
    select.disabled = false;
    document.getElementById('workspace-settings-status').textContent = ok ? 'Saved. New panels will use this width.' : 'Could not save. Please try again.';
  }
  window.libroWorkspace = {saveTools, toolsSaved, addCustomTool, zoom, shortcutFor:id => toolKeys[id] || '', select, refresh, launcher, toggle, maximize, navigate, settings, showSettings, closeSettings, saveSettings, settingsSaved, saveToolKeys, resetToolKeys, toolKeysSaved, saveAgentCommand, agentCommandSaved, addCustomAgent, tool, bottom, terminalExited};
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
