package libro

// chromeClientJS returns the JavaScript that manages Chrome tab canvas views.
func chromeClientJS() string {
	return chromeClientScript
}

const chromeClientScript = `
(function(){
window.__chromeWS = window.__chromeWS || {};
var initialized = {};

function initChromeView(container) {
	var appID = container.getAttribute('data-chrome-app');
	if (!appID || initialized[appID]) return;
	initialized[appID] = true;

	var targetURL = container.getAttribute('data-chrome-url') || '';
	container.style.position = 'relative';
	container.style.overflow = 'hidden';

	// Create canvas — no CSS scaling, pixel-perfect
	var canvas = document.createElement('canvas');
	canvas.style.cssText = 'position:absolute;top:0;left:0;cursor:default;outline:none';
	container.appendChild(canvas);
	var ctx = canvas.getContext('2d');

	// Loading indicator
	var loading = document.createElement('div');
	loading.style.cssText = 'position:absolute;inset:0;display:flex;align-items:center;justify-content:center;color:#9ca3af;font-family:ui-monospace,monospace;font-size:12px;letter-spacing:0.05em;z-index:1';
	loading.textContent = 'Connecting to browser...';
	container.appendChild(loading);

	var ws, reconnectTimer, frameImg = new Image();
	var viewW = 0, viewH = 0;
	var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';

	// Measure container and update canvas + Chrome viewport
	function measure() {
		var rect = container.getBoundingClientRect();
		var w = Math.round(rect.width);
		var h = Math.round(rect.height);
		if (w < 10 || h < 10) return false; // not laid out yet
		if (w === viewW && h === viewH) return true; // no change
		viewW = w;
		viewH = h;
		canvas.width = viewW;
		canvas.height = viewH;
		canvas.style.width = viewW + 'px';
		canvas.style.height = viewH + 'px';
		return true;
	}

	function buildURL() {
		return proto + '//' + location.host + '/chrome/' + appID + '/?url=' + encodeURIComponent(targetURL) + '&w=' + viewW + '&h=' + viewH;
	}

	function connect() {
		if (!measure()) {
			// Container not ready, retry
			setTimeout(connect, 100);
			return;
		}
		ws = new WebSocket(buildURL());
		window.__chromeWS[appID] = ws;

		ws.onopen = function() {
			if (loading.parentNode) loading.remove();
		};

		ws.onmessage = function(e) {
			var msg;
			try { msg = JSON.parse(e.data); } catch(x) { return; }

			if (msg.t === 'f') {
				frameImg.onload = function() {
					ctx.drawImage(frameImg, 0, 0, canvas.width, canvas.height);
				};
				frameImg.src = 'data:image/jpeg;base64,' + msg.d;
			} else if (msg.t === 'nav') {
				var inp = document.getElementById('urlinput-' + appID);
				if (inp && msg.url) inp.value = msg.url;
			}
		};

		ws.onclose = function() {
			delete window.__chromeWS[appID];
			if (document.contains(container)) {
				reconnectTimer = setTimeout(function() {
					var inp = document.getElementById('urlinput-' + appID);
					if (inp) targetURL = inp.value;
					connect();
				}, 1000);
			}
		};
	}

	connect();

	// Coordinates: map canvas pixel position to Chrome viewport position (1:1 since no CSS scaling)
	function coords(e) {
		var r = canvas.getBoundingClientRect();
		return {
			x: Math.round(e.clientX - r.left),
			y: Math.round(e.clientY - r.top)
		};
	}

	function modifiers(e) {
		var m = 0;
		if (e.altKey) m |= 1;
		if (e.ctrlKey) m |= 2;
		if (e.metaKey) m |= 4;
		if (e.shiftKey) m |= 8;
		return m;
	}

	function cdpButton(e) {
		return e.button === 0 ? 'left' : e.button === 1 ? 'middle' : e.button === 2 ? 'right' : 'none';
	}

	function send(msg) {
		if (ws && ws.readyState === 1) ws.send(JSON.stringify(msg));
	}

	// Mouse events — attach to container (above the canvas and any overlays)
	container.addEventListener('mousedown', function(e) {
		e.preventDefault();
		e.stopPropagation();
		container.focus();
		// If this app is not selected, trigger selection first
		var appFrame = container.closest('[data-app-id]');
		if (appFrame && !appFrame.className.match(/border-teal-500(?!\/)/)) {
			var overlay = appFrame.querySelector('[data-click-overlay]');
			if (overlay) { overlay.click(); return; }
		}
		var c = coords(e);
		send({ t:'m', a:'mousePressed', x:c.x, y:c.y, b:cdpButton(e), cc:1, mod:modifiers(e) });
	}, true);

	container.addEventListener('mouseup', function(e) {
		e.preventDefault();
		e.stopPropagation();
		var c = coords(e);
		send({ t:'m', a:'mouseReleased', x:c.x, y:c.y, b:cdpButton(e), cc:1, mod:modifiers(e) });
	}, true);

	container.addEventListener('mousemove', function(e) {
		var c = coords(e);
		send({ t:'m', a:'mouseMoved', x:c.x, y:c.y, mod:modifiers(e) });
	}, true);

	container.addEventListener('dblclick', function(e) {
		e.preventDefault();
		e.stopPropagation();
		var c = coords(e);
		send({ t:'m', a:'mousePressed', x:c.x, y:c.y, b:cdpButton(e), cc:2, mod:modifiers(e) });
		send({ t:'m', a:'mouseReleased', x:c.x, y:c.y, b:cdpButton(e), cc:2, mod:modifiers(e) });
	}, true);

	// Scroll events
	container.addEventListener('wheel', function(e) {
		e.preventDefault();
		e.stopPropagation();
		var c = coords(e);
		send({ t:'m', a:'mouseWheel', x:c.x, y:c.y, dx:Math.round(e.deltaX), dy:Math.round(e.deltaY), mod:modifiers(e) });
	}, { passive: false, capture: true });

	container.addEventListener('contextmenu', function(e) {
		e.preventDefault();
		e.stopPropagation();
	}, true);

	// Keyboard events
	container.addEventListener('keydown', function(e) {
		// Let libro shortcuts pass through (Meta+H/L/J/K/D//)
		if (e.metaKey && /^[hljkdHLJKD\/]$/.test(e.key)) return;
		e.preventDefault();
		e.stopPropagation();
		// For printable chars: keyDown carries the text directly
		// For Enter/Tab: need rawKeyDown (no text) + char (with text)
		var text = e.key.length === 1 ? e.key : '';
		if (e.key === 'Enter' || e.key === 'Tab') {
			var ct = e.key === 'Enter' ? '\r' : '\t';
			send({ t:'k', a:'rawKeyDown', key:e.key, code:e.code, text:'', mod:modifiers(e), wk:e.keyCode, nk:e.keyCode });
			send({ t:'k', a:'char', key:e.key, code:'', text:ct, mod:modifiers(e), wk:e.keyCode, nk:e.keyCode });
		} else {
			send({ t:'k', a:'keyDown', key:e.key, code:e.code, text:text, mod:modifiers(e), wk:e.keyCode, nk:e.keyCode });
		}
	}, true);

	container.addEventListener('keyup', function(e) {
		if (e.metaKey && /^[hljkdHLJKD\/]$/.test(e.key)) return;
		e.preventDefault();
		e.stopPropagation();
		send({
			t:'k', a:'keyUp',
			key: e.key, code: e.code,
			mod: modifiers(e), wk: e.keyCode, nk: e.keyCode
		});
	}, true);

	// Resize: update canvas + Chrome viewport
	if (window.ResizeObserver) {
		var resizeTimer;
		var ro = new ResizeObserver(function(entries) {
			var entry = entries[0];
			if (!entry) return;
			var newW = Math.round(entry.contentRect.width);
			var newH = Math.round(entry.contentRect.height);
			if (newW > 10 && newH > 10 && (newW !== viewW || newH !== viewH)) {
				viewW = newW;
				viewH = newH;
				canvas.width = viewW;
				canvas.height = viewH;
				canvas.style.width = viewW + 'px';
				canvas.style.height = viewH + 'px';
				// Debounce resize commands to Chrome
				clearTimeout(resizeTimer);
				resizeTimer = setTimeout(function() {
					send({ t:'r', w:viewW, h:viewH });
				}, 200);
			}
		});
		ro.observe(container);
	}

	// Cleanup when container removed
	var observer = new MutationObserver(function() {
		if (!document.contains(container)) {
			observer.disconnect();
			clearTimeout(reconnectTimer);
			if (ws) ws.close();
			delete window.__chromeWS[appID];
			delete initialized[appID];
		}
	});
	observer.observe(document.body, { childList: true, subtree: true });
}

function initAll() {
	document.querySelectorAll('[data-chrome-app]').forEach(initChromeView);
}

initAll();

var bodyObserver = new MutationObserver(function() { initAll(); });
bodyObserver.observe(document.body, { childList: true, subtree: true });
})();
`
