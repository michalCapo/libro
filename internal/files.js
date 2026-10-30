(function () {
  const states = new Map();
  // Tokens are unique across mounts, so a reply to a removed panel never matches its replacement.
  let sequence = 0;
  const shortcutHelp = [
    ['j / k', 'Tree and lists: move down / up'],
    ['h / l', 'Tree: fold / open folder'],
    ['Backspace', 'Tree: parent folder'],
    ['gg / G', 'Tree, lists and previews: first / last'],
    ['/', 'Tree: filter files'],
    ['Enter', 'Preview file'],
    ['o', 'Tree: open externally'],
    ['a / r / d', 'Tree: add / rename / delete (end a new name with / for a folder)'],
    ['Space Space', 'Find project files in tree'],
    ['Space f / Esc', 'Back to file tree (Esc in view mode)'],
    ['i / a / o / I / A / O', 'Source: start editing in insert mode'],
    ['Esc', 'Insert: back to normal mode'],
    ['u / Ctrl+R', 'Source: undo / redo'],
    [':w / Ctrl+S / :w!', 'Save / overwrite changes made on disk'],
    [':q / :q!', 'Back to view mode / and discard changes'],
    [':wq / :x', 'Save and back to view mode'],
    [':e!', 'Reload file from disk'],
    ['h j k l', 'Rendered preview and image: scroll'],
    ['+ / - / 0', 'Image: zoom in / out / reset'],
    ['h j k l / w b', 'Source movement'],
    ['00 / 44 / 5', 'Source: line start / end / matching bracket'],
    ['Ctrl+d / Ctrl+u', 'Source: 15 lines; previews and lists: half page'],
    ['/ / n / N', 'Find in file / next / previous'],
    ['v / V / y / yy', 'Select / select lines / copy'],
    ['Space yy', 'Copy file:line'],
    ['gd / gr', 'Definition / references'],
    ['gD / gi / gt', 'Declaration / implementation / type definition'],
    ['go / [f / ]f', 'Symbols / previous / next function'],
    ['Space ss / Space sw', 'Project search / word or selection'],
    ['Ctrl+o / Ctrl+i', 'Jump back / forward (also Tab in source)'],
    ['Space ,', 'Recent files'],
    ['Space sk', 'Search shortcuts'],
    ['Space lr', 'Restart language server'],
    ['Space p', 'Toggle preview'],
    ['Space w', 'Toggle wrapping'],
  ].map(([keys,action])=>({keys,action,label:keys+' '+action}));
  function remember(s) {
    if (s.file && s.editor) s.positions.set(s.parents + ':' + s.file.path, s.editor.position());
  }
  function focusPreview(s) {
    const text=s.el.querySelector('.ws-file-text'),media=s.el.querySelector('.ws-file-media'),frame=media.querySelector('.ws-file-rendered');
    if (s.editor && !text.hidden) s.editor.focus();
    // Focus inside the rendered preview so its keys reach the workspace.
    else if (frame) frame.focus();
    else { const target=text.hidden?media:text;target.tabIndex=0;target.focus(); }
  }
  async function copy(s, text) {
    try { await navigator.clipboard.writeText(text); s.status.textContent='Copied to clipboard'; }
    catch (_) { s.status.textContent='Clipboard unavailable. Select text and use your browser’s Copy command.'; }
  }
  function context(s) {
    if (!s.editor) return;
    const position=s.editor.position(), symbol=s.editor.symbols().filter(item=>item.from<=s.editor.view.state.selection.main.head && item.to>=s.editor.view.state.selection.main.head).pop();
    const mode=s.editor.mode(),badge=document.createElement('span');
    badge.className='ws-file-mode';badge.dataset.mode=mode;badge.textContent=mode.toUpperCase();
    const notes=[s.editor.dirty()&&'Modified',saving(s)&&'Saving…',s.diskChanged,s.notice].filter(Boolean);
    s.el.querySelector('.ws-file-context').replaceChildren(badge,` ${position.line}:${position.column+1}` + (symbol ? ' · '+symbol.label : '') + notes.map(note=>' · '+note).join(''));
    s.edit.checked=mode!=='view';
  }
  // Unsaved changes are kept until the user agrees to drop them.
  function guard(s,run) {
    if(!s.editor?.dirty()){run();return;}
    window.__libroConfirmAction('Discard changes?','“'+s.file.path+'” has unsaved changes. Discard them?',()=>{remember(s);showText(s,s.file.path,s.file.text);run();});
  }
  function startEditing(s) {
    if(!s.editor){
      // Rendered previews have no editor; edit the source instead.
      if(!s.file || s.file.mime)return;
      s.preview.checked=false;s.preview.onchange();
    }
    s.editor?.setEditable(true);
  }
  // A save belongs to its editor; a dropped or replaced one no longer blocks saving.
  const saving=s=>[...s.pending.values()].some(item=>item.kind==='save' && item.editor===s.editor);
  function save(s,force=false,quit=false) {
    if(!s.editor?.editable())return;
    if(saving(s)){s.notice='Already saving';context(s);return;}
    const doc=s.editor.doc(),token=String(++sequence);
    s.notice='';
    s.pending.set(token,{kind:'save',editor:s.editor,doc,quit});
    context(s);
    // The editor splits lines on any break; keep a file's Windows line breaks.
    const text=/\r\n/.test(s.file.text || '') ? doc.toString().replace(/\n/g,'\r\n') : doc.toString();
    __ws.call('files.save',{sid:window.__libroWorkspaceSID,id:s.id,request:token,path:s.file.path,parents:s.parents,text,version:force?'':s.file.version});
  }
  function command(s,name,force) {
    const dirty=s.editor.dirty();
    s.notice='';
    if(name==='write' || name==='wq' || name==='xit' && dirty)save(s,force,name!=='write');
    else if(name==='xit' || name==='quit' && (force || !dirty))(force?s.editor.revert:s.editor.setEditable)(false);
    else if(name==='edit' && (force || !dirty)){const token=request(s,s.file.path,true,false,s.parents);Object.assign(s.pending.get(token),{edit:s.editor.editable(),focus:true});}
    else s.notice='No write since last change (add ! to override)';
    context(s);
  }
  // Tree changes ask for a path in a row above the tree.
  function ask(s,label,value,submit) {
    s.ask?.remove();
    const form=document.createElement('form');form.className='ws-file-ask';
    const input=document.createElement('input');input.className='ws-file-filter';input.value=value;input.placeholder=label;input.setAttribute('aria-label',label);
    form.append(input);s.filter.after(form);s.ask=form;
    // Removing the focused input fires blur, which would close the form twice.
    const close=()=>{input.onblur=null;form.remove();if(s.ask===form)s.ask=null;};
    form.onsubmit=event=>{event.preventDefault();const path=input.value.trim();close();s.tree.focus();if(path)submit(path);};
    input.onkeydown=event=>{if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close();s.tree.focus();}};
    input.onblur=close;
    input.focus();
    // Select the name, so typing replaces it and keeps the folder.
    const name=value.replace(/\/$/,'').lastIndexOf('/')+1;input.setSelectionRange(value.endsWith('/')?value.length:name,value.length);
  }
  function change(s,action,args,done) {
    const token=String(++sequence);
    s.pending.set(token,{kind:'change',done});
    __ws.call(action,{sid:window.__libroWorkspaceSID,id:s.id,request:token,parents:s.filter.value?0:s.parents,...args});
  }
  // Show a changed path: open its folders, select it and reload the tree.
  function reveal(s,path,parents) {
    if(parents!==s.parents){s.indexed=null;if(s.filter.value)indexFiles(s);return;}
    const parts=path.split('/');
    for(let i=1;i<parts.length;i++){const dir=parts.slice(0,i).join('/');if(!s.expanded.has(dir)){s.expanded.add(dir);if(!s.children.has(dir))request(s,dir);}}
    s.select=path;s.indexed=null;
    if(s.filter.value)indexFiles(s);
    refresh(s);
  }
  // A path is affected when it is the open file or a folder above it.
  const affects=(s,path,parents)=>s.file && parents===s.parents && (s.file.path===path || s.file.path.startsWith(path+'/'));
  function addEntry(s,item) {
    const parents=s.filter.value?0:s.parents;
    const folder=!item ? '' : item.dir ? item.path : item.path.split('/').slice(0,-1).join('/');
    ask(s,'New file, or folder ending with /',folder?folder+'/':'',value=>{
      const directory=value.endsWith('/'),path=value.replace(/\/+$/,'');
      if(!path)return;
      change(s,'files.create',{path,directory},()=>{
        reveal(s,path,parents);
        if(directory){s.expanded.add(path);request(s,path);}
        else navigate(s,path,null,parents,true,true,true);
      });
    });
  }
  function renameEntry(s,item) {
    const parents=s.filter.value?0:s.parents;
    ask(s,'Rename '+item.path,item.path,target=>{
      target=target.replace(/\/+$/,'');
      if(!target || target===item.path)return;
      const run=()=>change(s,'files.rename',{path:item.path,target},()=>{
        reveal(s,target,parents);
        if(!affects(s,item.path,parents))return;
        // Reopen the file at its new path and place.
        const path=target+s.file.path.slice(item.path.length),position=s.editor?.position(),edit=s.editor?.editable();
        navigate(s,path,position,parents,false,false,edit);
      });
      if(affects(s,item.path,parents))guard(s,run);else run();
    });
  }
  function deleteEntry(s,item) {
    const parents=s.filter.value?0:s.parents;
    window.__libroConfirmAction(item.dir?'Delete folder?':'Delete file?','Delete “'+item.path+'”'+(item.dir?' and everything in it':'')+'? This cannot be undone.',()=>{
      s.tree.focus();
      change(s,'files.delete',{path:item.path},()=>{
        if(affects(s,item.path,parents)){clearFile(s);markOpen(s);}
        s.indexed=null;if(s.filter.value)indexFiles(s);
        refresh(s);
      });
    });
  }
  function showText(s, path, source) {
    const text=s.el.querySelector('.ws-file-text');
    s.editor?.destroy(); s.editor=null;s.diskChanged='';s.notice='';
    text.replaceChildren();
    // The editor is the tab stop; a focused wrapper would ignore Vim keys.
    text.tabIndex=path?-1:0;
    if (!path) { text.textContent=source; return; }
    s.editor=libroFileEditor.create({element:text,path,source,wrap:s.wrap.checked,
      onSymbols:()=>picker(s,'symbols'),onNavigate:kind=>codeNavigation(s,kind),onChange:edited=>{if(edited)s.notice='';context(s);},
      onCommand:(name,force)=>command(s,name,force),copy:value=>copy(s,value)});
    const pos=s.positions.get(s.parents+':'+path);
    if (pos) { s.editor.jump(pos.line,pos.column);const editor=s.editor;requestAnimationFrame(()=>{if(s.editor===editor && !s.jumpPending){editor.view.scrollDOM.scrollTop=pos.top;editor.view.scrollDOM.scrollLeft=pos.left;}}); }
    context(s);
  }
  function location(s) { return s.file ? {path:s.file.path,parents:s.parents,...(s.editor?.position() || {line:1,column:0})} : null; }
  function record(s, place) {
    if (!place) return;
    const previous=s.history[s.historyIndex];
    if (previous && previous.path===place.path && previous.parents===place.parents && previous.line===place.line && previous.column===place.column) return;
    s.history.splice(s.historyIndex+1); s.history.push(place);
    if(s.history.length>100) s.history.shift();
    s.historyIndex=s.history.length-1;
  }
  function navigate(s,path,position,parents=s.parents,history=true,focus=true,edit=false) {
    if((parents!==s.parents || path!==s.file?.path) && s.editor?.dirty()){guard(s,()=>navigate(s,path,position,parents,history,focus,edit));return;}
    remember(s);
    if (history) record(s,location(s));
    if (parents===s.parents && path===s.file?.path && s.editor && !s.el.querySelector('.ws-file-text').hidden) {
      s.previewRequest=null;
      if(position) s.editor.jump(position.line,position.column);
      if(edit)s.editor.setEditable(true);
      if(history)record(s,location(s));
      if(focus)focusPreview(s);
      return;
    }
    const token=request(s,path,true,false,parents);
    Object.assign(s.pending.get(token),{position,history,focus,edit});
  }
  function historyMove(s,direction) {
    const next=s.historyIndex+direction;
    if(next<0 || next>=s.history.length) {s.status.textContent='No more jump history';return;}
    s.history[s.historyIndex]=location(s);
    s.historyIndex=next;
    const target=s.history[next];
    navigate(s,target.path,target,target.parents,false);
  }
  function fuzzy(value,query) {
    if(!query)return 0;
    value=value.toLowerCase();query=query.toLowerCase();let at=0,score=0,last=-1;
    for(const char of query){const next=value.indexOf(char,at);if(next<0)return -1;score+=next===last+1?10:1;last=next;at=next+1;}
    return score-value.length/1000;
  }
  function indexFiles(s) {
    clearTimeout(s.indexTimer);
    if(s.indexed && s.indexIgnored===s.hidden.checked){render(s);return;}
    s.indexRequest=null;
    s.indexTimer=setTimeout(()=>{
      if(!s.filter.value)return;
      const token=String(++sequence);s.indexRequest=token;
      s.pending.set(token,{kind:'index',ignored:s.hidden.checked});s.status.textContent='Finding project files…';
      __ws.call('files.index',{sid:window.__libroWorkspaceSID,id:s.id,request:token,ignored:s.hidden.checked});
    },150);
  }
  function clearFilter(s) {
    s.filter.value='';s.indexRequest=null;s.indexed=null;
    clearTimeout(s.indexTimer);
    s.index=s.treeIndex || 0;render(s);
  }
  function closePicker(s,focus=true) {
    clearTimeout(s.searchTimer);s.searchRequest=null;s.navigationRequest=null;
    const opener=s.picker?.opener;
    s.picker?.covered?.forEach(el=>{el.inert=false;});
    s.picker?.element.remove();s.picker=null;
    if(focus && opener?.isConnected && s.el.contains(opener))opener.focus();
    else if(focus)focusPreview(s);
  }
  function picker(s,kind,query='') {
    // A clicked toolbar button is not a place to return to.
    const active=document.activeElement,opener=active?.closest('.ws-file-toolbar')?s.focusBack:active;
    closePicker(s,false);
    const element=document.createElement('section');element.className='ws-file-picker'+(kind==='navigation'?' ws-file-navigation':kind==='help'?' ws-file-help-dialog':kind==='symbols'?' ws-file-symbol-dialog':'');
    const header=document.createElement('div');header.className='ws-file-picker-header';
    const input=document.createElement('input');input.className='ws-file-filter';
    const title={search:'Search project',symbols:'File symbols',recent:'Recent files',help:'Keyboard shortcuts',navigation:navigationTitles[s.navigationKind]}[kind];
    input.placeholder=kind==='help'?'Search shortcuts':title;input.setAttribute('aria-label',input.placeholder);input.value=query;
    const close=document.createElement('button');close.type='button';close.className='ws-button';close.textContent='Close';close.onclick=()=>closePicker(s);
    if(kind!=='navigation')header.append(input);if(kind!=='symbols')header.append(close);element.append(header);
    if(kind==='navigation'||kind==='help'){
      const heading=document.createElement('h2');heading.className='ws-file-navigation-title';heading.textContent=title;header.prepend(heading);
    }
    const list=document.createElement('div');list.className='ws-file-results';list.tabIndex=0;list.setAttribute('role','listbox');list.setAttribute('aria-label',title+' results');
    const message=document.createElement('div');message.className='ws-file-result-status';message.setAttribute('role','status');
    const p=s.picker={element,input,list,message,kind,opener,index:0,items:[],sourceItems:[]};
    if(kind==='search'){
      const label=document.createElement('label');label.className='ws-file-hidden';
      const ignored=document.createElement('input');ignored.type='checkbox';label.append(ignored,document.createTextNode('Include hidden and ignored files'));element.append(label);p.ignored=ignored;ignored.onchange=()=>update();
    }
    element.append(list,message);
    if(kind==='symbols'){
      input.placeholder='Search buffer symbols…';element.setAttribute('role','dialog');element.setAttribute('aria-label','Buffer symbols');
      const hints=document.createElement('div');hints.className='ws-file-symbol-keys';hints.textContent='↑/↓ move   Ctrl+d/u half page   gg/G first/last   / search   Backspace erase   Enter open   Esc unfocus   2× Esc close';element.append(hints);
    }
    if(kind==='navigation'){
      const hints=document.createElement('div');hints.className='ws-file-navigation-keys';
      hints.textContent='j/k · Ctrl+n/p · ]e/[e next/previous   Ctrl+d/u half page   gg/G first/last   e/Enter open   q/Esc return';element.append(hints);
    }
    if(kind==='help'){
      const hints=document.createElement('div');hints.className='ws-file-navigation-keys';
      hints.textContent='↑/↓ move   Ctrl+d/u half page   gg/G first/last   / search   Esc close';element.append(hints);
    }
    s.el.querySelector('.ws-file-preview').prepend(element);
    if(kind==='navigation'||kind==='help'){element.setAttribute('role','region');element.setAttribute('aria-label',title);p.covered=Array.from(element.parentElement.children).filter(el=>el!==element);p.covered.forEach(el=>{el.inert=true;});}
    function update(){
      p.index=0;
      if(kind==='search'){
        clearTimeout(s.searchTimer);s.searchRequest=null;p.items=[];drawPicker(s);
        if(!input.value.trim()){message.textContent='Enter text to search the project.';return;}
        message.textContent='Searching…';
        s.searchTimer=setTimeout(()=>{
          const token=String(++sequence);s.searchRequest=token;s.pending.set(token,{kind:'search'});
          __ws.call('files.search',{sid:window.__libroWorkspaceSID,id:s.id,request:token,query:input.value,ignored:p.ignored.checked});
        },200);return;
      }
      const source=kind==='navigation'?p.sourceItems:kind==='symbols'?(s.editor?.symbols() || []).map(item=>({...item,label:(item.kind||'')+' '+(item.name||item.label)})):
        kind==='recent'?Array.from(s.recent.values()).reverse().map(item=>({...item,label:item.path})):
        shortcutHelp;
      p.items=source.filter(item=>fuzzy(item.label || item.path,input.value)>=0);
      drawPicker(s);
    }
    p.update=update;
    input.oninput=update;
    element.onkeydown=event=>{
      if(event.key==='Escape'){event.preventDefault();event.stopPropagation();if(kind==='symbols'&&event.target===input){list.focus();return;}closePicker(s);return;}
      const inList=event.target===list;
      if(event.ctrlKey&&['d','u'].includes(event.key)){
        event.preventDefault();event.stopPropagation();const row=list.querySelector('.ws-file-result');const half=Math.max(1,Math.floor(list.clientHeight/(row?.getBoundingClientRect().height||26)/2));p.index+=(event.key==='d'?1:-1)*half;drawPicker(s);list.focus();return;
      }
      if(inList&&(event.key==='G'||event.key==='g'&&p.lastKey==='g')){event.preventDefault();p.index=event.key==='G'?p.items.length-1:0;p.lastKey=null;drawPicker(s);return;}
      p.lastKey=inList?event.key:null;
      if(kind==='symbols'&&inList&&event.key==='Backspace'){event.preventDefault();input.value=input.value.slice(0,-1);update();return;}
      if(kind==='symbols'&&event.key==='Tab'){event.preventDefault();(inList?input:list).focus();return;}
      if(kind==='navigation'&&inList){
        let delta=0;
        if(event.key==='q'){event.preventDefault();closePicker(s);return;}
        if(event.key==='e'&&p.prefix){delta=p.prefix===']'?1:-1;p.prefix=null;}
        else if(event.key==='['||event.key===']'){p.prefix=event.key;event.preventDefault();return;}
        else {p.prefix=null;if(event.key==='*'||event.ctrlKey&&['n','e'].includes(event.key))delta=1;
          if(event.key==='#'||event.ctrlKey&&event.key==='p')delta=-1;}
        if(delta){event.preventDefault();event.stopPropagation();p.index+=delta;drawPicker(s);return;}
        if(event.key==='e'){event.preventDefault();choosePicker(s);return;}
      }
      if(event.key==='ArrowDown'||event.key==='ArrowUp'||inList&&['j','k'].includes(event.key)){
        event.preventDefault();event.stopPropagation();p.index+=['ArrowDown','j'].includes(event.key)?1:-1;drawPicker(s);list.focus();
      }else if(kind!=='navigation' && inList && event.key==='/'){event.preventDefault();input.focus();input.select();}
      // Buttons keep their own Enter.
      else if(event.key==='Enter'&&(inList||event.target===input)){event.preventDefault();event.stopPropagation();choosePicker(s);}
    };
    update();if(kind==='navigation')list.focus();else input.focus();
  }
  function drawPicker(s) {
    const p=s.picker;if(!p)return;
    p.index=Math.max(0,Math.min(p.index,p.items.length-1));
    if(p.renderedItems!==p.items){
    p.renderedItems=p.items;p.list.replaceChildren();
    let group=null;
    p.items.forEach((item,index)=>{
      const row=document.createElement('div');row.className='ws-file-result';row.id='file-result-'+s.id+'-'+index;row.setAttribute('role','option');row.setAttribute('aria-selected',String(index===p.index));
      if(p.kind==='navigation'){
        const key=(item.parents||0)+':'+item.path;
        if(group!==key){
          const heading=document.createElement('div');heading.className='ws-file-reference-file';
          const marker=document.createElement('span');marker.textContent='⌄';marker.setAttribute('aria-hidden','true');
          const name=document.createElement('strong');name.textContent=item.path.split('/').pop();
          const path=document.createElement('span');path.className='ws-file-reference-directory';path.textContent=item.path.includes('/')?item.path.slice(0,item.path.lastIndexOf('/')+1):'./';
          const open=document.createElement('button');open.type='button';open.className='ws-file-reference-open';open.textContent='Open File ↵';
          open.onclick=()=>{const selected=p.items[p.index];if(!selected||selected.path!==item.path||selected.parents!==item.parents)p.index=index;choosePicker(s);};
          heading.append(marker,name,path,open);p.list.append(heading);group=key;
        }
        row.setAttribute('aria-label',item.path+':'+item.line+':'+(item.column+1));
        if(item.context){
          const snippet=document.createElement('div');snippet.className='ws-file-reference-code';
          const numbers=document.createElement('pre');numbers.className='ws-file-reference-numbers';numbers.setAttribute('aria-hidden','true');
          const lines=item.context.split('\n');numbers.textContent=lines.map((_,i)=>item.startLine+i).join('\n');
          const code=document.createElement('pre');
          const target=Math.max(0,item.line-item.startLine),offset=lines.slice(0,target).reduce((n,line)=>n+line.length+1,0)+item.column;
          const word=(item.context.slice(offset).match(/^[\p{L}\p{N}_$]+/u)||[''])[0];
          snippet.style.setProperty('--reference-line',target);
          libroFileEditor.highlightContext(code,item.path,item.context,offset,offset+Math.max(1,word.length));
          snippet.append(numbers,code);row.append(snippet);
        }else row.textContent='Line '+item.line+' · source preview unavailable';
      }else if(p.kind==='symbols'){
        row.style.paddingLeft=(10+(item.depth||0)*16)+'px';
        const type=document.createElement('span');type.className='ws-file-symbol-kind';type.textContent=(item.kind||'')+' ';
        const name=document.createElement('span');name.className='ws-file-symbol-name';name.textContent=item.name||item.label;row.append(type,name);
      }else if(p.kind==='help'){
        const keys=document.createElement('kbd');keys.className='ws-file-shortcut-keys';keys.textContent=item.keys||'';
        const action=document.createElement('span');action.className='ws-file-shortcut-action';action.textContent=item.action||'';row.append(keys,action);
      }else row.textContent=item.label || `${item.path}:${item.line}  ${item.text}`;
      row.onclick=()=>{p.index=index;choosePicker(s);};p.list.append(row);
    });
    }else p.list.querySelectorAll('.ws-file-result').forEach((row,index)=>row.setAttribute('aria-selected',String(index===p.index)));
    if(p.items.length)p.list.setAttribute('aria-activedescendant','file-result-'+s.id+'-'+p.index);else p.list.removeAttribute('aria-activedescendant');
    const selected=p.list.querySelector('[aria-selected=true]');
    (selected?.querySelector('.ws-file-reference-hit')||selected)?.scrollIntoView({block:'nearest'});
    p.message.textContent=p.loading?navigationLoadingMessage(s.navigationKind):p.items.length?`${p.items.length} results`:'No results';
  }
  function choosePicker(s) {
    const p=s.picker,item=p?.items[p.index];if(!item || p.kind==='help')return;
    const kind=p.kind;closePicker(s,false);
    if(kind==='search'||kind==='navigation')s.preview.checked=false;
    if(kind==='symbols')navigate(s,s.file.path,item);
    else navigate(s,item.path,item,kind==='search'?0:item.parents);
  }
  const navigationTitles={definition:'Definitions',references:'References',declaration:'Declarations',implementation:'Implementations',typeDefinition:'Type definitions',restart:'Restart language server'};
  function navigationLoadingMessage(kind){return kind==='restart'?'Restarting language server…':'Finding '+navigationTitles[kind].toLowerCase()+'… First use may take longer while the language server initializes.';}
  function codeNavigation(s,kind){
    if(!s.editor || !s.file)return;
    // The language server reads the file from disk.
    if(s.editor.dirty()){s.notice='Save the file (:w) to use code navigation';context(s);return;}
    const origin=location(s),position=s.editor.position();
    s.navigationKind=kind;
    picker(s,'navigation');
    const token=String(++sequence);s.navigationRequest=token;
    s.pending.set(token,{kind:'navigation',navigation:kind,origin});
    s.picker.loading=true;s.picker.message.textContent=navigationLoadingMessage(kind);
    __ws.call('files.navigate',{sid:window.__libroWorkspaceSID,id:s.id,request:token,path:s.file.path,parents:s.parents,kind,line:position.line,column:position.column,version:s.file.version});
  }
  function focusTree(s,filter=false) {
    if(filter){s.filter.focus();s.filter.select();return;}
    const index=s.items.findIndex(item=>!item.dir && item.path===s.file?.path);
    if(index>=0 && index!==s.index){s.index=index;render(s);}
    s.tree.focus();s.tree.querySelector('[aria-selected=true]')?.scrollIntoView({block:'nearest'});
  }
  // Scroll steps sent to the rendered preview as [x, y, pages].
  const previewScroll={j:[0,40,0],k:[0,-40,0],h:[-40,0,0],l:[40,0,0],G:[0,1e9,0]};
  function scrollPreview(s,event){
    if(event.key!=='g')s.previewG=0;
    if(event.ctrlKey)return {d:[0,0,.5],u:[0,0,-.5]}[event.key];
    if(event.shiftKey && event.key!=='G')return;
    if(event.key==='g'){const twice=Date.now()-(s.previewG || 0)<500;s.previewG=twice?0:Date.now();return twice?[0,-1e9,0]:[0,0,0];}
    return previewScroll[event.key];
  }
  const leaderActions={
    ' ':s=>focusTree(s,true), 'f':s=>focusTree(s), ',':s=>picker(s,'recent'),
    'ss':s=>picker(s,'search'), 'sw':s=>picker(s,'search',s.editor?.selection() || s.editor?.word() || ''),
    'sk':s=>picker(s,'help'), 'lr':s=>codeNavigation(s,'restart'),
    // Toggling replaces the focused view, so keep focus in the preview.
    'p':s=>{const tree=document.activeElement===s.tree;s.preview.checked=!s.preview.checked;s.preview.onchange();if(!tree)focusPreview(s);},
    'yy':s=>{if(s.file)copy(s,s.file.path+':'+(s.editor?.position().line || 1));},
    'yl':s=>leaderActions.yy(s),
    'w':s=>{s.wrap.checked=!s.wrap.checked;s.wrap.onchange();},
  };
  // Capture only Files commands before workspace shortcuts (not text entry or other tools).
  window.addEventListener('keydown',event=>{
    // Find the panel by element, so a stale or reused panel ID cannot swallow its keys.
    const s=event.target.nodeType===1&&[...states.values()].find(item=>item.el.contains(event.target));
    if(!s || event.isComposing)return;
    const typing=event.libroTyping || event.target.matches('input,textarea,select') || event.target.closest('.cm-vim-panel');
    const source=event.target.closest('.cm-content');
    const frame=event.target.matches('.ws-file-rendered'),image=event.target===s.imageView?.media;
    const navigation=source || event.target===s.tree || event.target.matches('.ws-file-text') || event.target.closest('.ws-file-media');
    // Vim keeps keys while it waits for more, such as f<Space> or an Esc that ends a selection.
    const vimBusy=source && s.editor?.pending(),editing=source && s.editor?.editable();
    let action,scroll;
    if(editing && (event.ctrlKey || event.metaKey) && !event.altKey && !event.shiftKey && event.key.toLowerCase()==='s')action=()=>save(s);
    // Insert mode keeps every key, such as Space, Tab and Ctrl+O.
    else if(editing && s.editor.inserting()){}
    else if(navigation && !typing && !event.metaKey && !event.altKey){
      // View mode starts editing like Vim; the key then runs on the editable buffer. u, 2u and Ctrl+R reach changes kept from editing.
      if(source && !editing && s.leader==null && (!vimBusy && !event.ctrlKey && /^[iaoIAO]$/.test(event.key) || s.editor.history(event)))startEditing(s);
      // Workspace shortcuts win over Vim's Ctrl+O and Ctrl+I; Tab still moves forward in source.
      else if(event.ctrlKey && event.key.toLowerCase()==='o' && !window.libroWorkspace?.bound('Ctrl+O'))action=()=>historyMove(s,-1);
      else if((event.ctrlKey && event.key.toLowerCase()==='i' && !window.libroWorkspace?.bound('Ctrl+I')) || source && event.key==='Tab'&&!event.shiftKey)action=()=>historyMove(s,1);
      else if(!event.ctrlKey && (event.key===' ' && !vimBusy || s.leader!=null)){
        if(s.leader==null)s.leader='';else s.leader+=event.key;
        clearTimeout(s.leaderTimer);
        const command=leaderActions[s.leader];
        if(command){action=()=>command(s);s.leader=null;s.status.textContent='';}
        else if(event.key==='Escape'||!Object.keys(leaderActions).some(key=>key.startsWith(s.leader))){s.leader=null;s.status.textContent='';action=()=>{};}
        else {s.status.textContent='Space '+s.leader+' …';s.leaderTimer=setTimeout(()=>{s.leader=null;s.status.textContent='';},1500);action=()=>{};}
      }
      else if((frame || image) && (scroll=scrollPreview(s,event)))action=()=>frame?event.target.contentWindow?.postMessage({libroFilesScroll:scroll},'*'):event.target.scrollBy(scroll[0],scroll[1]+scroll[2]*event.target.clientHeight);
      // Esc goes back to the tree once Vim has nothing left to cancel.
      // Editing keeps an extra Esc in the editor, like Vim.
      else if(!vimBusy && !editing && event.target!==s.tree && event.key==='Escape' && !event.ctrlKey)action=()=>focusTree(s);
    }
    if(action){event.preventDefault();event.stopImmediatePropagation();action();}
  },true);
  function clearImageView(s) {
    if (s.imageResizeObserver) s.imageResizeObserver.disconnect();
    s.imageResizeObserver = null;
    s.imageView = null;
    const tools = s.el.querySelector('.ws-file-image-tools');
    tools.hidden = true;
  }
  function renderImageView(s, anchor) {
    const view = s.imageView;
    if (!view || !view.image.naturalWidth) return;
    const media = view.media, width = media.clientWidth, height = media.clientHeight;
    if (!width || !height) return;
    const old = {left:view.left || 0, top:view.top || 0, width:view.width || 0, height:view.height || 0};
    const base = Math.min(1, width / view.image.naturalWidth, height / view.image.naturalHeight);
    view.width = view.image.naturalWidth * base * view.zoom;
    view.height = view.image.naturalHeight * base * view.zoom;
    view.stage.style.width = Math.max(width, view.width) + 'px';
    view.stage.style.height = Math.max(height, view.height) + 'px';
    view.image.style.width = view.width + 'px';
    view.image.style.height = view.height + 'px';
    view.left = Math.max(0, (width - view.width) / 2);
    view.top = Math.max(0, (height - view.height) / 2);
    view.image.style.left = view.left + 'px';
    view.image.style.top = view.top + 'px';
    if (anchor && old.width && old.height) {
      media.scrollLeft = view.left + anchor.imageX * view.width - anchor.x;
      media.scrollTop = view.top + anchor.imageY * view.height - anchor.y;
    }
    s.el.querySelector('.ws-file-image-reset').textContent = Math.round(view.zoom * 100) + '%';
  }
  function zoomImage(s, zoom, x, y) {
    const view = s.imageView;
    if (!view || !view.width || !view.height) return;
    const media = view.media;
    x = x == null ? media.clientWidth / 2 : x;
    y = y == null ? media.clientHeight / 2 : y;
    const anchor = {
      x, y,
      imageX:Math.max(0, Math.min(1, (media.scrollLeft + x - view.left) / view.width)),
      imageY:Math.max(0, Math.min(1, (media.scrollTop + y - view.top) / view.height))
    };
    view.zoom = Math.max(0.25, Math.min(8, zoom));
    renderImageView(s, anchor);
  }
  function showImage(s, media, image) {
    const stage = document.createElement('div');
    stage.className = 'ws-file-image-stage';
    stage.append(image);
    media.append(stage);
    media.tabIndex = 0;
    media.setAttribute('aria-label', 'Image preview. Use the mouse wheel or plus and minus to zoom. Drag or use arrow keys to move.');
    s.imageView = {media,stage,image,zoom:1,left:0,top:0,width:0,height:0};
    s.el.querySelector('.ws-file-image-tools').hidden = false;
    image.onload = () => renderImageView(s);
    media.onwheel = event => {
      event.preventDefault();
      const rect = media.getBoundingClientRect();
      zoomImage(s, s.imageView.zoom * Math.exp(-event.deltaY * 0.0015), event.clientX - rect.left, event.clientY - rect.top);
    };
    media.onkeydown = event => {
      if (event.key === '+' || event.key === '=') zoomImage(s, s.imageView.zoom * 1.2);
      else if (event.key === '-') zoomImage(s, s.imageView.zoom / 1.2);
      else if (event.key === '0') zoomImage(s, 1);
      else if (event.key === 'ArrowLeft') media.scrollLeft -= 48;
      else if (event.key === 'ArrowRight') media.scrollLeft += 48;
      else if (event.key === 'ArrowUp') media.scrollTop -= 48;
      else if (event.key === 'ArrowDown') media.scrollTop += 48;
      else return;
      event.preventDefault();
    };
    media.onpointerdown = event => {
      if (event.button !== 0) return;
      const left = media.scrollLeft, top = media.scrollTop, x = event.clientX, y = event.clientY;
      media.focus();
      media.setPointerCapture(event.pointerId);
      media.classList.add('is-dragging');
      const move = next => { media.scrollLeft = left - (next.clientX - x); media.scrollTop = top - (next.clientY - y); };
      const stop = () => { media.classList.remove('is-dragging'); media.removeEventListener('pointermove',move); media.removeEventListener('pointerup',stop); media.removeEventListener('pointercancel',stop); };
      media.addEventListener('pointermove',move);
      media.addEventListener('pointerup',stop);
      media.addEventListener('pointercancel',stop);
      event.preventDefault();
    };
    if (window.ResizeObserver) {
      s.imageResizeObserver = new ResizeObserver(() => renderImageView(s));
      s.imageResizeObserver.observe(media);
    }
  }
  function request(s, path, preview = false, external = false, parents = s.parents, version = "") {
    const token = String(++sequence);
    s.pending.set(token, {path, preview, external, parents, doc:s.editor?.doc()});
    if (preview) s.previewRequest = token;
    s.status.textContent = external ? 'Opening…' : 'Loading…';
    __ws.call(external ? 'files.open' : 'files.read', {sid:window.__libroWorkspaceSID, id:s.id, path, parents, request:token,version});
    return token;
  }
  // Reload loaded folders and the open text file; unchanged results are ignored.
  function refresh(s) {
    // The root always reloads, so an empty tree can recover.
    for (const path of ['', ...s.expanded]) if (!path || s.children.has(path)) s.pending.get(request(s,path)).refresh = true;
    // A file the user is opening wins over reloading the shown one.
    const opening=[...s.pending.values()].some(item=>item.preview && !item.refresh);
    if (s.file && !s.file.mime && !opening) s.pending.get(request(s,s.file.path,true,false,s.parents,s.file.version)).refresh = true;
    s.status.textContent = '';
  }
  function clearFile(s) {
    s.file = null;
    Object.assign(s.el.querySelector('.ws-file-path'), {textContent:'Open file'}).dataset.dir = '';
    showText(s, '', 'Select a file from the tree.');
    s.el.querySelector('.ws-file-text').hidden = false;
    s.el.querySelector('.ws-file-media').replaceChildren();
    s.el.querySelector('.ws-file-media').hidden = true;
    clearImageView(s);
    s.wrap.disabled = false;
    s.edit.disabled = true; s.edit.checked = false;
    if (s.url) { URL.revokeObjectURL(s.url); s.url = null; }
  }
  function visible(s) {
    if(s.filter.value) return (s.indexed || []).map(item=>({...item,depth:0,score:fuzzy(item.path,s.filter.value)})).filter(item=>item.score>=0).sort((a,b)=>b.score-a.score).slice(0,500);
    const result = [], query = '';
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
    const selected = s.select ? s.items.findIndex(item => item.path === s.select) : -1;
    if (selected >= 0) { s.index = selected; s.select = null; requestAnimationFrame(() => s.tree.querySelector('[aria-selected=true]')?.scrollIntoView({block:'nearest'})); }
    s.index = Math.max(0, Math.min(s.index, s.items.length - 1));
    s.tree.replaceChildren();
    s.items.forEach((item, index) => {
      const row = document.createElement('div'); row.className = 'ws-file-row'; row.id = 'file-row-' + s.id + '-' + index;
      row.setAttribute('role','treeitem'); row.setAttribute('aria-level', String(item.depth + 1)); row.setAttribute('aria-selected', String(index === s.index));
      if (item.dir) row.setAttribute('aria-expanded', String(s.expanded.has(item.path)));
      row.style.paddingLeft = (8 + item.depth * 16) + 'px';
      const icon = document.createElement('i'); icon.className = 'material-icons-round'; icon.setAttribute('aria-hidden','true');
      icon.textContent = item.dir ? (s.expanded.has(item.path) ? 'expand_more' : 'chevron_right') : 'description';
      const label = document.createElement('span'); label.textContent = s.filter.value ? item.path : item.name; row.title = item.path; row.append(icon,label);
      row.onclick = () => { s.index = index; open(s); s.tree.focus(); };
      s.tree.append(row);
    });
    if (s.items.length) s.tree.setAttribute('aria-activedescendant','file-row-' + s.id + '-' + s.index);
    else { s.tree.removeAttribute('aria-activedescendant'); s.tree.textContent = s.filter.value ? 'No matching project files.' : 'This folder is empty.'; }
    markOpen(s);
  }
  function open(s, expandOnly = false) {
    const item = s.items[s.index]; if (!item) return;
    if (!item.dir) {
      // On small screens the list slides away once a file opens, even under the pointer.
      const sidebar=s.tree.closest('.ws-file-sidebar');if(sidebar.matches(':hover'))sidebar.dataset.closed='';
      render(s);navigate(s,item.path,null,s.filter.value?0:s.parents); return;
    }
    if (s.expanded.has(item.path)) {
      if (!expandOnly) s.expanded.delete(item.path);
      else s.index = Math.min(s.index + 1, s.items.length - 1);
    } else {
      s.expanded.add(item.path);
      if (!s.children.has(item.path)) request(s,item.path);
    }
    render(s);
  }
  // A closed or rebuilt panel keeps its unsaved text until the file opens again.
  const drafts=new Map(),draftKey=(s,path)=>s.project+'\n'+s.parents+'\n'+path;
  function restoreDraft(s) {
    const key=draftKey(s,s.file.path),draft=drafts.get(key);
    if(!draft || !s.editor)return;
    drafts.delete(key);
    startEditing(s);s.editor.view.dispatch({changes:{from:0,to:s.editor.doc().length,insert:draft.text}});
    // Saving checks the version the text was based on.
    if(draft.version!==s.file.version){s.file={...s.file,version:draft.version};s.diskChanged='Changed on disk';}
    s.notice='Restored unsaved changes';context(s);
  }
  function dispose(el) {
    const s = states.get(el.dataset.files);
    if (!s || s.el !== el) return;
    if (s.file && s.editor?.dirty()) drafts.set(draftKey(s,s.file.path),{text:s.editor.doc().toString(),version:s.file.version});
    if (s.url) URL.revokeObjectURL(s.url);
    s.editor?.destroy();s.imageResizeObserver?.disconnect();
    clearInterval(s.refreshTimer);clearTimeout(s.indexTimer);clearTimeout(s.searchTimer);clearTimeout(s.leaderTimer);
    states.delete(s.id);
  }
  function init() {
    for (const s of states.values()) if (!s.el.isConnected) dispose(s.el);
    document.querySelectorAll('[data-files]').forEach(el => {
      if (states.get(el.dataset.files)?.el === el) return;
      const s = {id:el.dataset.files,el,tree:el.querySelector('.ws-file-tree'),filter:el.querySelector('.ws-file-filter'),hidden:el.querySelector('.ws-file-hidden input'),status:el.querySelector('[role=status]'),children:new Map(),expanded:new Set(),pending:new Map(),index:0,parents:0,items:[],positions:new Map(),history:[],historyIndex:-1,recent:new Map()};
      states.set(s.id,s);
      // Panel selection returns focus here, such as the editor line, not always the tree.
      el.addEventListener('focusin',event=>{s.focused=event.target;s.focusedPreview=!!event.target.closest('.ws-file-preview');});
      el.addEventListener('focusout',event=>{if(!el.contains(event.relatedTarget)){s.leader=null;s.status.textContent='';clearTimeout(s.leaderTimer);}});
      el.querySelector('.ws-file-sidebar').addEventListener('mouseleave',event=>delete event.currentTarget.dataset.closed);
      // A mouse click on a toolbar control returns focus to the tree or preview, so Space and Esc keep working.
      el.addEventListener('mousedown',()=>{s.focusBack=document.activeElement;},true);
      el.addEventListener('click',event=>{
        const control=event.target.closest('.ws-file-toolbar label, .ws-file-toolbar button'),back=s.focusBack;
        if(!control || !event.detail)return;
        setTimeout(()=>{
          if(!control.contains(document.activeElement))return;
          if(back?.isConnected && s.el.contains(back) && !back.closest('.ws-file-toolbar'))back.focus();else focusPreview(s);
        });
      });
      s.preview = el.querySelector('.ws-file-render input');
      s.project = el.closest('[data-workspace-project]').dataset.workspaceProject;
      const preferenceKey = 'libro.files.preview:' + s.project;
      try { s.preview.checked = localStorage.getItem(preferenceKey) === 'true'; } catch (_) {}
      s.preview.onchange = () => {
        try { localStorage.setItem(preferenceKey, String(s.preview.checked)); } catch (_) {}
        if (!s.file || !previewable(s.file.path)) return;
        if (s.editor?.dirty()) { const checked=s.preview.checked;s.preview.checked=!checked;guard(s,()=>{s.preview.checked=checked;s.preview.onchange();});return; }
        remember(s);s.jumpPending=false;
        showFile(s);
      };
      s.edit = el.querySelector('.ws-file-edit input');
      s.edit.onchange = () => {
        if (s.edit.checked) { startEditing(s); return; }
        s.edit.checked = true;
        guard(s,() => { s.editor?.setEditable(false); focusPreview(s); });
      };
      s.wrap = el.querySelector('.ws-file-wrap input');
      s.wrap.onchange = () => {el.querySelector('.ws-file-text').classList.toggle('is-wrapped', s.wrap.checked);s.editor?.wrap(s.wrap.checked);};
      s.hidden.onchange = () => { s.index = 0; if(s.filter.value)indexFiles(s);else render(s); };
      el.querySelector('.ws-file-help').onclick = () => picker(s,'help');
      s.filter.onfocus = () => {if(!s.filter.value)s.treeIndex=s.index;};
      s.filter.oninput = () => { s.index = 0;if(s.filter.value)indexFiles(s);else clearFilter(s);render(s); };
      el.querySelector('.ws-file-image-tools').onclick = event => {
        const action = event.target.closest('[data-image-zoom]')?.dataset.imageZoom;
        if (!action || !s.imageView) return;
        zoomImage(s, action === 'reset' ? 1 : s.imageView.zoom * (action === 'in' ? 1.2 : 1 / 1.2));
        s.imageView.media.focus();
      };
      s.filter.onkeydown = event => {
        if(event.key==='Escape'){event.preventDefault();clearFilter(s);s.tree.focus();}
        else if(['ArrowDown','Enter'].includes(event.key)){event.preventDefault();s.tree.focus();if(event.key==='Enter')open(s);}
      };
      s.tree.onkeydown = event => {
        if (event.ctrlKey || event.altKey || event.metaKey) return;
        const key = event.key, item = s.items[s.index];
        if (!['j','k','h','l','g','G','o','a','r','d','/','Enter','Backspace','ArrowDown','ArrowUp','ArrowLeft','ArrowRight','Home','End','Escape'].includes(key)) return;
        event.preventDefault(); event.stopPropagation();
        if(key==='Escape'){if(s.filter.value)clearFilter(s);else focusPreview(s);return;}
        if (key === 'Backspace') {
          if (!event.repeat && !s.parentRequest && s.parents < 1024) guard(s,() => {
            s.parentRequest = true;
            request(s,'',false,false,s.parents + 1);
          });
          return;
        }
        if (event.repeat && ['a','r','d'].includes(key)) return;
        if (key === 'a') { addEntry(s,item); return; }
        if (key === 'r' || key === 'd') { if (item) (key === 'r' ? renameEntry : deleteEntry)(s,item); return; }
        if (key === '/') { focusTree(s,true); return; }
        if (key === 'o') { if (item && !item.dir && !event.repeat) request(s,item.path,false,true,s.filter.value?0:s.parents); return; }
        if (key !== 'g') s.lastG = 0;
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
      el.querySelector('.ws-file-refresh').onclick = () => refresh(s);
      s.refreshTimer=setInterval(()=>{
        // Replies lost to a dropped connection must not stop refreshing.
        for(const [token,item] of s.pending)if(Date.now()-(item.seen??=Date.now())>30000)s.pending.delete(token);
        if(document.hidden || !el.isConnected || !el.getClientRects().length || s.picker || s.pending.size)return;
        refresh(s);
      },3000);
    });
  }
  function markOpen(s) {
    s.tree.querySelectorAll('.ws-file-row').forEach((row, index) => row.setAttribute('aria-current', String(!s.items[index].dir && s.items[index].path === s.file?.path)));
  }
  const previewable = path => /\.(html?|md|markdown)$/i.test(path);
  function showFile(s) {
    markOpen(s);
    const result = s.file;
    // The folder sits muted before the file name.
    const path = s.el.querySelector('.ws-file-path'), cut = result.path.lastIndexOf('/') + 1;
    path.dataset.dir = result.path.slice(0, cut); path.textContent = result.path.slice(cut);
    const text = s.el.querySelector('.ws-file-text'), media = s.el.querySelector('.ws-file-media');
    clearImageView(s);
    media.onwheel = media.onkeydown = media.onpointerdown = null;
    media.removeAttribute('tabindex');
    media.removeAttribute('aria-label');
    media.classList.remove('is-image','is-dragging');
    media.replaceChildren();
    if (s.url) { URL.revokeObjectURL(s.url); s.url = null; }
    text.hidden = !!result.mime;
    media.hidden = !result.mime;
    s.wrap.disabled = !!result.mime;
    s.edit.disabled = !!result.mime; s.edit.checked = false;
    if (!result.mime && s.preview.checked && previewable(result.path)) {
      s.editor?.destroy();s.editor=null;s.el.querySelector('.ws-file-context').textContent='';
      text.hidden = true;
      media.hidden = false;
      s.wrap.disabled = true;
      const frame = document.createElement('iframe');
      frame.className = 'ws-file-rendered';
      frame.title = 'Preview of ' + result.path;
      frame.setAttribute('sandbox', 'allow-scripts');
      const html = result.html || '';
      frame.srcdoc = /<head[^>]*>/i.test(html) ? html.replace(/<head[^>]*>/i, head => head + previewKeys) : previewKeys + html;
      media.append(frame);
      return;
    }
    if (!result.mime) { showText(s, result.path, result.text || ''); return; }
    s.editor?.destroy();s.editor=null;s.el.querySelector('.ws-file-context').textContent='';
    const bytes = Uint8Array.from(atob(result.data || ''), c => c.charCodeAt(0));
    s.url = URL.createObjectURL(new Blob([bytes], {type:result.mime}));
    const kind = result.mime.split('/')[0];
    const element = document.createElement(kind === 'image' ? 'img' : kind === 'audio' || kind === 'video' ? kind : 'iframe');
    if (kind === 'image') element.alt = result.path;
    else if (kind === 'audio' || kind === 'video') { element.controls = true; element.preload = 'metadata'; }
    else element.title = 'Preview of ' + result.path;
    element.onerror = () => { s.status.textContent = 'This browser cannot preview this file. Press o in the file tree to open externally.'; };
    if (kind === 'image') { media.classList.add('is-image'); showImage(s,media,element); }
    else media.append(element);
    element.src = s.url;
  }
  function receive(result) {
    const s = states.get(result.id); if (!s?.el.isConnected) return;
    const pending = s.pending.get(result.request); if (!pending) return;
    s.pending.delete(result.request);
    if(pending.kind){
      if(pending.kind==='save'){
        // The buffer was replaced while saving.
        if(s.editor!==pending.editor){s.status.textContent=result.error || '';return;}
        if(result.error){s.notice=result.error;context(s);return;}
        s.file=result;s.diskChanged='';s.notice='Written';s.editor.saved(pending.doc);
        if(pending.quit && !s.editor.dirty())s.editor.setEditable(false);
        context(s);
      }else if(pending.kind==='change'){
        s.status.textContent=result.error || '';
        if(!result.error)pending.done();
      }else if(pending.kind==='navigation'){
        if(s.navigationRequest!==result.request || s.picker?.kind!=='navigation')return;
        if(s.file?.path!==pending.origin.path || s.parents!==pending.origin.parents){closePicker(s,false);return;}
        s.picker.loading=false;
        const matches=result.matches || [];
        if(result.error){s.picker.message.textContent=result.error;return;}
        if(pending.navigation==='restart'){s.picker.message.textContent='Language server restarted.';return;}
        if(matches.length===1 && pending.navigation!=='references'){
          closePicker(s,false);s.preview.checked=false;
          navigate(s,matches[0].path,matches[0],matches[0].parents || 0);return;
        }
        s.picker.sourceItems=matches.map(item=>({...item,parents:item.parents || 0,label:`${item.path}:${item.line}:${item.column+1}`})).sort((a,b)=>a.parents-b.parents||a.path.localeCompare(b.path)||a.line-b.line||a.column-b.column);
        s.picker.update();s.picker.list.focus();
        if(result.truncated)s.picker.message.textContent='First 200 locations.';
      }else if(pending.kind==='index'){
        if(s.indexRequest!==result.request || !s.filter.value)return;
        s.indexed=result.error?null:(result.entries || []);s.indexIgnored=pending.ignored;s.index=0;render(s);
        s.status.textContent=result.error || (result.truncated?'Showing the first 20,000 project files.':'');
      }else if(s.searchRequest===result.request && s.picker?.kind==='search'){
        s.picker.items=result.matches || [];s.picker.index=0;drawPicker(s);
        if(result.error)s.picker.message.textContent=result.error;
        else if(result.truncated)s.picker.message.textContent='First 500 results. Narrow your search for more.';
      }
      return;
    }
    if (pending.preview && s.previewRequest !== result.request) return;
    if (pending.parents !== s.parents) s.parentRequest = false;
    if (pending.refresh && result.error) {
      // Only a path removed on disk leaves the view. Other errors, such as a
      // refresh that races a project switch, keep the shown content.
      if (!result.missing) return;
      if (pending.path === s.file?.path && s.editor?.editable()) { s.diskChanged='Deleted on disk'; context(s); }
      else if (pending.path === s.file?.path) { clearFile(s); markOpen(s); }
      else if (s.children.delete(pending.path)) { s.expanded.delete(pending.path); render(s); }
      return;
    }
    // Refresh never replaces an edited buffer; :e! reloads it.
    if (pending.refresh && !result.directory && !result.error && s.editor?.editable()) {
      if (!result.unchanged && result.version !== s.file?.version) { s.diskChanged='Changed on disk (:e! reloads)'; context(s); }
      return;
    }
    s.status.textContent = result.error || '';
    if (result.error || pending.external || result.unchanged) return;
    if(pending.refresh && s.file?.path===result.path && s.file.text===result.text && s.file.mime===result.mime){s.file=result;return;}
    // Text typed while a file or folder loaded stays; the reply is dropped.
    if((!result.directory || pending.parents!==s.parents) && s.editor?.dirty() && s.editor.doc()!==pending.doc){s.notice='Kept unsaved changes';context(s);return;}
    if(!result.directory && pending.parents!==s.parents){
      remember(s);s.editor?.destroy();s.editor=null;s.pending.clear();
      s.parents=pending.parents;s.children.clear();s.expanded.clear();s.index=0;request(s,'');
    }
    if (result.directory && pending.parents !== s.parents) {
      remember(s);clearFile(s);clearFilter(s);
      s.parents = pending.parents;
      s.parentRequest = false;
      s.pending.clear();
      s.children.clear();
      s.expanded.clear();
      s.filter.value = '';
      s.index = 0;
    }
    if (result.directory) {
      const entries = result.entries || [];
      if (pending.refresh && JSON.stringify(s.children.get(pending.path)) === JSON.stringify(entries)) return;
      s.children.set(pending.path,entries); render(s);
    }
    else {
      const active=document.activeElement,restoreFocus=pending.refresh && s.el.contains(active) && !!active.closest('.ws-file-text, .ws-file-media');
      remember(s);
      s.file = result;
      s.jumpPending=!!pending.position;showFile(s);
      if(pending.position && s.editor)s.editor.jump(pending.position.line,pending.position.column);
      if(pending.edit)startEditing(s);
      restoreDraft(s);
      if(pending.history!==false && !pending.refresh)record(s,location(s));
      const key=s.parents+':'+result.path;s.recent.delete(key);s.recent.set(key,location(s));
      if(s.recent.size>100)s.recent.delete(s.recent.keys().next().value);
      if(pending.focus)focusPreview(s);
      else if(restoreFocus)focusPreview(s);
    }
  }
  // Runs inside the sandboxed preview, which cannot reach the workspace. It posts
  // shortcut and Vim keys, which are replayed on the iframe element for the
  // workspace handlers, and applies the scroll steps the workspace sends back.
  function previewScript() {
    const typing=target=>!!target.closest?.('input,textarea,select,[contenteditable]');
    addEventListener('keydown',e=>{
      const editing=typing(e.target);
      if(editing && e.key==='Escape'){e.target.blur();return;}
      if(!e.ctrlKey && !e.metaKey && !e.altKey && (editing || e.key.length!==1 && e.key!=='Escape'))return;
      if(e.key===' ')e.preventDefault();
      parent.postMessage({libroFilesKey:{key:e.key,code:e.code,ctrlKey:e.ctrlKey,metaKey:e.metaKey,altKey:e.altKey,shiftKey:e.shiftKey,repeat:e.repeat,typing:editing}},'*');
    },true);
    // Scroll the scrollable box under focus or at the centre, else the page.
    addEventListener('message',e=>{
      const step=e.source===parent && e.data?.libroFilesScroll;if(!step)return;
      let box=document.activeElement===document.body?document.elementFromPoint(innerWidth/2,innerHeight/2):document.activeElement;
      while(box && box!==document.body && box!==document.documentElement && !(box.scrollHeight>box.clientHeight && /auto|scroll/.test(getComputedStyle(box).overflowY)))box=box.parentElement;
      const page=!box || box===document.body || box===document.documentElement;
      (page?window:box).scrollBy(step[0],step[1]+step[2]*(page?innerHeight:box.clientHeight));
    });
  }
  const previewKeys = '<script>('+previewScript+')();<\/script>';
  window.addEventListener('message', event => {
    const key = event.data?.libroFilesKey;
    if (!key) return;
    const frame = [...document.querySelectorAll('[data-files] .ws-file-media iframe')].find(item => item.contentWindow === event.source);
    const {typing,...init}=key,replay=new KeyboardEvent('keydown', {...init, bubbles:true, cancelable:true});
    replay.libroTyping=typing;frame?.dispatchEvent(replay);
  });
  function focus(el) {
    const s=states.get(el.dataset.files),saved=s?.el===el && s.focused;
    if(saved?.isConnected && el.contains(saved))saved.focus({preventScroll:true});
    // A reloaded file replaces its editor; return to the new one.
    else if(saved && s.focusedPreview)focusPreview(s);
    // A disabled or hidden target cannot take focus.
    if(!el.contains(document.activeElement))el.querySelector('.ws-file-tree').focus({preventScroll:true});
  }
  window.libroFiles = {init,receive,dispose,focus};
})();
