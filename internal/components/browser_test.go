package components

import (
	"os/exec"
	"strings"
	"testing"
)

func TestBrowserShortcutKeyPassthrough(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert = require('node:assert/strict');
const vm = require('node:vm');
let script = '';
process.stdin.on('data', data => script += data);
process.stdin.on('end', () => {
  const start = script.indexOf('var browserShortcutsScript =');
  const end = script.indexOf('window.__libroOpenConsole =', start);
  assert.ok(start >= 0 && end > start);
  const context = { window: {}, document: {
    activeElement: null,
    addEventListener(name, fn) { if (name === 'keydown') this.keydown = fn; },
  }, console: { log() {} } };
  context.window.addEventListener = () => {};
  let scrolls = 0;
  context.window.scrollBy = () => scrolls++;
  vm.runInNewContext(script.slice(start, end), context);
  vm.runInNewContext(context.browserShortcutsScript, context);
  function press(key) {
    let handled = false;
    context.document.keydown({ key,
      preventDefault() { handled = true; },
      stopPropagation() { handled = true; },
    });
    return handled;
  }
  for (const key of ['/', 'n', 'N', 'Escape', 'b', 'f', 'y', 'c', 'Enter']) {
    assert.equal(press(key), false, key + ' must pass through');
  }
  assert.equal(press('j'), true);
  assert.equal(scrolls, 1);
  assert.equal(press('o'), true);
  assert.equal(press('p'), true);
  let blurred = false;
  context.document.activeElement = { tagName: 'TEXTAREA', blur() { blurred = true; } };
  assert.equal(press('Escape'), false);
  assert.equal(blurred, false);
  assert.equal(press('j'), false);
  context.document.activeElement = null;
  context.window.__libroBrowserMode = 'insert';
  assert.equal(press('j'), false);
});
`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser shortcut passthrough failed: %v\n%s", err, output)
	}
}

func TestBrowserRuntimeNavigation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert = require('node:assert/strict');
const vm = require('node:vm');
let script = '';
process.stdin.on('data', data => script += data);
process.stdin.on('end', () => {
  for (const electron of [false, true]) {
    const listeners = {};
    const observers = [];
    const webview = {
      style: {}, getAttribute: name => name === 'data-webview-app' ? 'test' : null,
      addEventListener: (name, fn) => { listeners[name] = fn; },
      closest: () => null, setAttribute(name, value) { this[name] = value; },
      focus() {},
    };
    const notice = { style: { display: 'none' } };
    const link = { setAttribute(name, value) { this[name] = value; } };
    const loading = { remove() { this.removed = true; } };
    const iframe = {
      closest: () => ({ querySelector: () => loading }),
      style: {}, attrs: {}, hasAttribute(name) { return name in this.attrs; },
      getAttribute(name) { return this.attrs[name]; },
      setAttribute(name, value) { this.attrs[name] = value; },
      contentWindow: { location: { reload() { iframe.reloaded = true; } },
        history: { back() { iframe.back = true; }, forward() { iframe.forward = true; } } },
    };
    let currentWebview = webview;
    const document = {
      body: {}, getElementById: () => null,
      querySelector: selector => selector.startsWith('iframe[') ? iframe
        : selector.startsWith('[data-browser-fallback-notice=') ? notice
        : selector.startsWith('[data-browser-external-link=') ? link : null,
      querySelectorAll: selector => selector.startsWith('webview[') ? [currentWebview] : selector.startsWith('iframe[') ? [iframe] : [],
    };
    const window = { addEventListener() {}, focus() {} };
    if (electron) window.libroElectron = {};
    const context = { window, document, setTimeout() {}, MutationObserver: class { constructor(fn) { observers.push(fn); } observe() {} } };
    vm.runInNewContext(script, context);
    assert.equal(notice.style.display, 'none');
    assert.ok(!loading.removed);
    const url = 'http://localhost:3000/preview';
    window.__libroWvNavigate('test', url);
    if (electron) {
      assert.equal(window.__libroWebviews.test, webview);
      assert.equal(webview.src, url);
      assert.equal(typeof listeners['dom-ready'], 'function');
      assert.equal(iframe.style.display, 'none');
      assert.equal(notice.style.display, 'none');
      // The init observer runs before cleanup when hydration replaces a guest.
      currentWebview = {...webview};
      observers[0]();
      assert.equal(window.__libroWebviews.test, currentWebview);
      const removedFrame = {nodeType: 1, querySelectorAll: () => [webview]};
      observers[1]([{removedNodes: [removedFrame]}]);
      assert.equal(window.__libroWebviews.test, currentWebview);
      window.__libroWvNavigate('test', url + '/replacement');
      assert.equal(currentWebview.src, url + '/replacement');
      currentWebview.nodeType = 1;
      currentWebview.tagName = 'WEBVIEW';
      currentWebview.isConnected = false;
      observers[1]([{removedNodes: [currentWebview]}]);
      assert.equal(window.__libroWebviews.test, undefined);
    } else {
      assert.equal(window.__libroWebviews.test, undefined);
      assert.equal(iframe.attrs.src, url);
      assert.equal(iframe.style.display, 'block');
      assert.equal(notice.style.display, 'block');
      assert.equal(link.href, url);
      assert.ok(loading.removed);
      window.__libroWvReload('test');
      window.__libroWvBack('test');
      window.__libroWvForward('test');
      assert.ok(iframe.reloaded && iframe.back && iframe.forward);
    }
  }
});
`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser navigation failed: %v\n%s", err, output)
	}
}

func TestBrowserFocusPreservesURLPopup(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert = require('node:assert/strict'), vm = require('node:vm');
let script = '';
process.stdin.on('data', data => script += data);
process.stdin.on('end', () => {
  for (const name of ['focusIfSelected', 'refocusWebview']) {
    const start = script.indexOf('function ' + name + '(');
    const end = script.indexOf('\n}', start) + 2;
    let popupOpen = false, focused = 'input';
    const timers = [];
    const window = { __libroSelectedApp: 'browser', focus() {} };
    const document = { querySelector() { return popupOpen ? {} : null; } };
    const context = { window, document, setTimeout(fn) { timers.push(fn); } };
    vm.runInNewContext(script.slice(start, end), context);
    const webview = { focus() { focused = 'webview'; } };
    context[name]('browser', webview);
    assert.equal(focused, 'webview');
    popupOpen = true;
    focused = 'input';
    timers.splice(0).forEach(fn => fn());
    assert.equal(focused, 'input', name + ' retries must preserve popup focus');
    context[name]('browser', webview);
    assert.equal(focused, 'input', name + ' must preserve popup focus on dom-ready');
    popupOpen = false;
    context[name]('browser', webview);
    assert.equal(focused, 'webview', name + ' must restore focus after closing');
  }
});`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser popup focus: %v\n%s", err, output)
	}
}

func TestDevtoolsBoundsExcludeHeader(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert = require('node:assert/strict');
const vm = require('node:vm');
let script = '';
process.stdin.on('data', data => script += data);
process.stdin.on('end', () => {
  const start = script.indexOf('function devtoolsPanelBounds(');
  const end = script.indexOf('function sameDevtoolsBounds(', start);
  const context = {
    window: { libroElectron: { getZoomFactor: () => 1.5 } },
    document: { getElementById(id) {
      assert.equal(id, 'devtools-host-test', 'native view must exclude the close header');
      return { getBoundingClientRect: () => ({ left: 20, top: 132, width: 600, height: 288 }) };
    } },
  };
  vm.runInNewContext(script.slice(start, end), context);
  const bounds = context.devtoolsPanelBounds('test');
  assert.deepEqual(JSON.parse(JSON.stringify(bounds)), { x: 30, y: 198, width: 900, height: 432 });
  context.document.getElementById = () => null;
  assert.equal(context.devtoolsPanelBounds('test'), null);
});
`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("DevTools bounds failed: %v\n%s", err, output)
	}
}

func TestPageAreaScreenshotPrompt(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert = require('node:assert/strict'), vm = require('node:vm');
let script = '';
process.stdin.on('data', data => script += data);
process.stdin.on('end', async () => {
  let opened, sent, toast;
  const guest = { getWebContentsId: () => 7, executeJavaScript: async js => { opened = js; } };
  const context = {
    window: {
      libroElectron: { capturePageArea: async (id, area) => { assert.equal(id, 7); assert.equal(area.width, 80); return '/tmp/selection.png'; } },
      __libroSendPageToolPrompt: prompt => { sent = prompt; return true; },
      __libroShowToast: title => { toast = title; },
    },
    document: {}, pageToolWebview: () => guest, pageToolButtonState() {},
  };
  vm.runInNewContext(script.slice(script.indexOf('function pageToolURL('), script.indexOf('function currentAppWidth(')), context);
  const payload = { kind: 'area', url: 'http://localhost:3000/payments', area: { x: 10, y: 20, width: 80, height: 40 } };
  await context.receivePageToolMessage('app', 'capture-area', JSON.stringify(payload));
  assert.ok(opened.includes('/tmp/selection.png'));
  assert.equal(sent, undefined);
  payload.screenshot = '/tmp/selection.png'; payload.request = 'Move the button';
  payload.area.text = 'unwanted center text';
  payload.area.elements = [{tag: 'button', selector: '#unwanted', html: '<button>unwanted HTML</button>'}];
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(payload));
  for (const text of [payload.url, payload.request, payload.screenshot]) assert.ok(sent.includes(text));
  for (const text of ['unwanted', 'Elements intersecting', 'Viewport coordinates', 'Page coordinates']) assert.ok(!sent.includes(text));
  const elementPayload = {
    kind: 'element', url: payload.url, request: 'Move this element',
    element: { selector: 'main > button.save', tag: 'button', id: 'unwanted-id', classes: 'unwanted-class',
      text: 'unwanted text', html: '<button>unwanted HTML</button>', attributes: {title: 'unwanted attribute'},
      viewportRect: {x: 10, y: 20, width: 80, height: 40} },
  };
  sent = undefined;
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(elementPayload));
  assert.equal(sent, undefined, 'element selection requires its screenshot');
  await context.receivePageToolMessage('app', 'capture-area', JSON.stringify(elementPayload));
  assert.ok(opened.includes('/tmp/selection.png'));
  assert.ok(opened.includes(elementPayload.element.selector));
  elementPayload.screenshot = '/tmp/selection.png';
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(elementPayload));
  for (const text of [elementPayload.url, elementPayload.request, elementPayload.element.selector, elementPayload.screenshot]) assert.ok(sent.includes(text));
  for (const text of ['unwanted', 'Outer HTML', 'Attributes:', 'Viewport rectangle', '- Tag:', '- ID:', '- Classes:']) assert.ok(!sent.includes(text));
  const pagePayload = {kind: 'page', url: payload.url, request: 'Improve the whole page'};
  sent = undefined;
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(pagePayload));
  assert.equal(sent, undefined, 'whole page annotation requires its screenshot');
  context.window.libroElectron.capturePageArea = async (id, area) => {
    assert.equal(id, 7); assert.equal(area.fullPage, true); return '/tmp/page.png';
  };
  await context.receivePageToolMessage('app', 'capture-area', JSON.stringify(pagePayload));
  assert.ok(opened.includes('/tmp/page.png'));
  pagePayload.screenshot = '/tmp/page.png';
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(pagePayload));
  for (const text of [pagePayload.url, pagePayload.request, pagePayload.screenshot, 'Whole-page screenshot:']) assert.ok(sent.includes(text));
  assert.ok(!sent.includes('Target element path:'));
  pagePayload.images = ['data:image/png;base64,example'];
  context.window.libroElectron.savePageToolImages = async (id, images) => {
    assert.equal(id, 7); assert.equal(images.length, 1); return ['/tmp/pasted.png'];
  };
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(pagePayload));
  assert.ok(sent.includes('Additional image: "/tmp/pasted.png"'));
  sent = undefined;
  context.window.libroElectron.savePageToolImages = async () => { throw new Error('failed'); };
  await context.receivePageToolMessage('app', 'selection', JSON.stringify(pagePayload));
  assert.equal(sent, undefined, 'attachment failure keeps the prompt for retry');
  assert.ok(opened.includes('Could not attach images'));

  opened = undefined;
  context.window.libroElectron.capturePageArea = async () => { throw new Error('failed'); };
  await context.receivePageToolMessage('app', 'capture-area', JSON.stringify(payload));
  assert.equal(opened, undefined);
  assert.equal(toast, 'Screenshot failed');
  await context.receivePageToolMessage('app', 'capture-area', JSON.stringify(elementPayload));
  assert.equal(opened, undefined);
  assert.equal(toast, 'Screenshot failed');
}).on('error', error => { throw error; });`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("page area screenshot failed: %v\n%s", err, output)
	}
}

func TestBrowserAgentPauseState(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert=require('node:assert/strict'),vm=require('node:vm');
let source='';process.stdin.on('data',d=>source+=d);process.stdin.on('end',async()=>{
 const icon={textContent:'pause'},attrs={};
 const button={setAttribute:(k,v)=>attrs[k]=v,querySelector:()=>icon};
 let paused=false,listener;const setting={value:'on'};let applied;
 const window={libroElectron:{setBrowserControlEnabled:async enabled=>{applied=enabled;return {enabled,paused:!enabled}},onBrowserControlState:fn=>listener=fn,browserControlState:async action=>{if(action==='toggle')paused=!paused;if(action==='stop')paused=true;return {paused}}}};
 const start=source.indexOf('var agentControlPaused =');
 const end=source.indexOf('// --- Browser shortcuts',start);
 vm.runInNewContext(source.slice(start,end),{window,document:{querySelectorAll:()=>[button],getElementById:()=>setting}});
 await Promise.resolve();
 assert.equal(attrs['aria-label'],'Pause agent browser control');
 window.__libroBrowserControlPause('toggle');await Promise.resolve();
 assert.equal(attrs['aria-label'],'Resume agent browser control');assert.equal(attrs['aria-pressed'],'true');assert.equal(icon.textContent,'play_arrow');
 window.__libroBrowserControlPause('toggle');await Promise.resolve();
 assert.equal(icon.textContent,'pause');
 listener({paused:true});assert.equal(icon.textContent,'play_arrow');
 window.__libroBrowserControlPause('stop');await Promise.resolve();assert.equal(paused,true);
 window.__libroApplyBrowserControlSetting(false);await Promise.resolve();
 assert.equal(applied,false);assert.equal(button.disabled,true);assert.equal(setting.value,'off');assert.equal(attrs['aria-label'],'Agent browser control is off in Settings');
 window.__libroApplyBrowserControlSetting(true);await Promise.resolve();
 assert.equal(button.disabled,false);assert.equal(setting.value,'on');
});`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser pause controls: %v\n%s", err, output)
	}
}

func TestBrowserViewportFits(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	const harness = `
const assert = require('node:assert/strict'), vm = require('node:vm');
let source = '';
process.stdin.on('data', data => source += data);
process.stdin.on('end', () => {
  const element = () => ({style: {}, getAttribute: () => 'original', setAttribute(name, value) { this.restored = value; }});
  const frame = element(), content = element(), guest = element();
  const host = guest.parentElement = {clientWidth: 320, clientHeight: 400};
  let resized, disconnected = false, level = 0;
  guest.getZoomLevel = () => level; guest.setZoomLevel = value => level = value;
  const context = {
    window: {libroElectron: {}, __libroWebviews: {test: guest}},
    document: {querySelector: selector => selector.includes('data-app-content') ? content : frame},
    mobileViewState: {}, mobileViewportOrientation: {},
    mobileSizes: {sm: {width:480,height:896}, md: {width:640,height:932}, xl: {width:720,height:1280}},
    currentAppWidth: () => 'lg', whenReady: (id, fn) => fn(),
    ResizeObserver: class {constructor(fn) {resized = fn;} observe() {} disconnect() {disconnected = true;}},
  };
  vm.runInNewContext(source.slice(source.indexOf('function applyMobileView('), source.indexOf('window.__libroToggleSelectedBrowserMobile')), context);
  vm.runInNewContext(source.slice(source.indexOf('window.__libroWvZoom ='), source.indexOf('window.__libroOpenNewTab =')), context);
  function fits(width, height) {
    assert.equal(guest.style.width, width + 'px');
    assert.equal(guest.style.height, height + 'px');
    const scale = Number(guest.style.transform.slice(6, -1));
    assert.ok(width * scale <= host.clientWidth + 1e-6);
    assert.ok(height * scale <= host.clientHeight + 1e-6);
    assert.ok(parseFloat(guest.style.top) >= -1e-6);
    assert.ok(parseFloat(guest.style.left) >= -1e-6);
  }
  for (const mode of ['sm', 'md', 'xl']) {
    for (const orientation of ['portrait', 'landscape']) {
      context.applyMobileView('test', mode, orientation);
      const size = context.mobileSizes[mode];
      const [width, height] = orientation === 'portrait' ? [size.width, size.height] : [size.height, size.width];
      fits(width, height);
      for (const step of [-1, 1, 1, 0]) {context.window.__libroWvZoom('test', step); fits(width, height);}
      host.clientWidth = 240; host.clientHeight = 180; resized(); fits(width, height);
      host.clientWidth = 1200; host.clientHeight = 900; resized(); fits(width, height);
    }
  }
  context.applyMobileView('test', 'normal');
  assert.ok(disconnected);
  for (const el of [frame, content, guest]) assert.equal(el.restored, 'original');
  assert.equal(context.mobileViewState.test, undefined);
});`
	cmd := exec.Command(node, "-e", harness)
	cmd.Stdin = strings.NewReader(BrowserJS())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("viewport fit: %v\n%s", err, output)
	}
}
