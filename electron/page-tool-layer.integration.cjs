// Run with: xvfb-run -a node_modules/.bin/electron --no-sandbox --ozone-platform=x11 electron/page-tool-layer.integration.cjs
const {app, BrowserWindow} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  const win = new BrowserWindow({width:540,height:540,show:false});
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
        const input = panel.shadowRoot.querySelector('input');
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
    console.log('PASS: drag outline, prompt layers, and clean element capture payload');
  } finally { win.destroy(); app.quit(); }
}).catch(error => {console.error(error);app.exit(1)});
