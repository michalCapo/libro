// Run with: xvfb-run -a node_modules/.bin/electron --no-sandbox --ozone-platform=x11 electron/page-tool-layer.integration.cjs
const {app, BrowserWindow} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  const win = new BrowserWindow({width:540,height:540,show:true,webPreferences:{backgroundThrottling:false}});
  try {
    const source = fs.readFileSync(path.join(__dirname, '../internal/components/browser.go'),'utf8');
    const start = source.indexOf('var browserShortcutsScript =');
    const end = source.indexOf('window.__libroOpenConsole =', start);
    await win.loadURL('data:text/html,<dialog style="width:95vw;height:90vh;background:green"><button>Page popup</button></dialog>');
    await win.webContents.executeJavaScript(source.slice(start,end) + ';eval(browserShortcutsScript)');
    for (const modal of [false,true]) {
      const result = await win.webContents.executeJavaScript(`(() => {
        if (${modal}) document.querySelector('dialog').showModal();
        window.__libroSetPageToolMode('area');
        const target = ${modal} ? document.querySelector('dialog') : document.body;
        target.dispatchEvent(new PointerEvent('pointerdown', {bubbles:true,button:0,clientX:30,clientY:60,pointerId:1}));
        target.dispatchEvent(new PointerEvent('pointermove', {bubbles:true,clientX:230,clientY:210,pointerId:1}));
        const outline = document.querySelector('.libro-page-tool-area');
        const bounds = outline.getBoundingClientRect();
        const css = getComputedStyle(outline);
        if (!outline.matches(':popover-open') || bounds.x !== 30 || bounds.y !== 60 || bounds.width !== 200 || bounds.height !== 150 || css.borderTopWidth !== '2px' || css.borderTopStyle !== 'solid') throw new Error('Drag outline is not visible at the selected bounds');
        window.__libroSetPageToolMode('');
        if (document.querySelector('.libro-page-tool-area')) throw new Error('Outline was not removed before capture');
        window.__libroPageToolPromptOpen({kind:'area',area:{x:30,y:60,width:200,height:150},screenshot:'/tmp/example.png'},'http://example.test');
        const panel = document.querySelector('[popover]');
        const input = panel.shadowRoot.querySelector('textarea');
        const box = input.getBoundingClientRect();
        const result = {open:panel.matches(':popover-open'),focused:panel.shadowRoot.activeElement === input,top:document.elementFromPoint(box.x+5,box.y+5) === panel,inside:!${modal} || panel.parentElement.matches('dialog:modal'),fits:box.x>=0 && box.right<=innerWidth};
        panel.shadowRoot.querySelector('.close').click();
        result.closed = !panel.isConnected;
        result.modalPreserved = !${modal} || document.querySelector('dialog').open;
        return result;
      })()`);
      for (const [key,value] of Object.entries(result)) assert.equal(value,true,`${modal}: ${key}`);
    }
    const selection = await win.webContents.executeJavaScript(`new Promise(resolve => {
      const originalLog = console.log;
      console.log = function(message) {
        if (typeof message === 'string' && message.startsWith('__libro:page-tool:capture-area:')) {
          console.log = originalLog;
          resolve({payload:JSON.parse(message.slice('__libro:page-tool:capture-area:'.length)),
            clean:!document.querySelector('[popover]')});
        } else originalLog.apply(console, arguments);
      };
      const button = document.querySelector('button');
      window.__libroSetPageToolMode('annotate');
      button.dispatchEvent(new PointerEvent('pointerover', {bubbles:true}));
      button.click();
    })`);
    assert.equal(selection.clean, true, 'capture must happen after outline and prompt removal');
    assert.equal(selection.payload.kind, 'element');
    assert.ok(selection.payload.element.selector.endsWith('button'));
    assert.deepEqual(Object.keys(selection.payload.element).sort(), ['selector','viewportRect']);
    assert.ok(selection.payload.element.viewportRect.width > 0);
    await win.webContents.executeJavaScript(`document.querySelector('dialog').close(); document.body.style.height = '1800px'; scrollTo(0, 300);`);
    const pageSelection = await win.webContents.executeJavaScript(`new Promise(resolve => {
      const originalLog = console.log;
      console.log = function(message) {
        if (typeof message === 'string' && message.startsWith('__libro:page-tool:capture-area:')) {
          console.log = originalLog;
          resolve({payload:JSON.parse(message.slice('__libro:page-tool:capture-area:'.length)), clean:!document.querySelector('[popover]')});
        } else originalLog.apply(console, arguments);
      };
      window.__libroSetPageToolMode('page');
    })`);
    assert.equal(pageSelection.payload.kind, 'page');
    assert.equal(pageSelection.clean, true);
    const filename = await require('./page-area').capturePageArea(win.webContents, {fullPage:true}, app.getPath('temp'));
    try {
      const image = require('electron').nativeImage.createFromPath(filename);
      assert.ok(image.getSize().height >= 1800, 'captures content below the viewport');
      const result = await win.webContents.executeJavaScript(`(() => {
        window.__libroPageToolPromptOpen({kind:'page',screenshot:${JSON.stringify(filename)}},'http://example.test');
        const panel = document.querySelector('[popover]');
        return {label:panel.getAttribute('aria-label'), title:panel.shadowRoot.querySelector('.header span').textContent, focused:panel.shadowRoot.activeElement === panel.shadowRoot.querySelector('textarea'), scroll:scrollY};
      })()`);
      assert.equal(result.label, 'Describe whole page');
      assert.equal(result.title, 'Describe this page');
      assert.equal(result.focused, true);
      assert.equal(result.scroll, 300, 'capture preserves scroll position');
      const pasted = await win.webContents.executeJavaScript(`(async () => {
        const panel = document.querySelector('[popover]');
        const input = panel.shadowRoot.querySelector('textarea');
        const canvas = document.createElement('canvas'); canvas.width = 20; canvas.height = 20;
        canvas.getContext('2d').fillRect(0,0,20,20);
        const blob = await new Promise(resolve => canvas.toBlob(resolve));
        const clipboard = new DataTransfer();
        clipboard.items.add(new File([blob], 'first.png', {type:'image/png'}));
        clipboard.items.add(new File([blob], 'second.png', {type:'image/png'}));
        input.dispatchEvent(new ClipboardEvent('paste',{clipboardData:clipboard,bubbles:true,cancelable:true}));
        await new Promise((resolve,reject) => {
          let attempts = 0;
          const timer = setInterval(() => {
            if (panel.shadowRoot.querySelectorAll('.attachment').length === 2) { clearInterval(timer); resolve(); }
            else if (++attempts > 100) { clearInterval(timer); reject(new Error('Paste previews missing')); }
          }, 10);
        });
        panel.shadowRoot.querySelector('.attachment button').click();
        return panel.shadowRoot.querySelectorAll('.attachment').length;
      })()`);
      assert.equal(pasted, 1, 'pasted images can be removed');
      const prompt = await win.webContents.executeJavaScript(`(() => {
        const panel = document.querySelector('[popover]');
        const input = panel.shadowRoot.querySelector('textarea');
        const originalLog = console.log;
        let selection;
        console.log = function(message) {
          if (typeof message === 'string' && message.startsWith('__libro:page-tool:selection:')) selection = JSON.parse(message.slice('__libro:page-tool:selection:'.length));
        };
        input.value = 'First line'; input.setSelectionRange(input.value.length,input.value.length);
        input.dispatchEvent(new KeyboardEvent('keydown',{key:'j',ctrlKey:true,bubbles:true,cancelable:true,composed:true}));
        input.setRangeText('Second line',input.selectionStart,input.selectionEnd,'end');
        input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true,composed:true}));
        const wraps = getComputedStyle(input).whiteSpace === 'pre-wrap';
        input.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true,composed:true}));
        console.log = originalLog;
        return {selection, closed:!panel.isConnected, wraps, multiline:input.rows === 3};
      })()`);
      assert.equal(prompt.selection.request, 'First line\nSecond line');
      assert.equal(prompt.selection.kind, 'page');
      assert.equal(prompt.selection.images.length, 1);
      const saved = await require('./page-area').savePageToolImages(prompt.selection.images, app.getPath('temp'));
      try { assert.equal(require('electron').nativeImage.createFromPath(saved[0]).getSize().width, 20); }
      finally { fs.rmSync(path.dirname(saved[0]), {recursive:true,force:true}); }
      await assert.rejects(require('./page-area').savePageToolImages(['data:image/png;base64,invalid'], app.getPath('temp')));
      assert.equal(prompt.closed, true, 'Escape cancels the prompt');
      assert.equal(prompt.wraps, true);
      assert.equal(prompt.multiline, true);
    } finally { fs.rmSync(path.dirname(filename), {recursive:true,force:true}); }
    console.log('PASS: drag outline, prompt layers, clean element capture, and whole-page annotation');
  } finally { win.destroy(); app.quit(); }
}).catch(error => {console.error(error);app.exit(1)});
