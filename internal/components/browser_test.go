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
  for (const key of ['/', 'n', 'N', 'p', 'Escape', 'b', 'f', 'y', 'c', 'Enter']) {
    assert.equal(press(key), false, key + ' must pass through');
  }
  assert.equal(press('j'), true);
  assert.equal(scrolls, 1);
  assert.equal(press('o'), true);
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
