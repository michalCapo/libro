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
    console.log('PASS: prompt is above modal, focused, clickable, and closes without closing page popup');
  } finally { win.destroy(); app.quit(); }
}).catch(error => {console.error(error);app.exit(1)});
