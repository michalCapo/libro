(function () {
  const states = new Map();
  function request(s, path, preview = false, external = false) {
    const token = String(++s.sequence);
    s.pending.set(token, {path, preview, external});
    if (preview) s.previewRequest = token;
    s.status.textContent = external ? 'Opening…' : 'Loading…';
    __ws.call(external ? 'files.open' : 'files.read', {sid:window.__libroWorkspaceSID, id:s.id, path, request:token});
  }
  function visible(s) {
    const result = [], query = s.filter.value.toLowerCase();
    function walk(path, depth) {
      for (const item of s.children.get(path) || []) {
        if (!s.hidden.checked && item.name.startsWith('.')) continue;
        if (!query || item.path.toLowerCase().includes(query)) result.push({...item, depth});
        if (item.dir && s.expanded.has(item.path)) walk(item.path, depth + 1);
      }
    }
    walk('',0); return result;
  }
  function render(s) {
    s.items = visible(s);
    s.index = Math.max(0, Math.min(s.index, s.items.length - 1));
    s.tree.replaceChildren();
    s.items.forEach((item, index) => {
      const row = document.createElement('div'); row.className = 'ws-file-row'; row.id = 'file-row-' + s.id + '-' + index;
      row.setAttribute('role','treeitem'); row.setAttribute('aria-level', String(item.depth + 1)); row.setAttribute('aria-selected', String(index === s.index));
      if (item.dir) row.setAttribute('aria-expanded', String(s.expanded.has(item.path)));
      row.style.paddingLeft = (8 + item.depth * 16) + 'px';
      const icon = document.createElement('i'); icon.className = 'material-icons-round'; icon.setAttribute('aria-hidden','true');
      icon.textContent = item.dir ? (s.expanded.has(item.path) ? 'expand_more' : 'chevron_right') : 'description';
      const label = document.createElement('span'); label.textContent = item.name; row.title = item.path; row.append(icon,label);
      row.onclick = () => { s.index = index; open(s); s.tree.focus(); };
      s.tree.append(row);
    });
    if (s.items.length) s.tree.setAttribute('aria-activedescendant','file-row-' + s.id + '-' + s.index);
    else { s.tree.removeAttribute('aria-activedescendant'); s.tree.textContent = s.filter.value ? 'No matching files in loaded folders.' : 'This folder is empty.'; }
  }
  function open(s, expandOnly = false) {
    const item = s.items[s.index]; if (!item) return;
    if (!item.dir) { request(s,item.path,true); return; }
    if (s.expanded.has(item.path)) {
      if (!expandOnly) s.expanded.delete(item.path);
      else s.index = Math.min(s.index + 1, s.items.length - 1);
    } else {
      s.expanded.add(item.path);
      if (!s.children.has(item.path)) request(s,item.path);
    }
    render(s);
  }
  function init() {
    for (const [id,s] of states) if (!s.el.isConnected) { if (s.url) URL.revokeObjectURL(s.url); states.delete(id); }
    document.querySelectorAll('[data-files]').forEach(el => {
      if (states.get(el.dataset.files)?.el === el) return;
      const s = {id:el.dataset.files,el,tree:el.querySelector('.ws-file-tree'),filter:el.querySelector('.ws-file-filter'),hidden:el.querySelector('.ws-file-hidden input'),status:el.querySelector('[role=status]'),children:new Map(),expanded:new Set(),pending:new Map(),index:0,sequence:0,items:[]};
      states.set(s.id,s);
      s.wrap = el.querySelector('.ws-file-wrap input');
      s.wrap.onchange = () => el.querySelector('.ws-file-text').classList.toggle('is-wrapped', s.wrap.checked);
      s.hidden.onchange = () => { s.index = 0; render(s); };
      s.filter.oninput = () => { s.index = 0; render(s); };
      s.filter.onkeydown = event => { if (event.key === 'Escape' || event.key === 'ArrowDown' || event.key === 'Enter') { event.preventDefault(); s.tree.focus(); if (event.key === 'Enter') open(s); } };
      s.tree.onkeydown = event => {
        if (event.ctrlKey || event.altKey || event.metaKey) return;
        const key = event.key, item = s.items[s.index];
        if (!['j','k','h','l','g','G','o','/','Enter','Backspace','ArrowDown','ArrowUp','ArrowLeft','ArrowRight','Home','End'].includes(key)) return;
        event.preventDefault(); event.stopPropagation();
        if (key === '/') { s.filter.focus(); s.filter.select(); return; }
        if (key === 'o') { if (item && !item.dir && !event.repeat) request(s,item.path,false,true); return; }
        if (key === 'g') { if (Date.now() - (s.lastG || 0) < 500) s.index = 0; s.lastG = Date.now(); }
        else if (key === 'G' || key === 'End') s.index = s.items.length - 1;
        else if (key === 'Home') s.index = 0;
        else if (key === 'j' || key === 'ArrowDown') s.index++;
        else if (key === 'k' || key === 'ArrowUp') s.index--;
        else if (key === 'l' || key === 'ArrowRight' || key === 'Enter') { open(s,key !== 'Enter'); return; }
        else if (item) {
          if (key !== 'Backspace' && item.dir && s.expanded.has(item.path)) s.expanded.delete(item.path);
          else { const parent = item.path.split('/').slice(0,-1).join('/'); const index = s.items.findIndex(entry => entry.path === parent); if (index >= 0) s.index = index; }
        }
        render(s); s.tree.querySelector('[aria-selected=true]')?.scrollIntoView({block:'nearest'});
      };
      request(s,'');
    });
  }
  function receive(result) {
    const s = states.get(result.id); if (!s?.el.isConnected) return;
    const pending = s.pending.get(result.request); if (!pending) return;
    s.pending.delete(result.request);
    if (pending.preview && s.previewRequest !== result.request) return;
    s.status.textContent = result.error || '';
    if (result.error || pending.external) return;
    if (result.directory) { s.children.set(pending.path,result.entries || []); render(s); }
    else {
      s.el.querySelector('.ws-file-path').textContent = result.path;
      const text = s.el.querySelector('.ws-file-text'), media = s.el.querySelector('.ws-file-media');
      media.replaceChildren();
      if (s.url) { URL.revokeObjectURL(s.url); s.url = null; }
      text.hidden = !!result.mime;
      media.hidden = !result.mime;
      s.wrap.disabled = !!result.mime;
      if (!result.mime) { text.textContent = result.text || '(Empty file)'; text.scrollTop = 0; return; }
      const bytes = Uint8Array.from(atob(result.data || ''), c => c.charCodeAt(0));
      s.url = URL.createObjectURL(new Blob([bytes], {type:result.mime}));
      const kind = result.mime.split('/')[0];
      const element = document.createElement(kind === 'image' ? 'img' : kind === 'audio' || kind === 'video' ? kind : 'iframe');
      if (kind === 'image') element.alt = result.path;
      else if (kind === 'audio' || kind === 'video') { element.controls = true; element.preload = 'metadata'; }
      else element.title = 'Preview of ' + result.path;
      element.onerror = () => { s.status.textContent = 'This browser cannot preview this file. Press o in the file tree to open externally.'; };
      element.src = s.url;
      media.append(element);
    }
  }
  window.libroFiles = {init,receive};
})();
