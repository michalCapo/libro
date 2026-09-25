package libro

// urlPopupJS handles browser addresses and locally persisted recent entries.
func urlPopupJS(_ string) string {
	return `
(function(){
  var appID='';
  var dialog=document.getElementById('url-popup');
  var input=document.getElementById('url-popup-input');
  if(!dialog||!input)return;
  var results=document.getElementById('url-popup-results');
  var historyKey='libro.browser.history';
  var matches=[], selected=-1;
  var focusRequest=0;
  function history(){
    try {
      var entries=JSON.parse(localStorage.getItem(historyKey)||'[]');
      return Array.isArray(entries)?entries.filter(function(e){return e&&typeof e.url==='string'&&Number.isFinite(e.time)&&e.time>Date.now()-30*86400000;}).slice(0,200):[];
    }catch(_){return [];}
  }
  window.__libroRememberURL=function(url){
    var entries=history().filter(function(e){return e.url!==url;});
    entries.unshift({url:url,time:Date.now()});
    try{localStorage.setItem(historyKey,JSON.stringify(entries.slice(0,200)));}catch(_){}
  };
  function render(){
    results.replaceChildren();
    input.removeAttribute('aria-activedescendant');
    input.setAttribute('aria-expanded',String(matches.length>0));
    matches.forEach(function(entry,index){
      var row=document.createElement('div');
      row.className='ws-command-row';row.id='url-history-'+index;
      row.setAttribute('role','option');row.setAttribute('aria-selected',String(index===selected));
      row.dataset.selected=String(index===selected);
      var icon=document.createElement('i');icon.className='material-icons-round';icon.textContent=entry.application?'language':'history';icon.setAttribute('aria-hidden','true');
      var label=document.createElement('span');label.className='ws-command-label';label.textContent=entry.url;
      row.append(icon,label);
      row.addEventListener('mousedown',function(e){e.preventDefault();});
      row.addEventListener('click',function(){if(window.__libroNavigateAddress(appID,entry.url))close();});
      results.appendChild(row);
      if(index===selected){input.setAttribute('aria-activedescendant',row.id);row.scrollIntoView({block:'nearest'});}
    });
    results.hidden=!matches.length;
  }
  function filter(){
    var query=input.value.trim().toLowerCase();
    var url=window.__libroApplicationURL||'';
    var entries=history();
    if(url){
      entries=entries.filter(function(e){return e.url.replace(/\/$/,'')!==url.replace(/\/$/,'');});
      entries.unshift({url:url,application:true});
    }
    matches=entries.filter(function(e){return e.url.toLowerCase().includes(query);}).slice(0,8);
    selected=query&&matches.length?0:-1;
    render();
  }
  input.addEventListener('input',filter);
  function close(){
    dialog.classList.add('hidden');
    if(window.__libroParkFloatingPopups)window.__libroParkFloatingPopups();
    if(appID&&window.__libroFocusAppByID)window.__libroFocusAppByID(appID);
  }
  function open(id,value){
    appID=id||window.__libroSelectedApp||'';
    var frame=document.getElementById('frame-'+appID);
    var address=document.getElementById('urlinput-'+appID);
    if(!frame||!address)return;
    var content=frame.querySelector('[data-app-content]');
    if(content)content.appendChild(dialog);
    if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dialog);
    input.value=typeof value==='string'?value:address.value;
    if(input.value==='about:blank')input.value='';
    filter();
    if(!input.value&&matches.length)input.value=matches[0].url;
    dialog.classList.remove('hidden');
    input.focus();input.select();
    // DOM focus alone does not transfer native keyboard focus from an Electron guest.
    var request=++focusRequest;
    if(window.libroElectron&&window.libroElectron.focusWorkspace){
      window.libroElectron.focusWorkspace().then(function(){
        if(request!==focusRequest||dialog.classList.contains('hidden'))return;
        input.focus();
      }).catch(function(){});
    }
  }
  input.addEventListener('keydown',function(event){
    if(event.key==='ArrowDown'||event.key==='ArrowUp'){
      event.preventDefault();event.stopImmediatePropagation();
      if(matches.length){selected=(selected+(event.key==='ArrowDown'?1:matches.length-1)+matches.length)%matches.length;render();}
    }else if(event.key==='Enter'){
      event.preventDefault();event.stopImmediatePropagation();
      if(window.__libroNavigateAddress(appID,selected>=0?matches[selected].url:input.value))close();
    }else if(event.key==='Escape'){
      event.preventDefault();event.stopImmediatePropagation();close();
    }
  });
  window.__libroOpenURLPopup=function(){open();};
  window.__libroOpenURLPopupFor=open;
})();`
}
