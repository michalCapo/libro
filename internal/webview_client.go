package libro

// webviewClientJS returns the JavaScript that manages Electron webview elements.
// It initializes webview tags, handles navigation events, and provides
// back/forward/reload/navigate functions via the webview DOM API.
func webviewClientJS() string {
	return webviewClientScript
}

const webviewClientScript = `
(function(){
window.__libroWebviews = window.__libroWebviews || {};
var ready = {};       // appID -> true when dom-ready has fired
var queued = {};      // appID -> [fn, fn, ...] calls waiting for dom-ready
var initialized = {};

function whenReady(appID, fn) {
	if (ready[appID]) { fn(); return; }
	if (!queued[appID]) queued[appID] = [];
	queued[appID].push(fn);
}

function initWebview(wv) {
	var appID = wv.getAttribute('data-webview-app');
	if (!appID || initialized[appID]) return;
	initialized[appID] = true;

	window.__libroWebviews[appID] = wv;

	wv.addEventListener('dom-ready', function() {
		ready[appID] = true;
		var q = queued[appID];
		if (q) { queued[appID] = null; q.forEach(function(fn){ fn(); }); }
	});

	// Update URL bar on navigation
	wv.addEventListener('did-navigate', function(e) {
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
	});
	wv.addEventListener('did-navigate-in-page', function(e) {
		if (!e.isMainFrame) return;
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
	});

	// Loading indicator removal
	var loading = wv.parentNode && wv.parentNode.querySelector('[data-webview-loading]');
	if (loading) {
		wv.addEventListener('did-finish-load', function() {
			if (loading.parentNode) loading.remove();
		});
		wv.addEventListener('did-fail-load', function() {
			if (loading.parentNode) loading.remove();
		});
	}

	// Keyboard shortcut interception is handled in the main process
	// (electron/main.js) via webContents before-input-event, which is
	// more reliable than the renderer-side <webview> DOM event.
}

function initAll() {
	document.querySelectorAll('webview[data-webview-app]').forEach(initWebview);
}

initAll();

var bodyObserver = new MutationObserver(function() { initAll(); });
bodyObserver.observe(document.body, { childList: true, subtree: true });

// Cleanup when webview removed from DOM
var cleanupObserver = new MutationObserver(function(mutations) {
	mutations.forEach(function(m) {
		m.removedNodes.forEach(function(node) {
			if (node.nodeType !== 1) return;
			var wvs = node.querySelectorAll ? node.querySelectorAll('webview[data-webview-app]') : [];
			wvs.forEach(function(wv) {
				var id = wv.getAttribute('data-webview-app');
				if (id) {
					delete window.__libroWebviews[id];
					delete initialized[id];
					delete ready[id];
					delete queued[id];
				}
			});
			if (node.tagName === 'WEBVIEW' && node.getAttribute('data-webview-app')) {
				var id = node.getAttribute('data-webview-app');
				delete window.__libroWebviews[id];
				delete initialized[id];
				delete ready[id];
				delete queued[id];
			}
		});
	});
});
cleanupObserver.observe(document.body, { childList: true, subtree: true });

// Global helpers — safe to call before dom-ready (calls are queued)
window.__libroWvBack = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv) return;
	whenReady(appID, function() { if (wv.canGoBack()) wv.goBack(); });
};
window.__libroWvForward = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv) return;
	whenReady(appID, function() { if (wv.canGoForward()) wv.goForward(); });
};
window.__libroWvReload = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv) return;
	whenReady(appID, function() { wv.reload(); });
};
window.__libroWvNavigate = function(appID, url) {
	var wv = window.__libroWebviews[appID];
	if (!wv) return;
	if (ready[appID]) {
		wv.loadURL(url);
	} else {
		// Not ready yet — set src attribute to trigger initial load
		wv.setAttribute('src', url);
	}
};
})();
`
