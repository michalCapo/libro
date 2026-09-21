// Run: xvfb-run -a node_modules/.bin/electron --no-sandbox electron/notes.integration.cjs
const {app, BrowserWindow} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  const win = new BrowserWindow({width:640,height:900,show:true});
  win.webContents.on('console-message', event => console.log(event.message));
  try {
    const css = fs.readFileSync(path.join(__dirname, '../internal/workspace.css'), 'utf8');
    await win.loadURL('data:text/html,' + encodeURIComponent('<link rel="stylesheet" href="https://fonts.googleapis.com/icon?family=Material+Icons+Round"><style>body{margin:0;font-family:system-ui}button{cursor:pointer}[data-workspace-project]{height:100vh}</style><div id="libro-workspace"><div data-workspace-project="test"><div data-notes="notes" class="ws-notes"></div></div></div>'));
    await win.webContents.insertCSS(css);
    await win.webContents.executeJavaScript(`
      window.__libroActiveProject = 'test'; window.__libroWorkspaceSID = 'test';
      window.savedNotes = []; window.sent = [];
      window.__libroSendPageToolPrompt = (prompt, execute) => { sent.push({prompt,execute}); return true; };
      window.fetch = async (url, options) => {
        const data = JSON.parse(options.body), action = 'notes.' + data.action;
        const result = {id:data.id,request:data.request};
        if(action === 'notes.list') { result.notes = structuredClone(savedNotes.filter(note => note.project === data.project)); result.projects = ['test', 'other'].filter(project => project !== data.project); }
        if(action === 'notes.save') {
          result.note = {...structuredClone(data.note), id:data.note.id || String(savedNotes.length+1), project:data.project};
          savedNotes = [result.note,...savedNotes.filter(note=>note.id !== result.note.id)];
        }
        if(action === 'notes.move') { savedNotes.find(note => note.id === data.noteID && note.project === data.project).project = data.target; result.noteID = data.noteID; }
        if(action === 'notes.send') result.prompt = savedNotes.find(note=>note.id === data.noteID).body;
        return {ok:true, json:async () => result};
      };
      window.clickText = text => Array.from(document.querySelectorAll('button')).find(button=>button.firstChild?.textContent.trim()===text).click();
      window.input = (selector,value) => { const el=document.querySelector(selector); el.value=value; el.dispatchEvent(new Event('input')); };
      void 0;
    `);
    await win.webContents.executeJavaScript(fs.readFileSync(path.join(__dirname, '../internal/note-editor.bundle.js'), 'utf8'));
    await win.webContents.executeJavaScript(fs.readFileSync(path.join(__dirname, '../internal/notes.js'), 'utf8') + ';libroNotes.init();');
    await new Promise(resolve => setTimeout(resolve, 100));
    await win.webContents.executeJavaScript(`
      window.editor = () => document.querySelector('.ws-note-body').editor;
      window.pasteText = text => {
        const transfer = new DataTransfer(); transfer.setData('text/plain', text);
        document.querySelector('.ws-note-body').dispatchEvent(new ClipboardEvent('paste', {clipboardData:transfer,bubbles:true,cancelable:true}));
      };
      clickText('New issue'); input('.ws-note-title','Improve document requests');
      editor().commands.focus();
      pasteText('## Office logo\\n\\nShow the **office logo** above the request.\\n\\nSender information follows below.');
      editor().commands.setTextSelection(14);
      void 0;
    `);
    const png = fs.readFileSync(path.join(__dirname, '../winres/icon.png')).toString('base64');
    await win.webContents.executeJavaScript(`
      window.pasteImage = () => {
        const bytes = Uint8Array.from(atob(${JSON.stringify(png)}), c=>c.charCodeAt(0));
        const transfer = new DataTransfer(); transfer.items.add(new File([bytes], 'clipboard.png', {type:'image/png'}));
        document.querySelector('.ws-note-body').dispatchEvent(new ClipboardEvent('paste', {clipboardData:transfer,bubbles:true,cancelable:true}));
      };
      pasteImage();
      if (!Array.from(document.querySelectorAll('button')).find(button=>button.textContent==='Save issue').disabled) throw new Error('Save must wait for clipboard reads');
    `);
    await new Promise(resolve => setTimeout(resolve, 200));
    assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-note-body img').naturalWidth > 0`), true, 'pasted image is visible inside editor');
    assert.equal(await win.webContents.executeJavaScript(`!!document.querySelector('.ws-note-body strong') && !!document.querySelector('.ws-note-body h2')`), true, 'Markdown rendered in editable area');
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('textarea,.ws-note-preview').length`), 0, 'one editor without separate preview');
    assert.equal(await win.webContents.executeJavaScript(`editor().getMarkdown().indexOf('![Screenshot]') < editor().getMarkdown().indexOf('Sender information')`), true, 'image stays at paste position');
    await win.webContents.executeJavaScript(`editor().commands.undo(); void 0;`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-body img').length`), 0, 'undo image paste');
    await win.webContents.executeJavaScript(`editor().commands.redo(); void 0;`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-body img').length`), 1, 'redo image paste');
    fs.mkdirSync(path.join(__dirname, '../.impeccable/review'), {recursive:true});
    for (const width of [1209,640,320]) {
      win.setSize(width,900);
      await new Promise(resolve => setTimeout(resolve, 100));
      assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-notes').scrollWidth <= document.querySelector('.ws-notes').clientWidth`), true, 'no overflow at '+width);
      fs.writeFileSync(path.join(__dirname, '../.impeccable/review/notes-'+width+'.png'), (await win.webContents.capturePage()).toPNG());
    }
    await win.webContents.executeJavaScript(`window.originalBody=editor().getMarkdown(); clickText('Save issue');`);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`savedNotes[0].images.length`), 1, 'image persisted');
    assert.equal(await win.webContents.executeJavaScript(`savedNotes[0].body === originalBody && editor().getMarkdown() === originalBody`), true, 'Markdown and image position survive reopening');
    await win.webContents.executeJavaScript(`editor().commands.setContent('Discard this',{contentType:'markdown'}); clickText('Cancel'); if (!document.activeElement.matches('.ws-note-row')) throw new Error('Cancel lost note focus'); document.querySelector('.ws-note-row').click();`);
    assert.equal(await win.webContents.executeJavaScript(`editor().getMarkdown() === originalBody`), true, 'cancel discards changes');
    await win.webContents.executeJavaScript(`clickText('Send to agent')`);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`sent.length === 1 && sent[0].prompt === originalBody && sent[0].execute`), true, 'send saved Markdown to agent');
    await win.webContents.executeJavaScript(`
      let imagePos; editor().state.doc.descendants((node,pos)=>{if(node.type.name==='image')imagePos=pos});
      editor().chain().setNodeSelection(imagePos).deleteSelection().run();
      clickText('Save issue');
    `);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`savedNotes[0].images.length`), 0, 'deleting image also removes saved attachment');
    await win.webContents.executeJavaScript(`document.querySelector('[aria-label="Archive issue"]').click(); clickText('Save issue')`);
    await new Promise(resolve => setTimeout(resolve, 100));
    await win.webContents.executeJavaScript(`clickText('Cancel')`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-row').length`), 0, 'archived note hidden from Open');
    await win.webContents.executeJavaScript(`clickText('Archived')`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-row').length`), 1, 'archived note accessible');
    await win.webContents.executeJavaScript(`
      savedNotes[0].images=[{data:'data:image/png;base64,'+${JSON.stringify(png)}}];
      document.querySelector('.ws-note-row').click(); clickText('Cancel');
      document.querySelector('[data-notes]').replaceChildren();
      document.querySelector('[data-notes]').dataset.notes='legacy'; libroNotes.init();
    `);
    await new Promise(resolve => setTimeout(resolve, 100));
    await win.webContents.executeJavaScript(`clickText('Archived'); document.querySelector('.ws-note-row').click();`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-body img').length`), 1, 'legacy attachments remain visible');
    await win.webContents.executeJavaScript(`
      editor().commands.setContent('![Remote](https://example.invalid/tracker.png)', {contentType:'markdown'});
      void 0;
    `);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-body img').length`), 0, 'external images never load');
    await win.webContents.executeJavaScript(`pasteImage(); clickText('Cancel'); clickText('New issue'); input('.ws-note-title','New draft');`);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-body img').length`), 0, 'cancelled paste cannot leak into another draft');
    win.show(); win.focus();
    await win.webContents.executeJavaScript(`editor().commands.focus(); void 0;`);
    await new Promise(resolve => setTimeout(resolve, 100));
    for (const character of '# ') win.webContents.sendInputEvent({type:'char',keyCode:character});
    await new Promise(resolve => setTimeout(resolve, 50));
    assert.equal(await win.webContents.executeJavaScript(`!!document.querySelector('.ws-note-body h1')`), true, 'typed Markdown heading shortcut');
    await win.webContents.executeJavaScript(`editor().commands.setContent(''); editor().commands.focus(); void 0;`);
    await new Promise(resolve => setTimeout(resolve, 100));
    for (const character of '**bold**') win.webContents.sendInputEvent({type:'char',keyCode:character});
    await new Promise(resolve => setTimeout(resolve, 50));
    assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-note-body strong')?.textContent`), 'bold', 'typed Markdown bold shortcut');
    await win.webContents.executeJavaScript(`
      clickText('Cancel'); clickText('Archived'); document.querySelector('.ws-note-row').click();
      window.issueToMove = savedNotes[0].id;
      const picker = document.querySelector('[aria-label="Move issue to project"]');
      picker.value = 'other'; picker.dispatchEvent(new Event('change'));
      input('.ws-note-title', 'Unsaved title');
      if (!Array.from(document.querySelectorAll('button')).find(button => button.textContent === 'Move issue').disabled) throw new Error('Move must require saved changes');
      input('.ws-note-title', savedNotes[0].title);
      clickText('Move issue');
    `);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-row').length`), 0, 'moved issue disappears from source');
    await win.webContents.executeJavaScript(`
      const grid = document.querySelector('[data-workspace-project]');
      grid.dataset.workspaceProject = 'other'; window.__libroActiveProject = 'other'; libroNotes.init();
    `);
    await new Promise(resolve => setTimeout(resolve, 100));
    await win.webContents.executeJavaScript(`clickText('Archived')`);
    assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-note-row').dataset.noteId === issueToMove`), true, 'same panel rebinds to destination project');
    await win.webContents.executeJavaScript(`document.querySelector('.ws-note-row').click()`);
    for (const width of [1209,640,320]) {
      win.setSize(width,900);
      await new Promise(resolve => setTimeout(resolve, 100));
      assert.equal(await win.webContents.executeJavaScript(`document.querySelector('.ws-notes').scrollWidth <= document.querySelector('.ws-notes').clientWidth`), true, 'move controls fit at '+width);
      fs.writeFileSync(path.join(__dirname, '../.impeccable/review/issues-move-'+width+'.png'), (await win.webContents.capturePage()).toPNG());
    }
    await win.webContents.executeJavaScript(`
      clickText('Cancel'); window.__libroActiveProject = 'test'; libroNotes.init();
      savedNotes = []; window.__libroActiveProject = 'other'; libroNotes.init();
    `);
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.equal(await win.webContents.executeJavaScript(`document.querySelectorAll('.ws-note-row').length`), 0, 'returning to project refreshes cached list');
    console.log('Notes editor integration passed');
  } finally { win.destroy(); app.quit(); }
}).catch(error => { console.error(error); app.exit(1); });
