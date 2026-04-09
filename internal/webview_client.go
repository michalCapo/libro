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
	document.addEventListener('mouseover', function(e) {
		var el = e.target;
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
var consoleFilter = {}; // appID -> {warn: bool, error: bool}
var consoleMaximized = {}; // appID -> bool

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
	// Update console panel count
	var countEl = document.getElementById('console-count-' + appID);
	if (countEl) {
		var msgs = consoleMessages[appID] || [];
		countEl.textContent = msgs.length > 0 ? msgs.length + ' messages' : '';
	}
}

function addConsoleMessage(appID, level, message, source, line) {
	if (!consoleMessages[appID]) consoleMessages[appID] = [];
	consoleMessages[appID].push({level: level, message: message, source: source, line: line});
	// Cap at 500 messages
	if (consoleMessages[appID].length > 500) consoleMessages[appID].shift();
	renderConsoleMessage(appID, level, message, source, line);
}

function shouldShowConsoleMessage(appID, level) {
	var f = getConsoleFilter(appID);
	// If both unchecked, show all (log + warn + error)
	if (!f.warn && !f.error) return true;
	// If warn checked, show warn (1) and error (2)
	if (f.warn && level >= 1) return true;
	// If error checked, show error (2)
	if (f.error && level >= 2) return true;
	// Show logs only if no filter is active
	return false;
}

function renderConsoleMessage(appID, level, message, source, line) {
	var container = document.getElementById('console-messages-' + appID);
	if (!container) return;
	if (!shouldShowConsoleMessage(appID, level)) return;
	var row = document.createElement('div');
	row.className = 'flex items-start gap-2 px-3 py-0.5 border-b border-stone-100 dark:border-stone-800 hover:bg-stone-100 dark:hover:bg-zinc-800/50';
	// Color by level: 0=log, 1=warning, 2=error
	var textColor = 'text-stone-600 dark:text-stone-300';
	var bgColor = '';
	var icon = '';
	if (level === 2) {
		textColor = 'text-red-600 dark:text-red-400';
		bgColor = ' bg-red-50/50 dark:bg-red-950/20';
		icon = 'error_outline';
	} else if (level === 1) {
		textColor = 'text-yellow-600 dark:text-yellow-400';
		bgColor = ' bg-yellow-50/50 dark:bg-yellow-950/20';
		icon = 'warning_amber';
	}
	if (bgColor) row.className += bgColor;
	var html = '';
	if (icon) {
		html += '<span class="material-icons-round text-[12px] mt-[3px] shrink-0 ' + textColor + '">' + icon + '</span>';
	} else {
		html += '<span class="w-3 shrink-0"></span>';
	}
	var safeMsg = message.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
	html += '<span class="flex-1 break-all whitespace-pre-wrap ' + textColor + '">' + safeMsg + '</span>';
	if (source) {
		var shortSource = source.split('/').pop();
		if (line) shortSource += ':' + line;
		html += '<span class="shrink-0 text-stone-400 dark:text-stone-500 text-[10px] mt-[2px]">' + shortSource.replace(/&/g,'&amp;').replace(/</g,'&lt;') + '</span>';
	}
	row.innerHTML = html;
	container.appendChild(row);
	// Auto-scroll to bottom
	container.scrollTop = container.scrollHeight;
}

function renderAllConsoleMessages(appID) {
	var container = document.getElementById('console-messages-' + appID);
	if (!container) return;
	container.innerHTML = '';
	var msgs = consoleMessages[appID] || [];
	for (var i = 0; i < msgs.length; i++) {
		var m = msgs[i];
		if (!shouldShowConsoleMessage(appID, m.level)) continue;
		renderConsoleMessage(appID, m.level, m.message, m.source, m.line);
	}
}

window.__libroToggleConsole = function(appID) {
	var panel = document.getElementById('console-panel-' + appID);
	if (!panel) return;
	if (panel.classList.contains('hidden')) {
		panel.classList.remove('hidden');
		panel.classList.add('flex');
		renderAllConsoleMessages(appID);
	} else {
		panel.classList.add('hidden');
		panel.classList.remove('flex');
	}
};

window.__libroClearConsole = function(appID) {
	consoleMessages[appID] = [];
	consoleCounts[appID] = {errors: 0};
	updateConsoleBadges(appID);
	var container = document.getElementById('console-messages-' + appID);
	if (container) container.innerHTML = '';
};

function getConsoleFilter(appID) {
	if (!consoleFilter[appID]) consoleFilter[appID] = {warn: false, error: true};
	return consoleFilter[appID];
}

function updateFilterCheckbox(appID, type) {
	var cb = document.getElementById('console-filter-' + type + '-' + appID);
	if (!cb) return;
	var f = getConsoleFilter(appID);
	var checked = f[type];
	cb.className = 'flex items-center justify-center w-4 h-4 rounded border cursor-pointer transition-colors ' +
		(checked ? 'bg-blue-500 border-blue-500' : 'border-stone-400 dark:border-stone-500 hover:border-stone-500 dark:hover:border-stone-400');
	cb.innerHTML = checked ? '<span class="material-icons-round text-[12px] text-white">check</span>' : '';
}

window.__libroToggleFilter = function(appID, type) {
	var f = getConsoleFilter(appID);
	f[type] = !f[type];
	updateFilterCheckbox(appID, type);
	renderAllConsoleMessages(appID);
};

window.__libroCopyConsole = function(appID) {
	var msgs = consoleMessages[appID] || [];
	var lines = [];
	for (var i = 0; i < msgs.length; i++) {
		var m = msgs[i];
		if (!shouldShowConsoleMessage(appID, m.level)) continue;
		var prefix = m.level === 2 ? '[ERROR] ' : (m.level === 1 ? '[WARN] ' : '');
		var src = m.source ? ' (' + m.source.split('/').pop() + (m.line ? ':' + m.line : '') + ')' : '';
		lines.push(prefix + m.message + src);
	}
	var joined = lines.join('\n');
	if (window.libroElectron && window.libroElectron.copyToClipboard) {
		window.libroElectron.copyToClipboard(joined);
	} else if (navigator.clipboard) {
		navigator.clipboard.writeText(joined);
	}
	if (window.__libroShowToast) {
		window.__libroShowToast('Copied', lines.length + ' messages', 1500);
	}
};

window.__libroToggleConsoleMaximize = function(appID) {
	var panel = document.getElementById('console-panel-' + appID);
	if (!panel) return;
	var maximized = consoleMaximized[appID] || false;
	if (maximized) {
		panel.style.height = '240px';
		panel.style.flex = '';
		consoleMaximized[appID] = false;
	} else {
		panel.style.height = '';
		panel.style.flex = '1';
		consoleMaximized[appID] = true;
	}
	// Update icon
	var icon = document.getElementById('console-maximize-icon-' + appID);
	if (icon) icon.textContent = consoleMaximized[appID] ? 'close_fullscreen' : 'open_in_full';
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
					delete consoleMessages[oldAppID];
					delete consoleCounts[oldAppID];
				}
				window.__libroWebviews[appID] = pooled;
				initialized[appID] = true;
				ready[appID] = true;
				// Remove loading indicator since the webview is already loaded
				var lp = pooled.closest('.flex.flex-col') || pooled.parentNode;
				var loading = lp && lp.querySelector('[data-webview-loading]');
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
		consoleCounts[appID] = {errors: 0};
		consoleMessages[appID] = [];
		updateConsoleBadges(appID);
		var container = document.getElementById('console-messages-' + appID);
		if (container) container.innerHTML = '';
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
	if (!consoleCounts[appID]) consoleCounts[appID] = {errors: 0};
	wv.addEventListener('console-message', function(e) {
		var msg = e.message;
		if (msg === '__libro:search') showSearchBar(appID);
		else if (msg === '__libro:findnext') findInPageNext(appID);
		else if (msg === '__libro:findprev') findInPagePrev(appID);
		else if (msg === '__libro:searchclear') clearSearch(appID);
		else if (msg === '__libro:enter') handleEnter(appID);
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
