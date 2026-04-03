package libro

// webviewClientJS returns the JavaScript that manages Electron webview elements.
// It initializes webview tags, handles navigation events, and provides
// back/forward/reload/navigate functions via the webview DOM API.
// It also injects browser-mode keyboard shortcuts (j/k/h/l/b/f/Enter)
// into webview guest pages and provides find-in-page support (/, n, p).
func webviewClientJS() string {
	return webviewClientScript
}

const webviewClientScript = `
(function(){
window.__libroWebviews = window.__libroWebviews || {};
var ready = {};       // appID -> true when dom-ready has fired
var queued = {};      // appID -> [fn, fn, ...] calls waiting for dom-ready
var initialized = {};

// --- Browser shortcuts script injected into webview guest pages ---
var browserShortcutsScript = '(' + function(){
	if(window.__libroBrowserShortcuts) return;
	window.__libroBrowserShortcuts = true;
	document.addEventListener('keydown', function(e) {
		if(e.metaKey || e.ctrlKey || e.altKey) return;
		var ae = document.activeElement;
		var tag = ae ? ae.tagName.toUpperCase() : '';
		if(tag==='INPUT'||tag==='TEXTAREA'||tag==='SELECT'||(ae&&ae.isContentEditable)) return;
		var handled = true;
		switch(e.key) {
			case 'g': window.scrollTo({top: 0, behavior: 'smooth'}); break;
			case 'G': window.scrollTo({top: document.documentElement.scrollHeight, behavior: 'smooth'}); break;
			case 'j': window.scrollBy({top: 800, behavior: 'smooth'}); break;
			case 'k': window.scrollBy({top: -800, behavior: 'smooth'}); break;
			case 'h': window.scrollBy({left: -480, behavior: 'smooth'}); break;
			case 'l': window.scrollBy({left: 480, behavior: 'smooth'}); break;
			case 'b': history.back(); break;
			case 'f': history.forward(); break;
			case '/': console.log('__libro:search'); break;
			case 'n': console.log('__libro:findnext'); break;
			case 'N': console.log('__libro:findprev'); break;
			case 'p': console.log('__libro:findprev'); break;
			case 'Escape': console.log('__libro:searchclear'); break;
			case 'Enter':
				console.log('__libro:enter');
				break;
			default: handled = false;
		}
		if(handled) { e.preventDefault(); e.stopPropagation(); }
	}, true);
} + ')()';

// --- Console error/warning counts per webview ---
var consoleCounts = {}; // appID -> {errors, warnings}

function updateConsoleBadges(appID) {
	var c = consoleCounts[appID] || {errors: 0, warnings: 0};
	var errEl = document.getElementById('devtools-errors-' + appID);
	var warnEl = document.getElementById('devtools-warnings-' + appID);
	var wrapEl = document.getElementById('devtools-wrap-' + appID);
	if (errEl) {
		if (c.errors > 0) {
			errEl.style.display = 'inline';
			errEl.textContent = c.errors;
			if (wrapEl) wrapEl.style.opacity = '1';
		} else {
			errEl.style.display = 'none';
		}
	}
	if (warnEl) {
		if (c.warnings > 0) {
			warnEl.style.display = 'inline';
			warnEl.textContent = c.warnings;
			if (wrapEl) wrapEl.style.opacity = '1';
		} else {
			warnEl.style.display = 'none';
		}
	}
	// Restore hover-only if no issues
	if (wrapEl && c.errors === 0 && c.warnings === 0) {
		wrapEl.style.opacity = '';
	}
}

// --- Find-in-page state per webview ---
var searchState = {}; // appID -> {query, barEl, inputEl, countEl}

function injectBrowserShortcuts(wv, appID) {
	try { wv.executeJavaScript(browserShortcutsScript); } catch(err) {}
}

function getOrCreateSearchBar(appID) {
	if (searchState[appID] && searchState[appID].barEl && searchState[appID].barEl.parentNode) {
		return searchState[appID];
	}
	var wv = window.__libroWebviews[appID];
	if (!wv) return null;

	var container = wv.closest('[data-app-id]');
	if (!container) return null;
	var wrapper = container.children[1];
	if (!wrapper) return null;

	var bar = document.createElement('div');
	bar.className = 'absolute top-2 right-2 z-50 flex items-center gap-1.5 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-600 rounded-md shadow-lg px-2 py-1.5';
	bar.style.display = 'none';

	var input = document.createElement('input');
	input.type = 'text';
	input.placeholder = 'Find in page…';
	input.className = 'text-xs font-mono bg-transparent outline-none text-gray-700 dark:text-zinc-300 placeholder-gray-400 dark:placeholder-zinc-500';
	input.style.width = '180px';

	var countLabel = document.createElement('span');
	countLabel.className = 'text-[10px] text-gray-400 dark:text-zinc-500 font-mono whitespace-nowrap';

	var closeBtn = document.createElement('button');
	closeBtn.className = 'flex items-center justify-center text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 cursor-pointer';
	closeBtn.innerHTML = '<span class="material-icons-round text-sm">close</span>';

	bar.appendChild(input);
	bar.appendChild(countLabel);
	bar.appendChild(closeBtn);
	wrapper.appendChild(bar);

	var state = {query: '', barEl: bar, inputEl: input, countEl: countLabel, findActive: false, matchRect: null};
	searchState[appID] = state;

	function doFind(forward) {
		var q = input.value;
		if (!q) return;
		var isSame = (q === state.query && state.findActive);
		state.query = q;
		state.findActive = true;
		wv.findInPage(q, {forward: forward, findNext: isSame});
	}

	input.addEventListener('keydown', function(e) {
		e.stopPropagation();
		if (e.key === 'Enter') {
			e.preventDefault();
			doFind(!e.shiftKey);
			// Hide bar but keep search active — n/p will continue navigating
			state.barEl.style.display = 'none';
			wv.focus();
		}
		if (e.key === 'Escape') {
			e.preventDefault();
			clearSearch(appID);
		}
	});

	input.addEventListener('input', function() {
		var q = input.value;
		if (q) {
			// Stop any active search before starting a new one to prevent freezing
			if (state.findActive && q !== state.query) {
				try { wv.stopFindInPage('clearSelection'); } catch(err) {}
			}
			state.query = q;
			state.findActive = true;
			wv.findInPage(q, {forward: true, findNext: false});
		} else {
			state.findActive = false;
			state.countEl.textContent = '';
			try { wv.stopFindInPage('clearSelection'); } catch(err) {}
		}
	});

	closeBtn.addEventListener('click', function() {
		clearSearch(appID);
	});

	wv.addEventListener('found-in-page', function(e) {
		if (e.result && searchState[appID]) {
			searchState[appID].countEl.textContent = e.result.activeMatchOrdinal + '/' + e.result.matches;
			if (e.result.selectionArea) {
				searchState[appID].matchRect = e.result.selectionArea;
			}
		}
	});

	return state;
}

function showSearchBar(appID) {
	var state = getOrCreateSearchBar(appID);
	if (!state) return;
	// Stop previous search to prevent freeze when starting a new one
	if (state.findActive) {
		var wv = window.__libroWebviews[appID];
		if (wv) { try { wv.stopFindInPage('clearSelection'); } catch(err) {} }
		state.findActive = false;
	}
	state.barEl.style.display = 'flex';
	state.inputEl.value = state.query || '';
	state.inputEl.focus();
	state.inputEl.select();
}

function clearSearch(appID) {
	var state = searchState[appID];
	if (!state) return;
	state.barEl.style.display = 'none';
	state.query = '';
	state.findActive = false;
	state.matchRect = null;
	state.countEl.textContent = '';
	state.inputEl.value = '';
	var wv = window.__libroWebviews[appID];
	if (wv) {
		try { wv.stopFindInPage('clearSelection'); } catch(err) {}
		wv.focus();
	}
}

function findInPageNext(appID) {
	var state = searchState[appID];
	if (!state || !state.query || !state.findActive) {
		showSearchBar(appID);
		return;
	}
	var wv = window.__libroWebviews[appID];
	if (wv) wv.findInPage(state.query, {forward: true, findNext: true});
}

function findInPagePrev(appID) {
	var state = searchState[appID];
	if (!state || !state.query || !state.findActive) {
		showSearchBar(appID);
		return;
	}
	var wv = window.__libroWebviews[appID];
	if (wv) wv.findInPage(state.query, {forward: false, findNext: true});
}

function handleEnter(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv) return;
	var state = searchState[appID];
	if (state && state.findActive && state.matchRect) {
		var r = state.matchRect;
		var cx = Math.round(r.x + r.width / 2);
		var cy = Math.round(r.y + r.height / 2);
		wv.executeJavaScript(
			'(function(){' +
			'var el=document.elementFromPoint(' + cx + ',' + cy + ');' +
			'while(el){' +
			'var tn=el.tagName?el.tagName.toUpperCase():"";' +
			'if(tn==="A"){el.click();return;}' +
			'if(tn==="BUTTON"){el.click();return;}' +
			'if(el.getAttribute&&(el.getAttribute("role")==="link"||el.getAttribute("role")==="button")){el.click();return;}' +
			'el=el.parentElement;' +
			'}' +
			'})()'
		).catch(function(){});
	} else {
		wv.executeJavaScript(
			'(function(){' +
			'var ae=document.activeElement;' +
			'if(ae&&ae!==document.body&&ae!==document.documentElement)ae.click();' +
			'})()'
		).catch(function(){});
	}
}

// --- End browser shortcuts / find-in-page ---

function whenReady(appID, fn) {
	if (ready[appID]) { fn(); return; }
	if (!queued[appID]) queued[appID] = [];
	queued[appID].push(fn);
}

function initWebview(wv) {
	var appID = wv.getAttribute('data-webview-app');
	if (!appID || initialized[appID]) return;

	// Check pool for a webview with the same origin — reuse it to preserve session state
	var pool = document.getElementById('webview-pool');
	if (pool && !wv.getAttribute('data-pool-origin')) {
		var newSrc = wv.getAttribute('src') || '';
		var newOrigin = '';
		try { var u = new URL(newSrc); newOrigin = u.origin; } catch(e) {}
		if (newOrigin && newOrigin !== 'null') {
			var pooled = pool.querySelector('webview[data-pool-origin="' + newOrigin + '"]');
			if (pooled) {
				// Transfer the pooled webview into the new app frame
				var oldAppID = pooled.getAttribute('data-webview-app');
				pooled.removeAttribute('data-pool-origin');
				pooled.setAttribute('data-webview-app', appID);
				pooled.setAttribute('id', wv.id || '');
				pooled.style.display = '';
				wv.parentNode.replaceChild(pooled, wv);
				// Migrate JS state from old app ID to new one
				if (oldAppID && oldAppID !== appID) {
					delete window.__libroWebviews[oldAppID];
					delete initialized[oldAppID];
					delete ready[oldAppID];
					delete queued[oldAppID];
					delete searchState[oldAppID];
				}
				window.__libroWebviews[appID] = pooled;
				initialized[appID] = true;
				ready[appID] = true;
				// Remove loading indicator since the webview is already loaded
				var loading = pooled.parentNode && pooled.parentNode.querySelector('[data-webview-loading]');
				if (loading && loading.parentNode) loading.remove();
				// Update URL bar with current webview URL
				var inp = document.getElementById('urlinput-' + appID);
				if (inp) {
					try { inp.value = pooled.getURL() || newSrc; } catch(e) { inp.value = newSrc; }
				}
				// Re-bind events for the new app ID
				bindWebviewEvents(pooled, appID);
				return;
			}
		}
	}

	initialized[appID] = true;
	window.__libroWebviews[appID] = wv;
	bindWebviewEvents(wv, appID);
}

function bindWebviewEvents(wv, appID) {
	wv.addEventListener('dom-ready', function() {
		ready[appID] = true;
		var q = queued[appID];
		if (q) { queued[appID] = null; q.forEach(function(fn){ fn(); }); }
		injectBrowserShortcuts(wv, appID);
	});

	// Update URL bar on navigation and reset console counts
	wv.addEventListener('did-navigate', function(e) {
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
		consoleCounts[appID] = {errors: 0, warnings: 0};
		updateConsoleBadges(appID);
	});
	wv.addEventListener('did-navigate-in-page', function(e) {
		if (!e.isMainFrame) return;
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
	});

	// Re-inject browser shortcuts after full page navigation
	wv.addEventListener('did-finish-load', function() {
		injectBrowserShortcuts(wv, appID);
	});

	// Listen for browser shortcut messages and track errors/warnings
	if (!consoleCounts[appID]) consoleCounts[appID] = {errors: 0, warnings: 0};
	wv.addEventListener('console-message', function(e) {
		var msg = e.message;
		if (msg === '__libro:search') showSearchBar(appID);
		else if (msg === '__libro:findnext') findInPageNext(appID);
		else if (msg === '__libro:findprev') findInPagePrev(appID);
		else if (msg === '__libro:searchclear') clearSearch(appID);
		else if (msg === '__libro:enter') handleEnter(appID);
		else if (msg && !msg.startsWith('__libro:')) {
			// level: 0=log, 1=warning, 2=error
			if (e.level === 2) {
				consoleCounts[appID].errors++;
				updateConsoleBadges(appID);
			} else if (e.level === 1) {
				consoleCounts[appID].warnings++;
				updateConsoleBadges(appID);
			}
		}
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
	document.querySelectorAll('webview[data-webview-app]:not([data-pool-origin])').forEach(initWebview);
}

initAll();

var bodyObserver = new MutationObserver(function() { initAll(); });
bodyObserver.observe(document.body, { childList: true, subtree: true });

// Cleanup when webview removed from DOM (skip pooled webviews)
var cleanupObserver = new MutationObserver(function(mutations) {
	var pool = document.getElementById('webview-pool');
	mutations.forEach(function(m) {
		m.removedNodes.forEach(function(node) {
			if (node.nodeType !== 1) return;
			var wvs = node.querySelectorAll ? node.querySelectorAll('webview[data-webview-app]') : [];
			wvs.forEach(function(wv) {
				// Skip cleanup if webview was moved to pool
				if (pool && pool.contains(wv)) return;
				var id = wv.getAttribute('data-webview-app');
				if (id) {
					delete window.__libroWebviews[id];
					delete initialized[id];
					delete ready[id];
					delete queued[id];
					delete searchState[id];
				}
			});
			if (node.tagName === 'WEBVIEW' && node.getAttribute('data-webview-app')) {
				if (pool && pool.contains(node)) return;
				var id = node.getAttribute('data-webview-app');
				delete window.__libroWebviews[id];
				delete initialized[id];
				delete ready[id];
				delete queued[id];
				delete searchState[id];
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
		wv.loadURL(url).catch(function(){});
	} else {
		// Not ready yet — set src attribute to trigger initial load
		wv.setAttribute('src', url);
	}
};
})();
`
