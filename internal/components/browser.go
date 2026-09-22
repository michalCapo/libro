package components

// BrowserJS returns the JavaScript that manages Electron webview elements.
// It initializes webview tags, handles navigation events, and provides
// back/forward/reload/navigate functions via the webview DOM API.
// It also injects browser-mode scrolling, page tools, address, reload, and viewport shortcuts.
func BrowserJS() string {
	return browserScript
}

const browserScript = `
(function(){
window.__libroWebviews = window.__libroWebviews || {};
var ready = {};       // appID -> true when dom-ready has fired
var queued = {};      // appID -> [fn, fn, ...] calls waiting for dom-ready
var initialized = {};
var agentControlPaused = false;
var agentControlEnabled = true;
function renderAgentControlState(state) {
    if (state) { agentControlPaused = !!state.paused; agentControlEnabled = state.enabled !== false; }
    document.querySelectorAll('[data-browser-control]').forEach(function(button) {
        button.disabled = !window.libroElectron || !agentControlEnabled;
        var label = !agentControlEnabled ? 'Agent browser control is off in Settings' : agentControlPaused ? 'Resume agent browser control' : 'Pause agent browser control';
        button.setAttribute('title', label);
        button.setAttribute('aria-label', label);
        button.setAttribute('aria-pressed', String(agentControlPaused));
        var icon = button.querySelector('i');
        var value = agentControlPaused ? 'play_arrow' : 'pause';
        if (icon && icon.textContent !== value) icon.textContent = value;
    });
}
window.__libroApplyBrowserControlSetting = function(enabled) {
    agentControlEnabled = !!enabled;
    var select = document.getElementById('browser-control-enabled');
    if (select) select.value = enabled ? 'on' : 'off';
    renderAgentControlState();
    if (!window.libroElectron || !window.libroElectron.setBrowserControlEnabled) return;
    window.libroElectron.setBrowserControlEnabled(!!enabled).then(renderAgentControlState).catch(function(error) {
        if (window.__libroShowToast) window.__libroShowToast('Browser control setting not applied', error.message, 2600);
    });
};
window.__libroBrowserControlPause = function(action) {
    if (!window.libroElectron || !window.libroElectron.browserControlState) return;
    window.libroElectron.browserControlState(action).then(function(state) {
        renderAgentControlState(state);
        if (window.__libroShowToast) window.__libroShowToast(state.paused ? 'Agent browser control paused' : 'Agent browser control resumed', state.paused ? 'Use the play button to resume. Your browser still works normally.' : '', 2600);
    }).catch(function(error) {
        if (window.__libroShowToast) window.__libroShowToast('Browser control unavailable', error.message, 2600);
    });
};
if (window.libroElectron && window.libroElectron.onBrowserControlState) {
    window.libroElectron.onBrowserControlState(renderAgentControlState);
    window.libroElectron.browserControlState('status').then(renderAgentControlState).catch(function() {});
}


// --- Browser shortcuts script injected into webview guest pages ---
var browserShortcutsScript = '(' + function(){
	if(window.__libroBrowserShortcuts) return;
	window.__libroBrowserShortcuts = true;
	var pageToolMode = '';
	var pageToolHighlight = null;
	var pageToolOverlay = null;
	var pageToolStart = null;
	var pageToolPrompt = null;
	var pageToolListenersBound = false;

	function pageToolMessage(kind, payload) {
		try { console.log('__libro:page-tool:' + kind + ':' + JSON.stringify(payload)); } catch (err) {}
	}
	function removePageToolHighlight() {
		if (pageToolHighlight && pageToolHighlight.parentNode) pageToolHighlight.parentNode.removeChild(pageToolHighlight);
		pageToolHighlight = null;
	}
	function pageToolRect(rect, className) {
		if (!rect) return;
		if (!pageToolHighlight) {
			pageToolHighlight = document.createElement('div');
			pageToolHighlight.setAttribute('aria-hidden', 'true');
			pageToolHighlight.setAttribute('popover', 'manual');
			pageToolHighlight.style.inset = 'auto';
			pageToolHighlight.style.margin = '0';
			pageToolHighlight.style.padding = '0';
			pageToolHighlight.style.overflow = 'visible';
			pageToolHighlight.style.position = 'fixed';
			pageToolHighlight.style.pointerEvents = 'none';
			pageToolHighlight.style.zIndex = '2147483647';
			pageToolHighlight.style.boxSizing = 'border-box';
			var modals = document.querySelectorAll('dialog:modal');
			(modals[modals.length - 1] || document.documentElement).appendChild(pageToolHighlight);
			pageToolHighlight.showPopover();
		}
		pageToolHighlight.className = className || '';
		pageToolHighlight.style.left = Math.max(0, rect.left) + 'px';
		pageToolHighlight.style.top = Math.max(0, rect.top) + 'px';
		pageToolHighlight.style.width = Math.max(0, rect.width) + 'px';
		pageToolHighlight.style.height = Math.max(0, rect.height) + 'px';
		pageToolHighlight.style.border = '2px solid #2563eb';
		pageToolHighlight.style.background = 'rgba(37,99,235,.10)';
		pageToolHighlight.style.boxShadow = '0 0 0 1px rgba(255,255,255,.8), 0 2px 12px rgba(37,99,235,.18)';
	}
	function pageToolSelector(el) {
		if (!el || !el.tagName) return '';
		var parts = [];
		while (el && el.nodeType === 1 && parts.length < 6) {
			var part = el.tagName.toLowerCase();
			if (el.id) part += '#' + String(el.id).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
			else if (el.classList && el.classList.length) part += '.' + Array.prototype.slice.call(el.classList).filter(function(name) { return /^[a-zA-Z0-9_-]+$/.test(name); }).slice(0, 2).join('.');
			parts.unshift(part);
			el = el.parentElement;
		}
		return parts.join(' > ');
	}
	function pageToolElementData(el) {
		if (!el || el.nodeType !== 1) return null;
		var rect = el.getBoundingClientRect ? el.getBoundingClientRect() : null;
		return {
			selector: pageToolSelector(el),
			viewportRect: rect ? {x: Math.round(rect.left), y: Math.round(rect.top), width: Math.round(rect.width), height: Math.round(rect.height)} : null
		};
	}
	function pageToolAreaData(rect) {
		return {
			x: Math.round(rect.left), y: Math.round(rect.top),
			width: Math.round(rect.width), height: Math.round(rect.height)
		};
	}
	function pageToolPromptClose() {
		if (pageToolPrompt && pageToolPrompt.parentNode) pageToolPrompt.parentNode.removeChild(pageToolPrompt);
		pageToolPrompt = null;
	}
	function pageToolPromptAnchor(payload) {
		var rect = payload && payload.kind === 'element' && payload.element ? payload.element.viewportRect : payload && payload.area;
		if (!rect) return {left: window.innerWidth / 2, top: window.innerHeight / 2, right: window.innerWidth / 2, bottom: window.innerHeight / 2};
		var left = Number(rect.x) || 0;
		var top = Number(rect.y) || 0;
		var width = Math.max(0, Number(rect.width) || 0);
		var height = Math.max(0, Number(rect.height) || 0);
		return {left:left, top:top, right:left + width, bottom:top + height};
	}
	function pageToolPromptOpen(payload, appURL) {
		pageToolPromptClose();
		var panel = document.createElement('div');
		panel.setAttribute('popover', 'manual');
		panel.setAttribute('role', 'dialog');
		panel.setAttribute('aria-label', payload && payload.kind === 'area' ? 'Describe selected page area' : 'Describe selected page element');
		panel.style.position = 'fixed';
		panel.style.inset = 'auto';
		panel.style.margin = '0';
		panel.style.padding = '0';
		panel.style.border = '0';
		panel.style.background = 'transparent';
		panel.style.overflow = 'visible';
		panel.style.zIndex = '2147483647';
		panel.style.boxSizing = 'border-box';
		panel.style.width = 'min(380px, calc(100vw - 24px))';
		panel.style.maxWidth = 'calc(100vw - 24px)';
		panel.style.fontFamily = '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
		panel.style.fontSize = '13px';
		panel.style.lineHeight = '1.4';
		var root = panel.attachShadow ? panel.attachShadow({mode:'open'}) : panel;
		var style = document.createElement('style');
		style.textContent = '.card{box-sizing:border-box;padding:12px;border:1px solid #cbd5e1;border-radius:10px;background:#fff;color:#1f2937;box-shadow:0 8px 28px rgba(15,23,42,.24)}' +
			'.header{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:8px;font-weight:600}' +
			'.close{width:26px;height:26px;padding:0;border:0;border-radius:6px;background:transparent;color:#64748b;font-size:20px;line-height:1;cursor:pointer}' +
			'.close:hover{background:#f1f5f9;color:#1f2937}' +
			'.summary{margin-bottom:8px;color:#64748b;font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}' +
			'input[type=text]{display:block;box-sizing:border-box;width:100%;height:44px;margin:0;padding:9px 10px;border:1px solid #94a3b8;border-radius:7px;background:#fff;color:#1f2937;font:inherit;font-size:16px;line-height:1.45;outline:none}' +
			'input[type=text]:focus{border-color:#2563eb;box-shadow:0 0 0 2px rgba(37,99,235,.18)}' +
			'.actions{display:flex;justify-content:flex-end;gap:8px;margin-top:9px}' +
			'button.action{padding:7px 11px;border:1px solid #cbd5e1;border-radius:7px;background:#fff;color:#334155;font:inherit;cursor:pointer}' +
			'button.action.primary{border-color:#2563eb;background:#2563eb;color:#fff}' +
			'button.action:hover{filter:brightness(.97)}' +
			'@media (prefers-color-scheme: dark){.card{border-color:#4b5563;background:#202124;color:#f8fafc;box-shadow:0 8px 28px rgba(0,0,0,.5)}.close{color:#cbd5e1}.close:hover{background:#374151;color:#fff}.summary{color:#cbd5e1}input[type=text]{border-color:#64748b;background:#111827;color:#f8fafc}button.action{border-color:#64748b;background:#374151;color:#f8fafc}}';
		root.appendChild(style);
		var card = document.createElement('div'); card.className = 'card';
		var header = document.createElement('div'); header.className = 'header';
		var title = document.createElement('span'); title.textContent = payload && payload.kind === 'area' ? 'Describe this page area' : 'Describe this element';
		var close = document.createElement('button'); close.type = 'button'; close.className = 'close'; close.textContent = '×'; close.setAttribute('aria-label', 'Close prompt');
		var summary = document.createElement('div'); summary.className = 'summary';
		if (payload && payload.kind === 'area') summary.textContent = 'Selected rectangle · Screenshot attached';
		else if (payload && payload.element) summary.textContent = 'Selected ' + (payload.element.selector || 'element') + (payload.screenshot ? ' · Screenshot attached' : '');
		else summary.textContent = appURL || 'Selected page element';
		var input = document.createElement('input'); input.type = 'text'; input.placeholder = 'What should the agent do with this?'; input.setAttribute('aria-label', 'Page tool prompt'); input.autocomplete = 'off';
		var actions = document.createElement('div'); actions.className = 'actions';
		var cancel = document.createElement('button'); cancel.type = 'button'; cancel.className = 'action'; cancel.textContent = 'Cancel';
		var send = document.createElement('button'); send.type = 'button'; send.className = 'action primary'; send.textContent = 'Send to agent';
		header.append(title, close); actions.append(cancel, send); card.append(header, summary, input, actions); root.appendChild(card);
		// Popovers paint above page dialogs. Keep the prompt inside the active
		// modal as well, so the browser does not make its input inert.
		var modals = document.querySelectorAll('dialog:modal');
		(modals[modals.length - 1] || document.documentElement).appendChild(panel);
		panel.showPopover();
		pageToolPrompt = panel;
		var anchor = pageToolPromptAnchor(payload);
		function position() {
			if (!panel.parentNode) return;
			var box = panel.getBoundingClientRect();
			var margin = 12;
			var left = Math.max(margin, Math.min(anchor.left, window.innerWidth - box.width - margin));
			var below = anchor.bottom + 10;
			var above = anchor.top - box.height - 10;
			var top = below;
			if (below + box.height > window.innerHeight - margin && above >= margin) top = above;
			top = Math.max(margin, Math.min(top, window.innerHeight - box.height - margin));
			panel.style.left = Math.round(left) + 'px';
			panel.style.top = Math.round(top) + 'px';
		}
		function stopEvent(event) { event.stopPropagation(); }
		panel.addEventListener('pointerdown', stopEvent);
		panel.addEventListener('click', stopEvent);
		panel.addEventListener('keydown', function(event) { if (event.key === 'Escape') { event.preventDefault(); pageToolPromptClose(); } });
		close.onclick = function() { pageToolPromptClose(); };
		cancel.onclick = function() { pageToolPromptClose(); };
		send.onclick = function(event) {
			event.preventDefault();
			var request = String(input.value || '').trim();
			if (!request) { input.focus(); return; }
			pageToolMessage('selection', {kind: payload && payload.kind || 'element', url: appURL || window.location.href, request: request, element: payload && payload.element || null, area: payload && payload.area || null, screenshot: payload && payload.screenshot || ''});
		};
		window.__libroPageToolResult = function(sent) {
			if (sent) pageToolPromptClose();
			else { summary.textContent = 'Agent is not ready. Your prompt is saved here; try again.'; input.focus(); }
		};
		input.addEventListener('keydown', function(event) {
			if (event.key !== 'Enter' || event.isComposing) return;
			event.preventDefault();
			send.click();
		});
		position();
		requestAnimationFrame(position);
		input.focus();
	}
	window.__libroPageToolPromptOpen = pageToolPromptOpen;
	function pageToolStop(mode) {
		pageToolMode = '';
		pageToolStart = null;
		removePageToolHighlight();
		if (pageToolOverlay && pageToolOverlay.parentNode) pageToolOverlay.parentNode.removeChild(pageToolOverlay);
		pageToolOverlay = null;
		document.documentElement.style.cursor = '';
		pageToolMessage('mode', {mode: '', previous: mode || ''});
	}
	function pageToolSetMode(mode) {
		if (mode !== 'annotate' && mode !== 'area') mode = '';
		if (pageToolMode === mode) { pageToolStop(mode); return; }
		if (pageToolMode) pageToolStop(pageToolMode);
		pageToolMode = mode;
		if (mode) {
			document.documentElement.style.cursor = 'crosshair';
			pageToolMessage('mode', {mode: mode});
		}
	}
	window.__libroSetPageToolMode = pageToolSetMode;
	window.__libroGetPageToolMode = function() { return pageToolMode; };
	function pageToolPointerOver(event) {
		if (pageToolMode !== 'annotate') return;
		var el = event.target && event.target.nodeType === 1 ? event.target : event.target && event.target.parentElement;
		if (!el || el === pageToolHighlight || (pageToolOverlay && pageToolOverlay.contains(el))) return;
		try { pageToolRect(el.getBoundingClientRect(), 'libro-page-tool-highlight'); } catch (err) {}
	}
	function pageToolPointerOut(event) {
		if (pageToolMode !== 'annotate') return;
		if (event.relatedTarget && event.relatedTarget.nodeType === 1) return;
		removePageToolHighlight();
	}
	function pageToolClick(event) {
		if (pageToolMode !== 'annotate') return;
		var el = event.target && event.target.nodeType === 1 ? event.target : event.target && event.target.parentElement;
		if (!el || el === pageToolHighlight || (pageToolOverlay && pageToolOverlay.contains(el))) return;
		event.preventDefault(); event.stopPropagation();
		var data = pageToolElementData(el);
		pageToolStop('annotate');
		pageToolPromptClose();
		requestAnimationFrame(function() { requestAnimationFrame(function() {
			pageToolMessage('capture-area', {kind: 'element', url: window.location.href, element: data});
		}); });
	}
	function pageToolPointerDown(event) {
		if (pageToolMode !== 'area' || event.button !== 0) return;
		if (pageToolOverlay && pageToolOverlay.contains(event.target)) return;
		event.preventDefault(); event.stopPropagation();
		pageToolStart = {x: event.clientX, y: event.clientY};
		try { if (event.target.setPointerCapture) event.target.setPointerCapture(event.pointerId); } catch (err) {}
		if (!pageToolOverlay) {
			pageToolOverlay = document.createElement('div');
			pageToolOverlay.setAttribute('aria-hidden', 'true');
			pageToolOverlay.style.position = 'fixed';
			pageToolOverlay.style.inset = '0';
			pageToolOverlay.style.zIndex = '2147483646';
			pageToolOverlay.style.cursor = 'crosshair';
			pageToolOverlay.style.background = 'rgba(37,99,235,.035)';
			pageToolOverlay.style.pointerEvents = 'none';
			document.documentElement.appendChild(pageToolOverlay);
		}
		pageToolRect({left:event.clientX, top:event.clientY, right:event.clientX, bottom:event.clientY, width:0, height:0}, 'libro-page-tool-area');
	}
	function pageToolPointerMove(event) {
		if (pageToolMode !== 'area' || !pageToolStart) return;
		event.preventDefault();
		var left = Math.min(pageToolStart.x, event.clientX), top = Math.min(pageToolStart.y, event.clientY);
		var right = Math.max(pageToolStart.x, event.clientX), bottom = Math.max(pageToolStart.y, event.clientY);
		pageToolRect({left:left, top:top, right:right, bottom:bottom, width:right-left, height:bottom-top}, 'libro-page-tool-area');
	}
	function pageToolPointerUp(event) {
		if (pageToolMode !== 'area' || !pageToolStart) return;
		event.preventDefault(); event.stopPropagation();
		var left = Math.min(pageToolStart.x, event.clientX), top = Math.min(pageToolStart.y, event.clientY);
		var right = Math.max(pageToolStart.x, event.clientX), bottom = Math.max(pageToolStart.y, event.clientY);
		var rect = {left:left, top:top, right:right, bottom:bottom, width:right-left, height:bottom-top};
		if (rect.width < 6 || rect.height < 6) { pageToolStart = null; return; }
		var data = pageToolAreaData(rect);
		try { if (event.target.releasePointerCapture) event.target.releasePointerCapture(event.pointerId); } catch (err) {}
		pageToolStop('area');
		pageToolPromptClose();
		requestAnimationFrame(function() { requestAnimationFrame(function() {
			pageToolMessage('capture-area', {kind: 'area', url: window.location.href, area: data});
		}); });
	}
	if (!pageToolListenersBound) {
		pageToolListenersBound = true;
		document.addEventListener('pointerover', pageToolPointerOver, true);
		document.addEventListener('pointerout', pageToolPointerOut, true);
		document.addEventListener('click', pageToolClick, true);
		document.addEventListener('pointerdown', pageToolPointerDown, true);
		document.addEventListener('pointermove', pageToolPointerMove, true);
		document.addEventListener('pointerup', pageToolPointerUp, true);
		document.addEventListener('pointercancel', pageToolPointerUp, true);
	}
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
			case 'a': pageToolMessage('activate', {mode:'annotate'}); break;
			case 'd': pageToolMessage('activate', {mode:'area'}); break;
			case 'Escape':
				if (pageToolMode) pageToolStop(pageToolMode);
				else handled = false;
				break;
			case 'g': window.scrollTo({top: 0, behavior: 'smooth'}); break;
			case 'G': window.scrollTo({top: document.documentElement.scrollHeight, behavior: 'smooth'}); break;
			case 'j': window.scrollBy({top: 800, behavior: 'smooth'}); break;
			case 'k': window.scrollBy({top: -800, behavior: 'smooth'}); break;
			case 'h': window.scrollBy({left: -480, behavior: 'smooth'}); break;
			case 'l': window.scrollBy({left: 480, behavior: 'smooth'}); break;
			case 'o': console.log('__libro:urlpopup'); break;
			case 'r': console.log('__libro:reload'); break;
			case '-': console.log('__libro:zoom-out'); break;
			case '=': console.log('__libro:zoom-in'); break;
			case '0': console.log('__libro:zoom-reset'); break;
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
var pageToolState = {}; // appID -> '' | 'annotate' | 'area'
var mobileViewState = {}; // appID -> viewport preview styles and resize observer
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

function pageToolButtonState(appID, mode) {
	pageToolState[appID] = mode || '';
	var frame = document.querySelector('[data-app-id="' + appID + '"]');
	if (!frame) return;
	frame.querySelectorAll('[data-page-tool]').forEach(function(button) {
		var active = (button.getAttribute('data-page-tool') || '') === (mode || '');
		button.setAttribute('data-active', String(active));
		button.setAttribute('aria-pressed', String(active));
	});
}

function pageToolWebview(appID) {
	return window.__libroWebviews[appID] || document.querySelector('iframe[data-browser-iframe-app="' + appID + '"]');
}

function executePageToolMode(appID, mode) {
	var wv = pageToolWebview(appID);
	if (!wv) return;
	var js = 'if(window.__libroSetPageToolMode)window.__libroSetPageToolMode(' + JSON.stringify(mode || '') + ');';
	if (wv.executeJavaScript) {
		whenReady(appID, function() { try { wv.executeJavaScript(js); } catch (err) {} });
		return;
	}
	try {
		if (wv.contentWindow && wv.contentWindow.eval) wv.contentWindow.eval(js);
	} catch (err) {}
}

window.__libroTogglePageTool = function(appID, mode) {
	appID = appID || window.__libroSelectedApp || '';
	if (!appID || !mode) return;
	var next = pageToolState[appID] === mode ? '' : mode;
	if (next && window.__libroEnsurePageToolAgent && !window.__libroEnsurePageToolAgent(appID, mode)) return;
	pageToolButtonState(appID, next);
	executePageToolMode(appID, next);
	var label = next === 'annotate' ? 'Annotate mode' : next === 'area' ? 'Area select mode' : 'Page tool off';
	var hint = next === 'annotate' ? 'Hover and click an element' : next === 'area' ? 'Drag a rectangle over the page' : 'Ready';
	if (window.__libroShowToast) window.__libroShowToast(label, hint, 1200);
};


function pageToolURL(appID, payload) {
	var url = payload && payload.url ? String(payload.url) : '';
	if (url) return url;
	var input = document.getElementById('urlinput-' + appID);
	return input ? String(input.value || '') : '';
}

function pageToolContext(payload, appID) {
	var url = pageToolURL(appID, payload);
	var lines = [
		'Please make the requested change to the page element or region below.',
		'',
		'Page URL: ' + url
	];
	if (payload && payload.kind === 'element' && payload.element) {
		lines.push('Target element path: ' + (payload.element.selector || '(none)'));
	}
	if (payload && payload.screenshot) lines.push((payload.kind === 'element' ? 'Selected-element screenshot: ' : 'Selected-area screenshot: ') + JSON.stringify(payload.screenshot), 'Open this image and use it with the user request and page URL.');
	return lines.join('\n');
}

async function receivePageToolMessage(appID, kind, rawPayload) {
	if (!appID) return;
	var payload = {};
	try { payload = JSON.parse(rawPayload || '{}') || {}; } catch (err) { return; }
	if (kind === 'activate') {
		if (payload.mode === 'annotate' || payload.mode === 'area') window.__libroTogglePageTool(appID, payload.mode);
		return;
	}
	if (kind === 'mode') {
		pageToolButtonState(appID, payload.mode || '');
		return;
	}
	if (kind === 'capture-area') {
		var guest = pageToolWebview(appID);
		try {
			if (!guest || !window.libroElectron || !window.libroElectron.capturePageArea) throw new Error('Open Libro desktop to capture page areas.');
			var rect = payload.kind === 'element' ? payload.element && payload.element.viewportRect : payload.area;
			payload.screenshot = await window.libroElectron.capturePageArea(guest.getWebContentsId(), rect);
			await guest.executeJavaScript('window.__libroPageToolPromptOpen(' + JSON.stringify(payload) + ', ' + JSON.stringify(payload.url) + ')');
		} catch (err) {
			if (window.__libroShowToast) window.__libroShowToast('Screenshot failed', 'Select the element or area again in Libro desktop.', 2400);
		}
		return;
	}
	if (kind === 'selection') {
		pageToolButtonState(appID, '');
		var request = String(payload.request || '').trim();
		if (!request) return;
		if ((payload.kind === 'area' || payload.kind === 'element') && !payload.screenshot) return;
		var targetLabel = payload.kind === 'area' ? 'Page area annotation' : 'HTML element annotation';
		var prompt = '\n\n--- BEGIN ' + targetLabel + ' ---\nUser request: ' + request + '\n\n' + pageToolContext(payload, appID) + '\n--- END ' + targetLabel + ' ---\n\n';
		var sent = window.__libroSendPageToolPrompt && window.__libroSendPageToolPrompt(prompt, !!window.__libroPageToolsAutoExecute);
		var wv = pageToolWebview(appID);
		if (wv && wv.executeJavaScript) wv.executeJavaScript('window.__libroPageToolResult && window.__libroPageToolResult(' + !!sent + ')').catch(function() {});
		if (window.__libroShowToast) {
			window.__libroShowToast(sent ? (window.__libroPageToolsAutoExecute ? 'Prompt sent to agent' : 'Prompt pasted to agent') : 'No active agent panel', sent ? '' : 'Start or select an agent and try again.', 1600);
		}
	}
}

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
	var guest = window.libroElectron ? window.__libroWebviews[appID] : getBrowserFallbackFrame(appID);
	if (!frame || !content || !guest) return false;
	var state = mobileViewState[appID];
	if (!state) {
		state = mobileViewState[appID] = {
			mode: 'normal',
			orientation: mobileViewportOrientation[appID] || 'portrait',
			previousFrameStyle: frame.getAttribute('style') || '',
			previousContentStyle: content.getAttribute('style') || '',
			previousWidth: currentAppWidth(appID),
			guest: guest,
			previousGuestStyle: guest.getAttribute('style') || ''
		};
	}
	if (mode === 'normal') {
		mobileViewportOrientation[appID] = state.orientation || mobileViewportOrientation[appID] || 'portrait';
		if (state.observer) state.observer.disconnect();
		guest.setAttribute('style', state.previousGuestStyle);
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
	frame.style.maxWidth = '100%';
	frame.style.height = '100%';
	frame.style.maxHeight = '100%';
	frame.style.alignSelf = 'center';
	content.style.overflow = 'hidden';
	// Scale the guest, not its viewport: responsive layouts keep the selected
	// dimensions while the complete preview fits above any docked DevTools.
	state.fit = function() {
		var host = guest.parentElement;
		if (!host.clientWidth || !host.clientHeight) return;
		var scale = Math.min(1, host.clientWidth / width, host.clientHeight / height);
		guest.style.position = 'absolute';
		guest.style.width = width + 'px';
		guest.style.height = height + 'px';
		guest.style.left = (host.clientWidth - width * scale) / 2 + 'px';
		guest.style.top = (host.clientHeight - height * scale) / 2 + 'px';
		guest.style.transformOrigin = 'top left';
		guest.style.transform = 'scale(' + scale + ')';
	};
	if (!state.observer) {
		state.observer = new ResizeObserver(function() { state.fit(); });
		state.observer.observe(guest.parentElement);
	}
	state.fit();
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
		if (document.querySelector('#url-popup:not(.hidden)')) return;
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
	if (visible) startDevtoolsBoundsSync(appID);
	else stopDevtoolsBoundsSync(appID);
}

function isDevtoolsPanelVisible(appID) {
	var panel = document.getElementById('devtools-panel-' + appID);
	return !!(panel && !panel.classList.contains('hidden') && panel.getClientRects && panel.getClientRects().length);
}

function devtoolsPanelBounds(appID) {
	var panel = document.getElementById('devtools-host-' + appID);
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
		if (document.querySelector('#url-popup:not(.hidden)')) return;
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
	if (initialized[appID] && window.__libroWebviews[appID] === wv) return;
	// A replacement must get its own guest and readiness state.
	ready[appID] = false;
	delete queued[appID];
	observeDevtoolsPanel(appID);

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
		pageToolButtonState(appID, '');
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
		if (msg && msg.indexOf('__libro:page-tool:') === 0) {
			var separator = msg.indexOf(':', '__libro:page-tool:'.length);
			receivePageToolMessage(appID, msg.slice('__libro:page-tool:'.length, separator), msg.slice(separator + 1));
		}
		else if (msg === '__libro:urlpopup') { if (window.__libroOpenURLPopup) window.__libroOpenURLPopup(); }
		else if (msg === '__libro:reload') { if (window.__libroWvReload) window.__libroWvReload(appID); }
		else if (msg === '__libro:zoom-out' || msg === '__libro:zoom-in' || msg === '__libro:zoom-reset') {
			window.__libroWvZoom(appID, msg === '__libro:zoom-reset' ? 0 : msg === '__libro:zoom-out' ? -1 : 1);
		}
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
	renderAgentControlState();
	// Plain browsers expose <webview> as an inert element. Registering it would
	// route navigation into a guest that never becomes ready instead of the iframe.
	if (!window.libroElectron) return;
	document.querySelectorAll('webview[data-webview-app]').forEach(initWebview);
}

initAll();

var bodyObserver = new MutationObserver(function() { initAll(); });
bodyObserver.observe(document.body, { childList: true, subtree: true });
window.addEventListener('resize', function() {
	Object.keys(window.__libroWebviews).forEach(function(appID) {
		updateDevtoolsBounds(appID);
	});
});

// Clean up only the removed instance, never its replacement.
var cleanupObserver = new MutationObserver(function(mutations) {
	Object.keys(mobileViewState).forEach(function(appID) {
		var state = mobileViewState[appID];
		if (state.guest.isConnected) return;
		if (state.observer) state.observer.disconnect();
		delete mobileViewState[appID];
	});
	mutations.forEach(function(m) {
		m.removedNodes.forEach(function(node) {
				if (node.nodeType !== 1) return;
				var wvs = node.querySelectorAll ? node.querySelectorAll('webview[data-webview-app]') : [];
				wvs.forEach(function(wv) {
					if (wv.isConnected) return;
					var id = wv.getAttribute('data-webview-app');
					if (id && window.__libroWebviews[id] === wv) {
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
						delete pageToolState[id];
						delete mobileViewState[id];
						delete mobileViewportOrientation[id];
					}
				});
				if (node.tagName === 'WEBVIEW' && node.getAttribute('data-webview-app')) {
					if (node.isConnected) return;
					var id = node.getAttribute('data-webview-app');
					if (window.__libroWebviews[id] !== node) return;
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
					delete pageToolState[id];
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
		var preview = mobileViewState[appID];
		if (preview) preview.fit();
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
