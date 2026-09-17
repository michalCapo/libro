package components

// BrowserJS returns the JavaScript that manages Electron webview elements.
// It initializes webview tags, handles navigation events, and provides
// back/forward/reload/navigate functions via the webview DOM API.
// It also injects browser-mode scrolling, address, reload, and viewport shortcuts.
func BrowserJS() string {
	return browserScript
}

const browserScript = `
(function(){
window.__libroWebviews = window.__libroWebviews || {};
var ready = {};       // appID -> true when dom-ready has fired
var queued = {};      // appID -> [fn, fn, ...] calls waiting for dom-ready
var initialized = {};

// --- Browser shortcuts script injected into webview guest pages ---
var browserShortcutsScript = '(' + function(){
	if(window.__libroBrowserShortcuts) return;
	window.__libroBrowserShortcuts = true;
	function elementRole(el) {
		if (!el || !el.getAttribute) return '';
		return (el.getAttribute('role') || '').toLowerCase();
	}
	function isEditableElement(el) {
		if (!el) return false;
		if (el.isContentEditable) return true;
		var tag = el.tagName ? el.tagName.toUpperCase() : '';
		if (tag === 'TEXTAREA' || tag === 'SELECT') return true;
		if (tag === 'INPUT') {
			var inputType = ((el.getAttribute && el.getAttribute('type')) || el.type || '').toLowerCase();
			if (!inputType || ['text', 'search', 'email', 'url', 'tel', 'password', 'number'].indexOf(inputType) >= 0) return true;
		}
		if (el.getAttribute) {
			var role = elementRole(el);
			if (role === 'textbox' || role === 'searchbox' || role === 'combobox' || role === 'spinbutton') return true;
		}
		return false;
	}
	function activeEditableElement() {
		var el = document.activeElement;
		while (el && el.shadowRoot && el.shadowRoot.activeElement) {
			el = el.shadowRoot.activeElement;
		}
		if (isEditableElement(el)) return el;
		if (el && typeof el.closest === 'function') {
			var editableParent = el.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"], [role="textbox"], [role="searchbox"], [role="combobox"], [role="spinbutton"]');
			if (editableParent) return editableParent;
		}
		return null;
	}
	function syncInputFocus() {
		if (window.__libroKeyboardPassthrough) {
			console.log('__libro:passthrough');
			console.log('__libro:inputfocus');
			return;
		}
		console.log(activeEditableElement() ? '__libro:inputfocus' : '__libro:inputblur');
	}
	// Track input focus state — the Electron main process listens for these
	// console messages to decide whether to intercept plain keys (j/k/h/l etc.)
	// or let them through to text input fields.
	document.addEventListener('focusin', syncInputFocus, true);
	document.addEventListener('focusout', function() {
		setTimeout(syncInputFocus, 0);
	}, true);
	document.addEventListener('mousedown', function() {
		setTimeout(syncInputFocus, 0);
	}, true);
	document.addEventListener('selectionchange', syncInputFocus, true);
	window.addEventListener('pageshow', syncInputFocus, true);
	window.addEventListener('load', syncInputFocus, true);
	document.addEventListener('keydown', function(e) {
		if(e.metaKey || e.ctrlKey || e.altKey) return;
		// Insert mode is enforced in the main process (electron/main.js).
		// When in insert mode, all the cases below are skipped because the
		// main process has already short-circuited the matching shortcuts.
		if (window.__libroKeyboardPassthrough || window.__libroBrowserMode === 'insert') return;
		var ae = activeEditableElement();
		if(ae) return;
		var handled = true;
		switch(e.key) {
			case 'g': window.scrollTo({top: 0, behavior: 'smooth'}); break;
			case 'G': window.scrollTo({top: document.documentElement.scrollHeight, behavior: 'smooth'}); break;
			case 'j': window.scrollBy({top: 800, behavior: 'smooth'}); break;
			case 'k': window.scrollBy({top: -800, behavior: 'smooth'}); break;
			case 'h': window.scrollBy({left: -480, behavior: 'smooth'}); break;
			case 'l': window.scrollBy({left: 480, behavior: 'smooth'}); break;
			case 'o': console.log('__libro:urlpopup'); break;
			case 'r': console.log('__libro:reload'); break;
			case 'm': console.log('__libro:viewport'); break;
			case 'M': console.log('__libro:viewportrotate'); break;
			default: handled = false;
		}
		if(handled) { e.preventDefault(); e.stopPropagation(); }
	}, true);
	syncInputFocus();
} + ')()';

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
			refocusWebview(appID, wv);
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
			refocusWebview(appID, wv);
		} catch (err) {}
	});
};

var devtoolsPanelObservers = {};
var devtoolsPanelSyncers = {};
var browserModeState = {}; // appID -> 'normal' | 'insert'
var mobileViewState = {}; // appID -> {mode, orientation, previousFrameStyle, previousContentStyle, previousWidth}
var mobileViewportOrientation = {}; // appID -> last sm/md/xl orientation until browser instance closes
var mobileSizes = {
	sm: {label: 'SM', width: 480, height: 896},
	md: {label: 'MD', width: 640, height: 932},
	xl: {label: 'XL', width: 720, height: 1280}
};

function applyBrowserMode(appID, mode) {
	if (!appID) return;
	browserModeState[appID] = mode;
	var container = document.querySelector('[data-app-id="' + appID + '"]');
	if (container) {
		var isSelected = container.className.indexOf('border-blue-500') !== -1 ||
			container.className.indexOf('border-emerald-500') !== -1;
		if (isSelected) {
			if (mode === 'insert') {
				container.className = container.className.replace(/border-blue-500/g, 'border-emerald-500');
			} else {
				container.className = container.className.replace(/border-emerald-500/g, 'border-blue-500');
			}
		}
		var toolbar = container.querySelector('[data-app-toolbar]');
		if (toolbar) {
			// Only restyle the toolbar when the app is the selected one
			// (selected toolbars use bg-blue-600). Unselected toolbars are
			// gray/white and should remain unchanged regardless of mode.
			var toolbarIsSelected = toolbar.className.indexOf('bg-blue-600') !== -1 ||
				toolbar.className.indexOf('bg-emerald-600') !== -1;
			if (toolbarIsSelected) {
				if (mode === 'insert') {
					toolbar.className = toolbar.className
						.replace(/bg-blue-600/g, 'bg-emerald-600')
						.replace(/border-blue-700/g, 'border-emerald-700');
				} else {
					toolbar.className = toolbar.className
						.replace(/bg-emerald-600/g, 'bg-blue-600')
						.replace(/border-emerald-700/g, 'border-blue-700');
				}
			}
		}
	}
	// Mirror the mode flag into the guest page so its own keydown handler
	// short-circuits in insert mode without waiting for a round-trip.
	var wv = window.__libroWebviews[appID];
	if (wv && wv.executeJavaScript) {
		try { wv.executeJavaScript('window.__libroBrowserMode=' + JSON.stringify(mode) + ';'); } catch(err) {}
	}
}

window.__libroSetBrowserModeByWcId = function(wcId, mode) {
	wcId = Number(wcId) || 0;
	if (!wcId) return;
	for (var appID in window.__libroWebviews) {
		var wv = window.__libroWebviews[appID];
		if (!wv) continue;
		var id = 0;
		try { id = Number(wv.getWebContentsId ? wv.getWebContentsId() : 0) || 0; } catch(err) {}
		if (id === wcId) { applyBrowserMode(appID, mode); return; }
	}
};

window.__libroGetBrowserMode = function(appID) {
	return browserModeState[appID] || 'normal';
};

window.__libroApplyBrowserMode = applyBrowserMode;

function currentAppWidth(appID) {
	var frame = document.querySelector('[data-app-id="' + appID + '"]');
	if (!frame) return '';
	var active = frame.querySelector('[data-size-badges] button.bg-white\\/25, [data-size-badges] button.bg-blue-600');
	if (active) return (active.getAttribute('data-resize-width') || active.textContent || '').trim().toLowerCase();
	var widths = ['sm','md','lg','xl','2xl','3xl','full'];
	for (var i = 0; i < widths.length; i++) {
		var btn = frame.querySelector('[data-resize-width="' + widths[i] + '"]');
		if (btn && (btn.className || '').indexOf('text-white') !== -1) return widths[i];
	}
	return '';
}

function resizeAppForViewport(appID, width) {
	if (!width || !window.__libroResizeApp) return;
	var host = document.querySelector('[data-webview-app="' + appID + '"], iframe[data-browser-iframe-app="' + appID + '"]');
	var sid = host ? (host.getAttribute('data-sid') || '') : '';
	window.__libroResizeApp(appID, width, sid || undefined);
}

function applyMobileView(appID, mode, orientation) {
	var frame = document.querySelector('[data-app-id="' + appID + '"]');
	var content = document.querySelector('[data-app-content="' + appID + '"]');
	if (!frame || !content) return false;
	var state = mobileViewState[appID];
	if (!state) {
		state = mobileViewState[appID] = {
			mode: 'normal',
			orientation: mobileViewportOrientation[appID] || 'portrait',
			previousFrameStyle: frame.getAttribute('style') || '',
			previousContentStyle: content.getAttribute('style') || '',
			previousWidth: currentAppWidth(appID)
		};
	}
	if (mode === 'normal') {
		mobileViewportOrientation[appID] = state.orientation || mobileViewportOrientation[appID] || 'portrait';
		frame.setAttribute('style', state.previousFrameStyle || '');
		content.setAttribute('style', state.previousContentStyle || '');
		delete mobileViewState[appID];
		if (window.__libroSettleAppFrame) window.__libroSettleAppFrame(appID);
		return true;
	}
	var size = mobileSizes[mode];
	if (!size) return false;
	state.mode = mode;
	state.orientation = orientation || state.orientation || mobileViewportOrientation[appID] || 'portrait';
	mobileViewportOrientation[appID] = state.orientation;
	var width = state.orientation === 'landscape' ? size.height : size.width;
	var height = state.orientation === 'landscape' ? size.width : size.height;
	frame.style.width = width + 'px';
	frame.style.flex = '0 0 ' + width + 'px';
	frame.style.maxWidth = width + 'px';
	frame.style.height = 'auto';
	frame.style.maxHeight = 'calc(100% - 8px)';
	frame.style.alignSelf = 'center';
	content.style.height = height + 'px';
	content.style.flex = '0 0 ' + height + 'px';
	content.style.maxHeight = height + 'px';
	content.style.minHeight = '0';
	if (window.__libroScrollToApp) window.__libroScrollToApp(frame);
	if (window.__libroSettleAppFrame) window.__libroSettleAppFrame(appID);
	return true;
}

window.__libroToggleSelectedBrowserMobile = function(appID) {
	appID = appID || window.__libroSelectedApp || '';
	if (!appID) return;
	var target = document.querySelector('webview[data-webview-app="' + appID + '"], iframe[data-browser-iframe-app="' + appID + '"]');
	if (!target) return;
	var current = (mobileViewState[appID] && mobileViewState[appID].mode) || 'normal';
	var orientation = (mobileViewState[appID] && mobileViewState[appID].orientation) || mobileViewportOrientation[appID] || 'portrait';
	var next = current === 'normal' ? 'sm' : (current === 'sm' ? 'md' : (current === 'md' ? 'xl' : 'normal'));
	// Viewport preview sizes are applied locally with inline styles. Avoid also
	// resizing the app through the server, which causes a second layout jump when
	// the server patch arrives.
	setTimeout(function(){
		if (!applyMobileView(appID, next, orientation)) return;
		if (next !== 'normal') {
			setTimeout(function(){
				var state = mobileViewState[appID];
				if (state && state.mode === next) applyMobileView(appID, next, state.orientation || orientation);
			}, 80);
		}
		if (window.__libroShowToast) {
			if (next === 'normal') window.__libroShowToast('Viewport off', 'Restored previous browser size', 1200);
			else {
				var size = mobileSizes[next];
				var width = orientation === 'landscape' ? size.height : size.width;
				var height = orientation === 'landscape' ? size.width : size.height;
				window.__libroShowToast('Viewport ' + size.label, width + ' × ' + height, 1200);
			}
		}
		var wv = window.__libroWebviews[appID];
		if (wv) refocusWebview(appID, wv);
	}, 0);
};

window.__libroRotateSelectedBrowserViewport = function(appID) {
	appID = appID || window.__libroSelectedApp || '';
	var state = appID ? mobileViewState[appID] : null;
	if (!state || (state.mode !== 'sm' && state.mode !== 'md' && state.mode !== 'xl')) return;
	var nextOrientation = state.orientation === 'landscape' ? 'portrait' : 'landscape';
	mobileViewportOrientation[appID] = nextOrientation;
	if (!applyMobileView(appID, state.mode, nextOrientation)) return;
	if (window.__libroShowToast) {
		var size = mobileSizes[state.mode];
		var width = nextOrientation === 'landscape' ? size.height : size.width;
		var height = nextOrientation === 'landscape' ? size.width : size.height;
		window.__libroShowToast('Viewport ' + nextOrientation, width + ' × ' + height, 1200);
	}
	var wv = window.__libroWebviews[appID];
	if (wv) refocusWebview(appID, wv);
};

function injectBrowserShortcuts(wv, appID) {
	try { wv.executeJavaScript(browserShortcutsScript); } catch(err) {}
}

function refocusWebview(appID, wv) {
	if (!wv) return;
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

if (window.libroElectron && typeof window.libroElectron.onWebviewDevToolsClosed === 'function' && !window.__libroDevtoolsCloseSyncRegistered) {
	window.__libroDevtoolsCloseSyncRegistered = true;
	window.libroElectron.onWebviewDevToolsClosed(function(targetId) {
		var numericTargetId = Number(targetId) || 0;
		if (!numericTargetId) return;
		var webviews = document.querySelectorAll('webview[data-webview-app]');
		for (var i = 0; i < webviews.length; i++) {
			var wv = webviews[i];
			var webviewId = 0;
			try { webviewId = Number(wv.getWebContentsId ? wv.getWebContentsId() : 0) || 0; } catch (err) {}
			if (webviewId !== numericTargetId) continue;
			var appID = wv.getAttribute('data-webview-app') || '';
			if (appID) {
				setDevtoolsPanelVisible(appID, false);
				refocusWebview(appID, wv);
			}
			break;
		}
	});
}

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
		if (visible) {
			closeBtn.classList.remove('hidden');
			closeBtn.classList.add('inline-flex');
		} else {
			closeBtn.classList.add('hidden');
			closeBtn.classList.remove('inline-flex');
		}
	}
	if (visible) startDevtoolsBoundsSync(appID);
	else stopDevtoolsBoundsSync(appID);
}

function isDevtoolsPanelVisible(appID) {
	var panel = document.getElementById('devtools-panel-' + appID);
	return !!(panel && !panel.classList.contains('hidden') && panel.getClientRects && panel.getClientRects().length);
}

function devtoolsPanelBounds(appID) {
	var panel = document.getElementById('devtools-panel-' + appID);
	if (!panel) return null;
	var rect = panel.getBoundingClientRect();
	if (!rect.width || !rect.height) return null;
	var zoomFactor = 1;
	try {
		if (window.libroElectron && typeof window.libroElectron.getZoomFactor === 'function') {
			zoomFactor = Number(window.libroElectron.getZoomFactor()) || 1;
		}
	} catch (err) {}
	if (!zoomFactor || zoomFactor < 0.01) zoomFactor = 1;
	return {
		x: Math.round(rect.left * zoomFactor),
		y: Math.round(rect.top * zoomFactor),
		width: Math.max(1, Math.round(rect.width * zoomFactor)),
		height: Math.max(1, Math.round(rect.height * zoomFactor)),
	};
}

function sameDevtoolsBounds(a, b) {
	return !!(a && b && a.x === b.x && a.y === b.y && a.width === b.width && a.height === b.height);
}

function updateDevtoolsBounds(appID, force) {
	var wv = window.__libroWebviews[appID];
	var targetId = webviewContentsID(wv);
	var bounds = devtoolsPanelBounds(appID);
	if (!targetId || !bounds || !window.libroElectron || typeof window.libroElectron.updateWebviewDevToolsBounds !== 'function') return;
	var syncer = devtoolsPanelSyncers[appID];
	if (!force && syncer && sameDevtoolsBounds(syncer.lastBounds, bounds)) return;
	if (syncer) syncer.lastBounds = bounds;
	window.libroElectron.updateWebviewDevToolsBounds(targetId, bounds);
}

function observeDevtoolsPanel(appID) {
	if (!window.ResizeObserver || devtoolsPanelObservers[appID]) return;
	var panel = document.getElementById('devtools-panel-' + appID);
	if (!panel) return;
	var observer = new ResizeObserver(function() {
		updateDevtoolsBounds(appID);
	});
	observer.observe(panel);
	devtoolsPanelObservers[appID] = observer;
}

function startDevtoolsBoundsSync(appID) {
	if (!window.requestAnimationFrame) {
		updateDevtoolsBounds(appID, true);
		return;
	}
	var syncer = devtoolsPanelSyncers[appID];
	if (syncer && syncer.running) return;
	syncer = syncer || {};
	syncer.running = true;
	syncer.lastBounds = null;
	devtoolsPanelSyncers[appID] = syncer;
	updateDevtoolsBounds(appID, true);
	function tick() {
		var current = devtoolsPanelSyncers[appID];
		if (!current || !current.running) return;
		if (isDevtoolsPanelVisible(appID)) updateDevtoolsBounds(appID, false);
		current.rafId = window.requestAnimationFrame(tick);
	}
	syncer.rafId = window.requestAnimationFrame(tick);
}

function stopDevtoolsBoundsSync(appID) {
	var syncer = devtoolsPanelSyncers[appID];
	if (!syncer) return;
	syncer.running = false;
	syncer.lastBounds = null;
	if (syncer.rafId && window.cancelAnimationFrame) {
		window.cancelAnimationFrame(syncer.rafId);
	}
	syncer.rafId = 0;
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
	if (!appID) return;
	if (initialized[appID]) {
		var existing = window.__libroWebviews[appID];
		var existingPool = document.getElementById('webview-pool');
		if (existing && existingPool && existingPool.contains(existing) && existing !== wv && !wv.getAttribute('data-pool-origin')) {
			delete initialized[appID];
		} else {
			return;
		}
	}
	observeDevtoolsPanel(appID);

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
					if (devtoolsPanelObservers[oldAppID]) {
						try { devtoolsPanelObservers[oldAppID].disconnect(); } catch (err) {}
						delete devtoolsPanelObservers[oldAppID];
					}
					stopDevtoolsBoundsSync(oldAppID);
					delete devtoolsPanelSyncers[oldAppID];
					delete window.__libroWebviews[oldAppID];
					delete initialized[oldAppID];
					delete ready[oldAppID];
					delete queued[oldAppID];
					delete browserModeState[oldAppID];
					if (mobileViewportOrientation[oldAppID]) mobileViewportOrientation[appID] = mobileViewportOrientation[oldAppID];
					delete mobileViewportOrientation[oldAppID];
					delete mobileViewState[oldAppID];
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
					safeWebviewLoadURL(pooled, newSrc);
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

function safeWebviewLoadURL(wv, url) {
	if (!wv || !url) return;
	wv.__libroLastRequestedURL = url;
	// Avoid piling up overlapping Electron <webview>.loadURL calls. Those commonly
	// reject with ERR_ABORTED and can trigger GuestViewManager/MaxListeners noise.
	if (wv.__libroPendingURL === url) return;
	wv.__libroPendingURL = url;
	try {
		if (typeof wv.stop === 'function') wv.stop();
	} catch (err) {}
	try {
		var p = wv.loadURL(url);
		if (p && typeof p.catch === 'function') p.catch(function(err) {
			if (wv.__libroPendingURL === url) wv.__libroPendingURL = '';
			if (err && err.code && err.code !== 'ERR_ABORTED') console.warn('[libro-browser] loadURL failed:', err.code, url);
		});
	} catch (err) {
		wv.__libroPendingURL = '';
		if (err && err.code !== 'ERR_ABORTED') console.warn('[libro-browser] loadURL failed:', err && err.code ? err.code : err, url);
	}
}

function showBrowserError(wv, message, failedURL) {
	wv.__libroPendingURL = '';
	var host = wv.closest('[data-app-content]');
	if (!host) return;
	var previous = host.querySelector('.ws-browser-error'); if (previous) previous.remove();
	var panel = document.createElement('div'); panel.className = 'ws-browser-error'; panel.setAttribute('role', 'alert');
	var title = document.createElement('h2'); title.textContent = 'Could not load this page';
	var detail = document.createElement('p'); detail.textContent = message;
	var retry = document.createElement('button'); retry.className = 'ws-launch'; retry.textContent = 'Retry';
	retry.onclick = function() { panel.remove(); safeWebviewLoadURL(wv, failedURL || wv.__libroLastRequestedURL || wv.getAttribute('src') || 'about:blank'); };
	panel.append(title, detail, retry); host.appendChild(panel);
}

function updateBrowserNavigation(wv) {
	var frame = wv.closest('[data-app-id]'); if (!frame) return;
	try {
		var back = frame.querySelector('button[title="Back"]'); if (back) back.disabled = !wv.canGoBack();
		var forward = frame.querySelector('button[title="Forward"]'); if (forward) forward.disabled = !wv.canGoForward();
	} catch (_) {}
}

function bindWebviewEvents(wv) {
	if (wv.__libroEventsBound) return;
	wv.__libroEventsBound = true;
	wv.addEventListener('focus', function() {
		var appID = currentAppID(wv);
		if (appID && window.__libroSelectedApp !== appID && window.libroWorkspace) libroWorkspace.select(appID);
	});

	wv.addEventListener('dom-ready', function() {
		var appID = currentAppID(wv);
		if (!appID) return;
		ready[appID] = true;
		var q = queued[appID];
		if (q) { queued[appID] = null; q.forEach(function(fn){ fn(); }); }
		injectBrowserShortcuts(wv, appID);
		focusIfSelected(appID, wv);
	});

	// Update URL bar on navigation
	wv.addEventListener('did-navigate', function(e) {
		var appID = currentAppID(wv);
		if (!appID) return;
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
		updateBrowserNavigation(wv);
		if (e.url && !e.url.startsWith('data:')) __ws.call('app.url.set', {sid:wv.getAttribute('data-sid'), id:appID, url:e.url, observed:true});
		// Full-page navigation discards page JS — reset to normal mode
		applyBrowserMode(appID, 'normal');
	});
	wv.addEventListener('did-navigate-in-page', function(e) {
		if (!e.isMainFrame) return;
		var appID = currentAppID(wv);
		if (!appID) return;
		var inp = document.getElementById('urlinput-' + appID);
		if (inp && e.url) inp.value = e.url;
		updateBrowserNavigation(wv);
		if (e.url && !e.url.startsWith('data:')) __ws.call('app.url.set', {sid:wv.getAttribute('data-sid'), id:appID, url:e.url, observed:true});
	});

	// Re-inject browser shortcuts after full page navigation
	wv.addEventListener('did-finish-load', function() {
		var appID = currentAppID(wv);
		if (!appID) return;
		injectBrowserShortcuts(wv, appID);
	});

	// Listen for browser shortcut messages
	wv.addEventListener('console-message', function(e) {
		var appID = currentAppID(wv);
		if (!appID) return;
		var msg = e.message;
		if (msg === '__libro:urlpopup') { if (window.__libroOpenURLPopup) window.__libroOpenURLPopup(); }
		else if (msg === '__libro:reload') { if (window.__libroWvReload) window.__libroWvReload(appID); }
		else if (msg === '__libro:mobile' || msg === '__libro:viewport') { if (window.__libroToggleSelectedBrowserMobile) window.__libroToggleSelectedBrowserMobile(appID); }
		else if (msg === '__libro:viewportrotate') { if (window.__libroRotateSelectedBrowserViewport) window.__libroRotateSelectedBrowserViewport(appID); }
	});

	// Keep failures in host UI: error documents must not replace the requested
	// page, pollute history, or turn subframe failures into full-page failures.
	wv.addEventListener('did-fail-load', function(e) {
		if (e.errorCode === -3 || e.isMainFrame === false) return;
		showBrowserError(wv, e.errorDescription || 'Could not load this page', e.validatedURL);
	});
	wv.addEventListener('render-process-gone', function() {
		ready[currentAppID(wv)] = false;
		showBrowserError(wv, 'The browser process stopped. Reload to reconnect.');
	});
	wv.addEventListener('did-start-loading', function() {
		var frame = wv.closest('[data-app-id]');
		if (frame) { frame.dataset.browserLoading = 'true'; var error = frame.querySelector('.ws-browser-error'); if (error) error.remove(); }
	});
	wv.addEventListener('did-stop-loading', function() {
		wv.__libroPendingURL = '';
		var frame = wv.closest('[data-app-id]');
		if (frame) frame.dataset.browserLoading = 'false';
		updateBrowserNavigation(wv);
	});
	wv.addEventListener('page-title-updated', function(e) {
		var frame = wv.closest('[data-app-id]');
		if (frame && e.title) { frame.dataset.appName = e.title; if (window.libroWorkspace) libroWorkspace.refresh(); }
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
	// Plain browsers expose <webview> as an inert element. Registering it would
	// route navigation into a guest that never becomes ready instead of the iframe.
	if (!window.libroElectron) return;
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
						if (devtoolsPanelObservers[id]) {
							try { devtoolsPanelObservers[id].disconnect(); } catch (err) {}
							delete devtoolsPanelObservers[id];
						}
						stopDevtoolsBoundsSync(id);
						delete devtoolsPanelSyncers[id];
						delete window.__libroWebviews[id];
						delete initialized[id];
						delete ready[id];
						delete queued[id];
						delete browserModeState[id];
						delete mobileViewState[id];
						delete mobileViewportOrientation[id];
					}
				});
				if (node.tagName === 'WEBVIEW' && node.getAttribute('data-webview-app')) {
					if (pool && pool.contains(node)) return;
					var id = node.getAttribute('data-webview-app');
					if (devtoolsPanelObservers[id]) {
						try { devtoolsPanelObservers[id].disconnect(); } catch (err) {}
						delete devtoolsPanelObservers[id];
					}
					stopDevtoolsBoundsSync(id);
					delete devtoolsPanelSyncers[id];
					delete window.__libroWebviews[id];
					delete initialized[id];
					delete ready[id];
					delete queued[id];
					delete browserModeState[id];
					delete mobileViewState[id];
					delete mobileViewportOrientation[id];
				}
			});
		});
	});
cleanupObserver.observe(document.body, { childList: true, subtree: true });

function syncBrowserFallbackFrames(root) {
	var scope = root && root.querySelectorAll ? root : document;
	var useElectron = !!window.libroElectron;
	scope.querySelectorAll('webview[data-webview-app]').forEach(function(wv) {
		wv.style.display = useElectron ? 'inline-flex' : 'none';
	});
	scope.querySelectorAll('iframe[data-browser-iframe-app]').forEach(function(frame) {
		frame.style.display = useElectron ? 'none' : 'block';
		if (!useElectron && !frame.hasAttribute('src')) frame.setAttribute('src', frame.getAttribute('data-browser-src') || 'about:blank');
		if (!useElectron) updateBrowserFallbackUI(frame);
	});
}

function updateBrowserFallbackUI(frame) {
	var appID = frame.getAttribute('data-browser-iframe-app');
	var url = frame.getAttribute('src') || 'about:blank';
	var hasURL = url !== 'about:blank';
	var notice = document.querySelector('[data-browser-fallback-notice="' + appID + '"]');
	if (notice) notice.style.display = hasURL ? 'block' : 'none';
	var link = document.querySelector('[data-browser-external-link="' + appID + '"]');
	if (link) link.setAttribute('href', url);
	var host = frame.closest('[data-app-content]');
	var loading = host && host.querySelector('[data-webview-loading]');
	if (loading && hasURL) loading.remove();
}

function getBrowserFallbackFrame(appID) {
	return document.querySelector('iframe[data-browser-iframe-app="' + appID + '"]');
}

// Global helpers — safe to call before dom-ready (calls are queued)
window.__libroWvBack = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (wv) {
		whenReady(appID, function() { if (wv.canGoBack()) wv.goBack(); });
		return;
	}
	var frame = getBrowserFallbackFrame(appID);
	if (!frame) return;
	try { frame.contentWindow.history.back(); } catch (e) {}
};
window.__libroWvForward = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (wv) {
		whenReady(appID, function() { if (wv.canGoForward()) wv.goForward(); });
		return;
	}
	var frame = getBrowserFallbackFrame(appID);
	if (!frame) return;
	try { frame.contentWindow.history.forward(); } catch (e) {}
};
window.__libroWvReload = function(appID) {
	var wv = window.__libroWebviews[appID];
	if (wv) {
		whenReady(appID, function() { wv.reload(); });
		return;
	}
	var frame = getBrowserFallbackFrame(appID);
	if (!frame) return;
	try {
		frame.contentWindow.location.reload();
	} catch (e) {
		frame.setAttribute('src', frame.getAttribute('src') || 'about:blank');
	}
};
window.__libroWvZoom = function(appID, step) {
	var wv = window.__libroWebviews[appID];
	if (!wv) return;
	whenReady(appID, function() {
		var level = step === 0 ? 0 : Math.max(-5, Math.min(5, wv.getZoomLevel() + step));
		wv.setZoomLevel(level);
	});
};
window.__libroOpenNewTab = function(url) {
	if (url === 'about:blank') url = '';
	// Find the sid from any webview with a data-sid attribute
	var host = document.querySelector('webview[data-sid], iframe[data-browser-iframe-app][data-sid]');
	var sid = host ? host.getAttribute('data-sid') : 'default';
	__ws.call('app.start', {sid: sid, type: 'url', url: url, width: 'lg', side: 'right'});
};
window.__libroWvNavigate = function(appID, url) {
	var wv = window.__libroWebviews[appID];
	if (wv) {
		if (ready[appID]) {
			safeWebviewLoadURL(wv, url);
		} else {
			// Not ready yet — set src attribute to trigger initial load
			wv.setAttribute('src', url);
		}
		return;
	}
	var frame = getBrowserFallbackFrame(appID);
	if (!frame) return;
	frame.setAttribute('src', url || 'about:blank');
	updateBrowserFallbackUI(frame);
};

window.__libroNavigateAddress = function(appID, value) {
	var url = String(value || '').trim();
	if (!url) return false;
	if (!/^(https?|file):\/\//i.test(url)) {
		url = (/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\])(:|\/|$)/i.test(url) ? 'http://' : 'https://') + url;
	}
	try { var parsed = new URL(url); if (!['http:', 'https:', 'file:'].includes(parsed.protocol)) return false; url = parsed.href; }
	catch (_) { if (window.__libroShowToast) window.__libroShowToast('Invalid address', 'Enter a valid website or file URL.', 2200); return false; }
	var host = document.querySelector('[data-webview-app="'+appID+'"], [data-browser-iframe-app="'+appID+'"]');
	if (!host) return false;
	var input = document.getElementById('urlinput-'+appID); if (input) input.value = url;
	if(window.__libroRememberURL)window.__libroRememberURL(url);
	__ws.call('app.url.set', {sid:host.getAttribute('data-sid'), id:appID, url:url});
	return true;
};
syncBrowserFallbackFrames(document);
var runtimeFrameObserver = new MutationObserver(function(mutations) {
	mutations.forEach(function(mutation) {
		mutation.addedNodes.forEach(function(node) {
			syncBrowserFallbackFrames(node);
		});
	});
});
runtimeFrameObserver.observe(document.body, { childList: true, subtree: true });
})();
`
