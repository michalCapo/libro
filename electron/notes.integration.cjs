// Run: xvfb-run -a node_modules/.bin/electron --no-sandbox electron/notes.integration.cjs
const {app, BrowserWindow} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  const win = new BrowserWindow({width:640,height:900,show:false});
  try {
    const css = fs.readFileSync(path.join(__dirname, '../internal/workspace.css'), 'utf8');
    await win.loadURL('data:text/html,' + encodeURIComponent('<style>body{margin:0;font-family:system-ui}button{cursor:pointer}[data-workspace-project]{height:100vh}</style><div id="libro-workspace"><div data-workspace-project="test"><div data-notes="notes" class="ws-notes"></div></div></div>'));
    await win.webContents.insertCSS(css);
    await win.webContents.executeJavaScript(`
      window.__libroActiveProject = 'test'; window.__libroWorkspaceSID = 'test';
      window.savedNotes = []; window.sent = [];
      window.__libroSendPageToolPrompt = (prompt, execute) => { sent.push({prompt,execute}); return true; };
      window.fetch = async (url, options) => {
        const data = JSON.parse(options.body), action = 'notes.' + data.action;
        const result = {id:data.id,request:data.request};
        if(action === 'notes.list') result.notes = structuredClone(savedNotes);
        if(action === 'notes.preview') result.html = '<p><strong>Bold</strong> task</p>';
        if(action === 'notes.save') {
          result.note = {...structuredClone(data.note), id:data.note.id || String(savedNotes.length+1)};
          savedNotes = [result.note,...savedNotes.filter(note=>note.id !== result.note.id)];
        }
        if(action === 'notes.send') result.prompt = savedNotes.find(note=>note.id === data.noteID).body;
        return {ok:true, json:async () => result};
      };
      window.clickText = text => Array.from(document.querySelectorAll('button')).find(button=>button.textContent===text).click();
      window.input = (selector,value) => { const el=document.querySelector(selector); el.value=value; el.dispatchEvent(new Event('input')); };
      void 0;
    `);
    await win.webContents.executeJavaScript(fs.readFileSync(path.join(__dirname, '../internal/notes.js'), 'utf8') + ';libroNotes.init();');
    await new Promise(resolve => setTimeout(resolve, 100));
    await win.webContents.executeJavaScript(`clickText('Add note'); input('.ws-note-title','Fix toolbar alignment'); input('textarea','**Bold** task');`);
    const png = fs.readFileSync(path.join(__dirname, '../winres/icon.png')).toString('base64');
    await win.webContents.executeJavaScript(`
      const bytes = Uint8Array.from(atob(${JSON.stringify(png)}), c=>c.charCodeAt(0));
      const transfer = new DataTransfer(); transfer.items.add(new File([bytes], 'clipboard.png', {type:'image/png'}));
      document.querySelector('textarea').dispatchEvent(new ClipboardEvent('paste', {clipboardData:transfer,bubbles:true,cancelable:true}));
      if (!Array.from(document.querySelectorAll('button')).find(button=>button.textContent==='Save note').disabled) throw new Error('Save must wait for clipboard reads');
    `);
    await new Promise(resolve => setTimeout(resolve, 350));
    assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-note-image img').naturalWidth > 0`), true, 'pasted image is visible');
    assert.equal(await win.webContents.executeJavaScript(`!!document.querySelector('.ws-note-preview strong')`), true, 'live markdown preview');
    fs.mkdirSync(path.join(__dirname, '../.impeccable/review'), {recursive:true});
    for (const width of [640,320]) {
      win.setSize(width,900);
      await new Promise(resolve => setTimeout(resolve, 100));
      assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-notes').scrollWidth <= document.querySelector('.ws-notes').clientWidth`), true, 'no overflow at '+width);
      fs.writeFileSync(path.join(__dirname, '../.impeccable/review/notes-'+width+'.png'), (await win.webContents.capturePage()).toPNG());
    }
    await win.webContents.executeJavaScript(`clickText('Save note')`);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`savedNotes[0].images.length`), 1, 'image persisted');
    await win.webContents.executeJavaScript(`input('textarea','Discard this'); clickText('Cancel'); if (!document.activeElement.matches('.ws-note-row')) throw new Error('Cancel lost note focus'); document.querySelector('.ws-note-row').click();`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelector('textarea').value`), '**Bold** task', 'cancel discards changes');
    await win.webContents.executeJavaScript(`clickText('Send to agent')`);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.deepEqual(await win.webContents.executeJavaScript(`sent`), [{prompt:'**Bold** task',execute:true}]);
    await win.webContents.executeJavaScript(`const state=document.querySelector('.ws-note-editor select'); state.value='archived'; state.dispatchEvent(new Event('change')); clickText('Save note')`);
    await new Promise(resolve => setTimeout(resolve, 100));
    await win.webContents.executeJavaScript(`clickText('Cancel')`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-row').length`), 0, 'archived note hidden from New');
    await win.webContents.executeJavaScript(`const filter=document.querySelector('.ws-notes-toolbar select'); filter.value='archived'; filter.dispatchEvent(new Event('change')); if (!document.activeElement.matches('.ws-notes-toolbar select')) throw new Error('Filter lost focus');`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-row').length`), 1, 'archived note accessible');
    console.log('Notes editor integration passed');
  } finally { win.destroy(); app.quit(); }
}).catch(error => { console.error(error); app.exit(1); });
