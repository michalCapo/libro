package libro

// urlPopupJS handles direct browser addresses without search or stored history.
func urlPopupJS(_ string) string {
	return `
(function(){
  var appID='';
  var dialog=document.getElementById('url-popup');
  var input=document.getElementById('url-popup-input');
  if(!dialog||!input)return;
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
    dialog.classList.remove('hidden');
    input.focus();input.select();
  }
  input.addEventListener('keydown',function(event){
    if(event.key==='Enter'){
      event.preventDefault();event.stopImmediatePropagation();
      if(window.__libroNavigateAddress(appID,input.value))close();
    }else if(event.key==='Escape'){
      event.preventDefault();event.stopImmediatePropagation();close();
    }
  });
  window.__libroOpenURLPopup=function(){open();};
  window.__libroOpenURLPopupFor=open;
})();`
}
