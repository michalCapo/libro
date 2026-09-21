(function () {
  const states = new Map();
  const highlightLanguages = {
    bash:'bash', sh:'bash', zsh:'bash', c:'c', h:'c', cc:'cpp', cpp:'cpp', cxx:'cpp', hpp:'cpp',
    cs:'csharp', css:'css', diff:'diff', patch:'diff', go:'go', gql:'graphql', graphql:'graphql',
    ini:'ini', conf:'ini', properties:'ini', java:'java', js:'javascript', jsx:'javascript',
    cjs:'javascript', mjs:'javascript', json:'json', jsonc:'json', jsonl:'json', ndjson:'json',
    kt:'kotlin', kts:'kotlin', less:'less', lua:'lua', md:'markdown', markdown:'markdown', mdx:'markdown',
    m:'objectivec', mm:'objectivec', pl:'perl', php:'php', py:'python', pyi:'python', r:'r', rb:'ruby',
    rs:'rust', scss:'scss', sql:'sql', swift:'swift', ts:'typescript', tsx:'typescript', mts:'typescript',
    cts:'typescript', vb:'vbnet', wasm:'wasm', html:'xml', htm:'xml', svg:'xml', xml:'xml', vue:'xml',
    svelte:'xml', yaml:'yaml', yml:'yaml'
  };
  const highlightNames = {makefile:'makefile', gnumakefile:'makefile', gemfile:'ruby', rakefile:'ruby'};
  const highlightMaxCharacters = 256 * 1024, highlightMaxLines = 5000, highlightMaxLineLength = 1000;
  let highlighterPromise;
  function loadHighlighter() {
    if (window.hljs) return Promise.resolve(window.hljs);
    if (highlighterPromise) return highlighterPromise;
    highlighterPromise = new Promise((resolve,reject) => {
      const script = document.createElement('script');
      script.src = '/assets/highlight/highlight.min.js';
      script.onload = () => window.hljs ? resolve(window.hljs) : reject(new Error('Highlight.js did not initialize'));
      script.onerror = () => { script.remove(); reject(new Error('Highlight.js failed to load')); };
      document.head.append(script);
    }).catch(error => { highlighterPromise = null; throw error; });
    return highlighterPromise;
  }
  function highlightLanguage(path) {
    const name = path.split('/').pop().toLowerCase();
    if (highlightNames[name]) return highlightNames[name];
    const dot = name.lastIndexOf('.');
    return dot < 0 ? null : highlightLanguages[name.slice(dot + 1)] || null;
  }
  function highlightable(source) {
    if (source.length > highlightMaxCharacters) return false;
    let lines = 1, lineLength = 0;
    for (let i = 0; i < source.length; i++) {
      if (source.charCodeAt(i) === 10) { lines++; lineLength = 0; if (lines > highlightMaxLines) return false; }
      else if (++lineLength > highlightMaxLineLength) return false;
    }
    return true;
  }
  function showText(s, path, source) {
    const text = s.el.querySelector('.ws-file-text'), version = ++s.highlightVersion;
    text.classList.remove('has-syntax');
    text.removeAttribute('data-language');
    text.textContent = source || '(Empty file)';
    text.scrollTop = 0;
    const language = source && highlightLanguage(path);
    if (!language || !highlightable(source)) return;
    loadHighlighter().then(highlighter => {
      if (s.highlightVersion !== version || !text.isConnected || text.hidden) return;
      text.innerHTML = highlighter.highlight(source, {language, ignoreIllegals:true}).value;
      text.dataset.language = language;
      text.classList.add('has-syntax');
    }).catch(() => {});
  }
  function request(s, path, preview = false, external = false, parents = s.parents) {
    const token = String(++s.sequence);
    s.pending.set(token, {path, preview, external, parents});
    if (preview) s.previewRequest = token;
    s.status.textContent = external ? 'Opening…' : 'Loading…';
    __ws.call(external ? 'files.open' : 'files.read', {sid:window.__libroWorkspaceSID, id:s.id, path, parents, request:token});
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
      const s = {id:el.dataset.files,el,tree:el.querySelector('.ws-file-tree'),filter:el.querySelector('.ws-file-filter'),hidden:el.querySelector('.ws-file-hidden input'),status:el.querySelector('[role=status]'),children:new Map(),expanded:new Set(),pending:new Map(),index:0,sequence:0,parents:0,items:[],highlightVersion:0};
      states.set(s.id,s);
      s.preview = el.querySelector('.ws-file-render input');
      const preferenceKey = 'libro.files.preview:' + el.closest('[data-workspace-project]').dataset.workspaceProject;
      try { s.preview.checked = localStorage.getItem(preferenceKey) === 'true'; } catch (_) {}
      s.preview.onchange = () => {
        try { localStorage.setItem(preferenceKey, String(s.preview.checked)); } catch (_) {}
        if (s.file) showFile(s);
      };
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
        if (key === 'Backspace') {
          if (!event.repeat && !s.parentRequest && s.parents < 1024) {
            s.parentRequest = true;
            request(s,'',false,false,s.parents + 1);
          }
          return;
        }
        if (key === '/') { s.filter.focus(); s.filter.select(); return; }
        if (key === 'o') { if (item && !item.dir && !event.repeat) request(s,item.path,false,true); return; }
        if (key === 'g') { if (Date.now() - (s.lastG || 0) < 500) s.index = 0; s.lastG = Date.now(); }
        else if (key === 'G' || key === 'End') s.index = s.items.length - 1;
        else if (key === 'Home') s.index = 0;
        else if (key === 'j' || key === 'ArrowDown') s.index++;
        else if (key === 'k' || key === 'ArrowUp') s.index--;
        else if (key === 'l' || key === 'ArrowRight' || key === 'Enter') { open(s,key !== 'Enter'); return; }
        else if (item) {
          if (item.dir && s.expanded.has(item.path)) s.expanded.delete(item.path);
          else { const parent = item.path.split('/').slice(0,-1).join('/'); const index = s.items.findIndex(entry => entry.path === parent); if (index >= 0) s.index = index; }
        }
        render(s); s.tree.querySelector('[aria-selected=true]')?.scrollIntoView({block:'nearest'});
      };
      request(s,'');
    });
  }
  function showFile(s) {
    const result = s.file;
    s.el.querySelector('.ws-file-path').textContent = result.path;
    const text = s.el.querySelector('.ws-file-text'), media = s.el.querySelector('.ws-file-media');
    media.replaceChildren();
    if (s.url) { URL.revokeObjectURL(s.url); s.url = null; }
    text.hidden = !!result.mime;
    media.hidden = !result.mime;
    s.wrap.disabled = !!result.mime;
    if (!result.mime && s.preview.checked && /\.(html?|md|markdown)$/i.test(result.path)) {
      s.highlightVersion++;
      text.hidden = true;
      media.hidden = false;
      s.wrap.disabled = true;
      const frame = document.createElement('iframe');
      frame.title = 'Preview of ' + result.path;
      frame.setAttribute('sandbox', 'allow-scripts');
      frame.srcdoc = result.html || '';
      media.append(frame);
      return;
    }
    if (!result.mime) { showText(s, result.path, result.text || ''); return; }
    s.highlightVersion++;
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
  function receive(result) {
    const s = states.get(result.id); if (!s?.el.isConnected) return;
    const pending = s.pending.get(result.request); if (!pending) return;
    s.pending.delete(result.request);
    if (pending.preview && s.previewRequest !== result.request) return;
    if (pending.parents !== s.parents) s.parentRequest = false;
    s.status.textContent = result.error || '';
    if (result.error || pending.external) return;
    if (result.directory && pending.parents !== s.parents) {
      s.file = null;
      s.parents = pending.parents;
      s.parentRequest = false;
      s.pending.clear();
      s.children.clear();
      s.expanded.clear();
      s.filter.value = '';
      s.index = 0;
      s.el.querySelector('.ws-file-path').textContent = 'Open file';
      showText(s, '', 'Select a file from the tree.');
      s.el.querySelector('.ws-file-text').hidden = false;
      s.el.querySelector('.ws-file-media').replaceChildren();
      s.el.querySelector('.ws-file-media').hidden = true;
      s.wrap.disabled = false;
      if (s.url) { URL.revokeObjectURL(s.url); s.url = null; }
    }
    if (result.directory) { s.children.set(pending.path,result.entries || []); render(s); }
    else {
      s.file = result;
      showFile(s);
    }
  }
  window.libroFiles = {init,receive};
})();
