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
	window.__libroHoveredLink = '';
	window.__libroHoveredElement = null;
	document.addEventListener('mouseover', function(e) {
		var el = e.target;
		window.__libroHoveredElement = el || null;
		while (el && el.tagName !== 'A') el = el.parentElement;
		window.__libroHoveredLink = (el && el.href) ? el.href : '';
	}, true);
	// Track input focus state — the Electron main process listens for these
	// console messages to decide whether to intercept plain keys (j/k/h/l etc.)
	// or let them through to text input fields.
	document.addEventListener('focusin', function(e) {
		var t = e.target.tagName ? e.target.tagName.toUpperCase() : '';
		if(t==='INPUT'||t==='TEXTAREA'||t==='SELECT'||(e.target&&e.target.isContentEditable)) {
			console.log('__libro:inputfocus');
		}
	}, true);
	document.addEventListener('focusout', function(e) {
		var t = e.target.tagName ? e.target.tagName.toUpperCase() : '';
		if(t==='INPUT'||t==='TEXTAREA'||t==='SELECT'||(e.target&&e.target.isContentEditable)) {
			console.log('__libro:inputblur');
		}
	}, true);
	document.addEventListener('keydown', function(e) {
		if(e.metaKey || e.ctrlKey || e.altKey) return;
		var ae = document.activeElement;
		var tag = ae ? ae.tagName.toUpperCase() : '';
		if(tag==='INPUT'||tag==='TEXTAREA'||tag==='SELECT'||(ae&&ae.isContentEditable)) {
			if(e.key==='Escape' && ae) { ae.blur(); e.preventDefault(); e.stopPropagation(); }
			return;
		}
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
			case 'y':
				var sel = window.getSelection();
				var txt = sel ? sel.toString() : '';
				if (txt) {
					console.log('__libro:copytext:' + txt);
				} else if (window.__libroHoveredLink) {
					console.log('__libro:copytext:' + window.__libroHoveredLink);
				}
				break;
			case 'c':
				console.log('__libro:console');
				break;
			case '/': console.log('__libro:search'); break;
			case 'n': console.log('__libro:findnext'); break;
			case 'N': console.log('__libro:findprev'); break;
			case 'p': console.log('__libro:findprev'); break;
			case 'Escape': console.log('__libro:searchclear'); break;
			case 'Enter':
				var ae2=document.activeElement;
				var tn2=ae2?ae2.tagName.toUpperCase():'';
				if(tn2==='A'||tn2==='BUTTON'||(ae2&&ae2.getAttribute&&(ae2.getAttribute('role')==='link'||ae2.getAttribute('role')==='button'))){
					handled=false;
				}else{
					console.log('__libro:enter');
				}
				break;
			default: handled = false;
		}
		if(handled) { e.preventDefault(); e.stopPropagation(); }
	}, true);
} + ')()';

// --- Console error/warning counts per webview ---
var consoleCounts = {}; // appID -> {errors}
var consoleMessages = {}; // appID -> [{level, message, source, line}]
function updateConsoleBadges(appID) {
	var c = consoleCounts[appID] || {errors: 0};
	var errEl = document.getElementById('devtools-errors-' + appID);
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
	// Restore hover-only if no errors
	if (wrapEl && c.errors === 0) {
		wrapEl.style.opacity = '';
	}
}

function addConsoleMessage(appID, level, message, source, line) {
	if (!consoleMessages[appID]) consoleMessages[appID] = [];
	consoleMessages[appID].push({level: level, message: message, source: source, line: line});
	// Cap at 500 messages
	if (consoleMessages[appID].length > 500) consoleMessages[appID].shift();
}

window.__libroToggleConsole = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv || !window.libroElectron || typeof window.libroElectron.toggleWebviewDevTools !== 'function') return;
	whenReady(appID, function() {
		try {
			var targetId = webviewContentsID(wv);
			if (!targetId) return;
			var panel = document.getElementById('devtools-panel-' + appID);
			var opening = !!(panel && panel.classList.contains('hidden'));
			setDevtoolsPanelVisible(appID, opening);
			var bounds = devtoolsPanelBounds(appID);
			if (!bounds) return;
			window.libroElectron.toggleWebviewDevTools(targetId, bounds, 'console');
			if (!opening) setDevtoolsPanelVisible(appID, false);
		} catch (err) {}
	});
};

window.__libroOpenConsole = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv || !window.libroElectron || typeof window.libroElectron.openWebviewDevTools !== 'function') return;
	whenReady(appID, function() {
		try {
			var targetId = webviewContentsID(wv);
			if (!targetId) return;
			setDevtoolsPanelVisible(appID, true);
			var bounds = devtoolsPanelBounds(appID);
			if (!bounds) return;
			window.libroElectron.openWebviewDevTools(targetId, bounds, 'console');
		} catch (err) {}
	});
};

window.__libroCloseConsole = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (!wv || !window.libroElectron || typeof window.libroElectron.closeWebviewDevTools !== 'function') return;
	whenReady(appID, function() {
		try {
			var targetId = webviewContentsID(wv);
			if (!targetId) return;
			window.libroElectron.closeWebviewDevTools(targetId);
			setDevtoolsPanelVisible(appID, false);
		} catch (err) {}
	});
};

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
		// Use elementFromPoint with the match rectangle from find-in-page
		var r = state.matchRect;
		var cx = r.x + Math.round(r.width / 2);
		var cy = r.y + Math.round(r.height / 2);
		wv.executeJavaScript(
			'(function(){' +
			'var el=document.elementFromPoint(' + cx + ',' + cy + ');' +
			'var node=el;' +
			'while(node){' +
			'if(node.nodeType===1){' +
			'var tn=node.tagName.toUpperCase();' +
			'if(tn==="A"){node.click();return;}' +
			'if(tn==="BUTTON"){node.click();return;}' +
			'if(node.getAttribute&&(node.getAttribute("role")==="link"||node.getAttribute("role")==="button")){node.click();return;}' +
			'}' +
			'node=node.parentNode;' +
			'}' +
			'if(el)el.click();' +
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

function currentAppID(wv) {
	return wv ? (wv.getAttribute('data-webview-app') || '') : '';
}

function webviewContentsID(wv) {
	if (!wv) return 0;
	try { return Number(wv.getWebContentsId ? wv.getWebContentsId() : 0) || 0; } catch (err) {}
	return 0;
}

function setDevtoolsPanelVisible(appID, visible) {
	var panel = document.getElementById('devtools-panel-' + appID);
	if (!panel) return;
	if (visible) panel.classList.remove('hidden');
	else panel.classList.add('hidden');
	var closeBtn = document.getElementById('devtools-close-' + appID);
	if (closeBtn) {
		if (visible) closeBtn.classList.remove('hidden');
		else closeBtn.classList.add('hidden');
	}
}

function devtoolsPanelBounds(appID) {
	var panel = document.getElementById('devtools-panel-' + appID);
	if (!panel) return null;
	var rect = panel.getBoundingClientRect();
	if (!rect.width || !rect.height) return null;
	return {
		x: Math.round(rect.left),
		y: Math.round(rect.top),
		width: Math.max(1, Math.round(rect.width)),
		height: Math.max(1, Math.round(rect.height)),
	};
}

function updateDevtoolsBounds(appID) {
	var wv = window.__libroWebviews[appID];
	var targetId = webviewContentsID(wv);
	var bounds = devtoolsPanelBounds(appID);
	if (!targetId || !bounds || !window.libroElectron || typeof window.libroElectron.updateWebviewDevToolsBounds !== 'function') return;
	window.libroElectron.updateWebviewDevToolsBounds(targetId, bounds);
}

function focusIfSelected(appID, wv) {
	if (!appID || !wv) return;
	function attempt() {
		if ((window.__libroSelectedApp || '') !== appID) return;
		try { window.focus(); } catch(err) {}
		try { wv.focus(); } catch(err) {}
	}
	attempt();
	setTimeout(attempt, 40);
	setTimeout(attempt, 120);
	setTimeout(attempt, 260);
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
				var currentURL = '';
				try { currentURL = pooled.getURL() || pooled.getAttribute('src') || ''; } catch(e) { currentURL = pooled.getAttribute('src') || ''; }
				pooled.removeAttribute('data-pool-origin');
				pooled.setAttribute('data-webview-app', appID);
				pooled.setAttribute('id', wv.id || '');
				pooled.setAttribute('data-sid', wv.getAttribute('data-sid') || '');
				pooled.setAttribute('src', newSrc);
				pooled.style.display = '';
				wv.parentNode.replaceChild(pooled, wv);
				// Migrate JS state from old app ID to new one
				if (oldAppID && oldAppID !== appID) {
					delete window.__libroWebviews[oldAppID];
					delete initialized[oldAppID];
					delete ready[oldAppID];
					delete queued[oldAppID];
					delete searchState[oldAppID];
					delete consoleMessages[oldAppID];
					delete consoleCounts[oldAppID];
				}
				window.__libroWebviews[appID] = pooled;
				initialized[appID] = true;
				ready[appID] = currentURL === newSrc;
				// Update URL bar with the requested URL immediately.
				var inp = document.getElementById('urlinput-' + appID);
				if (inp) inp.value = newSrc;
				// Re-bind events for the current webview element if needed.
					bindWebviewEvents(pooled);
					focusIfSelected(appID, pooled);
				// Reused webviews keep their session, but must navigate to the new
				// target or a newly opened tab can show stale content from the prior tab.
				if (currentURL !== newSrc) {
					try { pooled.loadURL(newSrc).catch(function(){}); } catch(err) {}
				} else {
					// Remove loading indicator since the webview is already at the target.
					var lp = pooled.closest('.flex.flex-col') || pooled.parentNode;
					var loading = lp && lp.querySelector('[data-webview-loading]');
					if (loading && loading.parentNode) loading.remove();
				}
				return;
			}
		}
	}

	initialized[appID] = true;
	window.__libroWebviews[appID] = wv;
	bindWebviewEvents(wv);
	focusIfSelected(appID, wv);
}

function bindWebviewEvents(wv) {
	if (wv.__libroEventsBound) return;
	wv.__libroEventsBound = true;

	wv.addEventListener('dom-ready', function() {
		var appID = currentAppID(wv);
		if (!appID) return;
		ready[appID] = true;
		var q = queued[appID];
		if (q) { queued[appID] = null; q.forEach(function(fn){ fn(); }); }
		injectBrowserShortcuts(wv, appID);
		focusIfSelected(appID, wv);
	});

	// Update URL bar on navigation and reset console counts
	wv.addEventListener('did-navigate', function(e) {
		var appID = currentAppID(wv);
		if (!appID) return;
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
		consoleCounts[appID] = {errors: 0};
		consoleMessages[appID] = [];
		updateConsoleBadges(appID);
	});
	wv.addEventListener('did-navigate-in-page', function(e) {
		if (!e.isMainFrame) return;
		var appID = currentAppID(wv);
		if (!appID) return;
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
	});

	// Re-inject browser shortcuts after full page navigation
	wv.addEventListener('did-finish-load', function() {
		var appID = currentAppID(wv);
		if (!appID) return;
		injectBrowserShortcuts(wv, appID);
	});

	// Listen for browser shortcut messages and track errors/warnings
	wv.addEventListener('console-message', function(e) {
		var appID = currentAppID(wv);
		if (!appID) return;
		if (!consoleCounts[appID]) consoleCounts[appID] = {errors: 0};
		var msg = e.message;
		if (msg === '__libro:search') showSearchBar(appID);
		else if (msg === '__libro:findnext') findInPageNext(appID);
		else if (msg === '__libro:findprev') findInPagePrev(appID);
		else if (msg === '__libro:searchclear') clearSearch(appID);
		else if (msg === '__libro:enter') handleEnter(appID);
		else if (msg === '__libro:console') window.__libroOpenConsole(appID);
		else if (msg === '__libro:copyurl') {
			var inp = document.getElementById('urlinput-' + appID);
			if (inp && navigator.clipboard) navigator.clipboard.writeText(inp.value);
		}
		else if (msg && msg.startsWith('__libro:copytext:')) {
			var text = msg.substring('__libro:copytext:'.length);
			if (text) {
				if (window.libroElectron && window.libroElectron.copyToClipboard) {
					window.libroElectron.copyToClipboard(text);
				} else if (navigator.clipboard) {
					navigator.clipboard.writeText(text);
				}
				if (window.__libroShowToast) {
					var preview = text.length > 60 ? text.substring(0, 60) + '…' : text;
					window.__libroShowToast('Copied', preview, 1500);
				}
			}
		}
		else if (msg && !msg.startsWith('__libro:')) {
			// Store and display all console messages
			addConsoleMessage(appID, e.level, msg, e.sourceId || '', e.line || 0);
			// level: 0=log, 1=warning, 2=error
			if (e.level === 2) {
				consoleCounts[appID].errors++;
			}
			updateConsoleBadges(appID);
		}
	});

	// Show error page on load failure — for DNS errors, redirect to Google search
	wv.addEventListener('did-fail-load', function(e) {
		var appID = currentAppID(wv);
		if (!appID) return;
		// Ignore aborted loads (e.g. navigation interrupted by another navigation)
		if (e.errorCode === -3) return;
		var failedUrl = e.validatedURL || '';
		// DNS resolution failure (-105 ERR_NAME_NOT_RESOLVED, -106 ERR_INTERNET_DISCONNECTED)
		// Redirect to Google search using the hostname/path as query
		if (e.errorCode === -105 && failedUrl) {
			try {
				var u = new URL(failedUrl);
				var q = u.hostname + (u.pathname && u.pathname !== '/' ? u.pathname : '');
				if (q) {
					var searchUrl = 'https://www.google.com/search?q=' + encodeURIComponent(q);
					wv.loadURL(searchUrl);
					var inp = document.getElementById('urlinput-' + appID);
					if (inp) inp.value = searchUrl;
					// Update server state
					var sidAttr = wv.getAttribute('data-sid');
					if (sidAttr) __ws.call('app.url.set', {sid: sidAttr, id: appID, url: searchUrl});
					return;
				}
			} catch(err) {}
		}
		var errorDesc = e.errorDescription || 'Unknown error';
		var html = '<html><body style="display:flex;align-items:center;justify-content:center;height:100%;margin:0;font-family:system-ui,sans-serif;background:#fafafa;color:#333">' +
			'<div style="text-align:center;max-width:400px;padding:2rem">' +
			'<div style="font-size:2rem;margin-bottom:1rem">&#9888;</div>' +
			'<div style="font-size:14px;font-weight:600;margin-bottom:0.5rem">' + errorDesc + '</div>' +
			'<div style="font-size:12px;color:#888;word-break:break-all">' + failedUrl.replace(/</g,'&lt;') + '</div>' +
			'</div></body></html>';
		try { wv.loadURL('data:text/html;charset=utf-8,' + encodeURIComponent(html)); } catch(err) {}
	});

	// Loading indicator removal (search up to flex-col container)
	var loadingParent = wv.closest('.flex.flex-col') || wv.parentNode;
	var loading = loadingParent && loadingParent.querySelector('[data-webview-loading]');
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
window.addEventListener('resize', function() {
	Object.keys(window.__libroWebviews).forEach(function(appID) {
		updateDevtoolsBounds(appID);
	});
});

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
						delete consoleMessages[id];
						delete consoleCounts[id];
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
window.__libroOpenNewTab = function(url) {
	// Find the sid from any webview with a data-sid attribute
	var wv = document.querySelector('webview[data-sid]');
	var sid = wv ? wv.getAttribute('data-sid') : 'default';
	__ws.call('app.start', {sid: sid, type: 'url', url: url, width: 'lg', side: 'right'});
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
