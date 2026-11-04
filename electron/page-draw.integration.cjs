// Run with: node_modules/.bin/electron --no-sandbox --ozone-platform=x11 electron/page-draw.integration.cjs
// Uses a real webview and real mouse input. Needs a display, such as xvfb-run -a.
const {app, BrowserWindow, ipcMain, nativeImage, webContents} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const {capturePageArea, pageToolImages} = require('./page-area');
const page = `<body style="margin:0;background:#fff;font:15px sans-serif"><h1>Orders</h1>${'<p>Row of real page text with small letters</p>'.repeat(12)}<button id=b style="position:fixed;left:0;top:0;width:600px;height:400px;opacity:0" onclick="document.title='clicked'"></button></body>`;
const wait = ms => new Promise(done => setTimeout(done, ms));
// Counts pixels a person could see change. Scrollbar arrows redraw a few shades off.
function differentPixels(a, b, area) {
  const left = a.crop(area).toBitmap(), right = b.crop(area).toBitmap();
  let count = 0;
  for (let i = 0; i < left.length; i += 4) if (Math.abs(left[i] - right[i]) + Math.abs(left[i + 1] - right[i + 1]) + Math.abs(left[i + 2] - right[i + 2]) > 12) count++;
  return count;
}
function isRed(image, x, y) {
  const [b, g, r] = image.crop({x, y, width:1, height:1}).toBitmap();
  return r > 180 && g < 90 && b < 110;
}
app.commandLine.appendSwitch('force-device-scale-factor', process.env.DRAW_SCALE || '1');
app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  ipcMain.handle('libro-capture-page-area', (event, id, area) => capturePageArea(webContents.fromId(id), area));
  ipcMain.handle('libro-save-page-tool-images', (event, id, images) => pageToolImages(images));
  const win = new BrowserWindow({width:800, height:600, show:true, useContentSize:true, webPreferences:{webviewTag:true, backgroundThrottling:false, preload:path.join(__dirname, 'preload.js')}});
  try {
    await win.loadURL('data:text/html,' + encodeURIComponent('<!doctype html><body style="margin:0"><style>:root{--ws-popup:#fff;--ws-fg:#111;--ws-field:#eee;--ws-hover:#ddd;--ws-accent:#24d}</style><div id=wrap style="position:relative;width:600px;height:400px"><webview id=wv style="display:flex;width:100%;height:100%" src="data:text/html,' + encodeURIComponent(page).replace(/'/g, '%27').replace(/"/g, '%22') + '"></webview></div></body>'));
    await win.webContents.executeJavaScript(`new Promise(done => document.getElementById('wv').addEventListener('did-finish-load', done, {once:true}))`);
    await win.webContents.insertCSS(fs.readFileSync(path.join(__dirname, '../internal/workspace.css'), 'utf8').split('\n').filter(line => line.startsWith('.ws-page-draw')).join('\n'));
    await win.webContents.executeJavaScript(fs.readFileSync(path.join(__dirname, '../internal/page_draw.js'), 'utf8') + `;
      window.saved = []; window.prompts = [];
      window.__libroSavePageToolImages = async (id, images) => { const paths = images.map(image => '/tmp/' + window.saved.length + '-' + (window.saved.push(image) - 1) + '.png'); return paths; };
      window.__libroPageToolAgent = () => 'agent';
      // Workspace shortcuts listen on the window after the editor has loaded.
      window.addEventListener('keydown', event => { if (event.ctrlKey && event.key === 'Enter') window.shortcutLeaked = true; }, true);
      window.__libroSendPageToolPrompt = prompt => { window.prompts.push(prompt); return true; };
      true`);
    await wait(500);
    const guest = webContents.getAllWebContents().find(item => item.getType() === 'webview');
    const scale = Number(process.env.DRAW_SCALE || 1);
    const page_area = {x:0, y:0, width:Math.round(600 * scale), height:Math.round(300 * scale)};
    const before = await win.webContents.capturePage();
    const picture = await capturePageArea(guest, {viewport:true});
    const openEditor = () => win.webContents.executeJavaScript(`window.__libroPageDraw.open({appID:'a', guest:document.getElementById('wv'), picture:${JSON.stringify(picture)}, url:${JSON.stringify(guest.getURL())}, wanted:() => true, onClose(){ window.editorClosed = true; }})`);
    await openEditor();
    await wait(300);
    const after = await win.webContents.capturePage();
    assert.equal(differentPixels(before, after, page_area), 0, 'the picture must look exactly like the page');

    const mouse = async (x, y, type, extra = {}) => { win.webContents.sendInputEvent({type, x, y, button:'left', clickCount:1, ...extra}); await wait(40); };
    const drag = async (x1, y1, x2, y2) => { await mouse(x1, y1, 'mouseMove'); await mouse(x1, y1, 'mouseDown'); await mouse(x2, y2, 'mouseMove'); await mouse(x2, y2, 'mouseUp'); };
    const key = async (keyCode, modifiers = []) => { win.webContents.sendInputEvent({type:'keyDown', keyCode, modifiers}); win.webContents.sendInputEvent({type:'keyUp', keyCode, modifiers}); await wait(40); };
    const sent = async () => { for (let i = 0; i < 100 && !await win.webContents.executeJavaScript('window.editorClosed'); i++) await wait(100); };
    // P then Enter sends the whole page with nothing written or drawn.
    await key('Return');
    await sent();
    const whole = await win.webContents.executeJavaScript('({saved:window.saved, prompts:window.prompts, closed:window.editorClosed})');
    assert.equal(whole.closed, true, 'sending closes the editor');
    assert.equal(whole.saved.length, 1);
    const fullPage = nativeImage.createFromDataURL(whole.saved[0]).getSize();
    assert.ok(fullPage.height > 400, 'the whole page goes, not only the visible part');
    assert.match(whole.prompts[0], /Whole-page screenshot: /);
    assert.doesNotMatch(whole.prompts[0], /User request|Drawings/);
    assert.equal(differentPixels(before, await win.webContents.capturePage(), page_area), 0, 'the live page is back and unchanged');
    await win.webContents.executeJavaScript('window.saved = []; window.prompts = []; window.editorClosed = false');
    await openEditor();
    await wait(200);

    await drag(50, 60, 200, 60);
    await key('r');
    await drag(250, 100, 400, 200);
    await key('l');
    await drag(50, 250, 300, 250);
    await key('Delete');
    await key('z', ['control']);
    await mouse(320, 100, 'mouseMove'); await mouse(320, 100, 'mouseDown'); await mouse(320, 100, 'mouseUp');
    const typed = await win.webContents.executeJavaScript(`document.activeElement.tagName`);
    assert.equal(typed, 'TEXTAREA', 'clicking a shape opens its description');
    win.webContents.insertText('Make this bigger');
    await wait(60);
    await key('Return');
    assert.equal(await guest.executeJavaScript('document.title'), '', 'the page must not receive drawing clicks');
    // Ctrl+Enter reaches the editor first, and nothing can change during the send.
    await key('Return', ['control']);
    await win.webContents.executeJavaScript(`document.querySelector('.ws-page-draw-action').click(); document.querySelector('.ws-page-draw-tool[aria-label=Undo]').click(); true`);
    await sent();
    assert.equal(await win.webContents.executeJavaScript('!!window.shortcutLeaked'), false, 'Ctrl+Enter must not open the terminal');
    const [drawnData, cleanData, wholeData] = await win.webContents.executeJavaScript('window.saved');
    assert.ok(wholeData, 'the full-page capture must finish under the picture');
    assert.ok(nativeImage.createFromDataURL(wholeData).getSize().height > 400);
    const drawn = nativeImage.createFromDataURL(drawnData), clean = nativeImage.createFromDataURL(cleanData);
    assert.deepEqual(drawn.getSize(), clean.getSize());
    assert.ok(isRed(drawn, Math.round(120 * scale), Math.round(60 * scale)), 'arrow is in the sent picture');
    assert.ok(isRed(drawn, Math.round(400 * scale), Math.round(150 * scale)), 'rectangle is in the sent picture');
    assert.ok(isRed(drawn, Math.round(150 * scale), Math.round(250 * scale)), 'undo brought the line back');
    assert.ok(!isRed(clean, Math.round(120 * scale), Math.round(60 * scale)), 'clean picture has no drawings');
    const [prompt] = await win.webContents.executeJavaScript('window.prompts');
    assert.match(prompt, new RegExp('1\\. Arrow from \\(' + Math.round(50 * scale) + ', ' + Math.round(60 * scale) + '\\) to \\(' + Math.round(200 * scale) + ', ' + Math.round(60 * scale) + '\\)\\n'), 'a shape without a description is sent');
    assert.match(prompt, /2\. Rectangle from .*: Make this bigger\n/);
    assert.match(prompt, /3\. Line from /);
    assert.equal(await win.webContents.executeJavaScript('window.editorClosed'), true);
    console.log('page drawing passed');
    app.exit(0);
  } catch (err) {
    console.error(err);
    app.exit(1);
  }
});
