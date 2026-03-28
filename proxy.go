package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// proxyClient is a shared HTTP client for proxying requests.
// It does NOT follow redirects — we rewrite them to go through the proxy.
var proxyClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

// hopByHopHeaders are headers that should not be forwarded by proxies.
var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// frameBlockHeaders are response headers that prevent iframe embedding.
var frameBlockHeaders = map[string]bool{
	"x-frame-options":         true,
	"content-security-policy": true,
}

// registerProxy registers the /proxy handler on the g-sui app.
func registerProxy(app *r.App) {
	app.GET("/proxy", handleProxy)
	app.POST("/proxy", handleProxy)
}

func handleProxy(w http.ResponseWriter, req *http.Request) {
	targetURL := req.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	// Build the outgoing request
	outReq, err := http.NewRequestWithContext(req.Context(), req.Method, targetURL, req.Body)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	// Forward relevant request headers
	for key, vals := range req.Header {
		lower := strings.ToLower(key)
		if hopByHopHeaders[lower] || lower == "host" {
			continue
		}
		// Strip Accept-Encoding so we get uncompressed responses for HTML injection.
		// For non-HTML we'll pass through compressed responses as-is.
		if lower == "accept-encoding" {
			continue
		}
		for _, v := range vals {
			outReq.Header.Add(key, v)
		}
	}

	// Set proper headers so the target site sees a normal browser request
	outReq.Header.Set("Host", parsed.Host)
	outReq.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")
	outReq.Header.Set("Origin", parsed.Scheme+"://"+parsed.Host)

	// Accept compressed responses — we'll decompress when needed
	outReq.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := proxyClient.Do(outReq)
	if err != nil {
		serveOfflinePage(w, targetURL)
		return
	}
	defer resp.Body.Close()

	// Handle redirects — rewrite Location header to go through proxy
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc != "" {
			absLoc := resolveURL(loc, targetURL)
			w.Header().Set("Location", "/proxy?url="+url.QueryEscape(absLoc))
			w.WriteHeader(resp.StatusCode)
			return
		}
	}

	// Copy response headers, stripping frame-blocking and hop-by-hop headers
	for key, vals := range resp.Header {
		lower := strings.ToLower(key)
		if frameBlockHeaders[lower] || hopByHopHeaders[lower] {
			continue
		}
		// Rewrite Set-Cookie: strip Domain and Secure flags so cookies work through proxy
		if lower == "set-cookie" {
			for _, v := range vals {
				rewritten := rewriteSetCookie(v)
				w.Header().Add(key, rewritten)
			}
			continue
		}
		// Strip Location header (already handled above for redirects)
		if lower == "location" {
			continue
		}
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}

	ct := resp.Header.Get("Content-Type")

	// For HTML responses, decompress if needed and inject proxy interceptor
	if strings.Contains(ct, "text/html") {
		body, err := readResponseBody(resp)
		if err != nil {
			http.Error(w, "failed to read response", http.StatusBadGateway)
			return
		}

		finalURL := targetURL
		body = injectProxyScript(body, finalURL)

		w.Header().Del("Content-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	// For CSS responses, rewrite url() references
	if strings.Contains(ct, "text/css") {
		body, err := readResponseBody(resp)
		if err != nil {
			http.Error(w, "failed to read response", http.StatusBadGateway)
			return
		}

		body = rewriteCSSURLs(body, targetURL)

		w.Header().Del("Content-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	// For all other content types, stream through as-is (including compression)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// readResponseBody reads and decompresses the response body based on Content-Encoding.
func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	case "deflate":
		reader = flate.NewReader(resp.Body)
	}
	return io.ReadAll(reader)
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(ref, base string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(refURL).String()
}

// rewriteSetCookie strips Domain, Secure, and SameSite attributes from Set-Cookie
// so cookies work when proxied through localhost.
func rewriteSetCookie(cookie string) string {
	parts := strings.Split(cookie, ";")
	var kept []string
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		lower := strings.ToLower(trimmed)
		if i == 0 {
			kept = append(kept, part)
			continue
		}
		// Strip attributes that break cross-origin cookie usage
		if strings.HasPrefix(lower, "domain") ||
			strings.HasPrefix(lower, "secure") ||
			strings.HasPrefix(lower, "samesite") {
			continue
		}
		// Rewrite path to root so cookies are sent on all proxy requests
		if strings.HasPrefix(lower, "path") {
			kept = append(kept, " Path=/")
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, ";") + "; SameSite=Lax"
}

// rewriteCSSURLs rewrites url() references in CSS to go through the proxy.
var reCSSURL = regexp.MustCompile(`url\(\s*(['"]?)([^)'"]+)['"]?\s*\)`)

func rewriteCSSURLs(css []byte, baseURL string) []byte {
	return reCSSURL.ReplaceAllFunc(css, func(match []byte) []byte {
		sub := reCSSURL.FindSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		quote := string(sub[1])
		rawURL := strings.TrimSpace(string(sub[2]))

		// Skip data URIs, blob URIs, and already-proxied URLs
		if strings.HasPrefix(rawURL, "data:") ||
			strings.HasPrefix(rawURL, "blob:") ||
			strings.HasPrefix(rawURL, "#") ||
			strings.Contains(rawURL, "/proxy?url=") {
			return match
		}

		abs := resolveURL(rawURL, baseURL)
		return []byte(fmt.Sprintf("url(%s/proxy?url=%s%s)", quote, url.QueryEscape(abs), quote))
	})
}

// Patterns for stripping frame-busting meta tags from HTML
var (
	// <meta http-equiv="X-Frame-Options" ...>
	reMetaXFO = regexp.MustCompile(`(?i)<meta[^>]+http-equiv\s*=\s*["']?X-Frame-Options["']?[^>]*>`)
	// <meta http-equiv="Content-Security-Policy" ...> that contain frame-ancestors
	reMetaCSP = regexp.MustCompile(`(?i)<meta[^>]+http-equiv\s*=\s*["']?Content-Security-Policy["']?[^>]*>`)
)

// stripFrameBustingMeta removes <meta> tags that enforce frame restrictions
func stripFrameBustingMeta(body []byte) []byte {
	body = reMetaXFO.ReplaceAll(body, nil)
	body = reMetaCSP.ReplaceAll(body, nil)
	return body
}

// injectProxyScript inserts a <base> tag, frame-buster neutralizer, and JavaScript interceptor into HTML responses.
// The interceptor rewrites fetch, XHR, link clicks, and form submissions to route through /proxy.
func injectProxyScript(body []byte, baseURL string) []byte {
	// First strip meta tags that block framing
	body = stripFrameBustingMeta(body)

	baseJSON, _ := json.Marshal(baseURL)
	injection := fmt.Sprintf(`<base href="%s">`, html.EscapeString(baseURL)) +
		frameBustNeutralizer +
		fmt.Sprintf(proxyInterceptScript, string(baseJSON))

	lower := bytes.ToLower(body)
	if idx := bytes.Index(lower, []byte("<head")); idx != -1 {
		if closeIdx := bytes.IndexByte(body[idx:], '>'); closeIdx != -1 {
			pos := idx + closeIdx + 1
			result := make([]byte, 0, len(body)+len(injection))
			result = append(result, body[:pos]...)
			result = append(result, []byte(injection)...)
			result = append(result, body[pos:]...)
			return result
		}
	}
	return append([]byte(injection), body...)
}

// frameBustNeutralizer is injected before all other scripts to prevent frame-busting.
// It makes window.top and window.parent return window itself, so checks like
// `if (top !== self)` or `if (window !== window.top)` evaluate to false.
// It also neutralizes common redirect-based frame-busting patterns and blocks Service Workers.
const frameBustNeutralizer = `<script>(function(){
try{
// Save real parent reference before overriding (used for URL tracking postMessage)
window.__libroP=window.parent;
// Override top/parent to point to self so frame-detection checks fail
Object.defineProperty(window,'top',{get:function(){return window},configurable:false});
Object.defineProperty(window,'parent',{get:function(){return window},configurable:false});
// Neutralize frameElement detection
Object.defineProperty(window,'frameElement',{get:function(){return null},configurable:false});
// Block attempts to detect cross-origin parent via window.length
Object.defineProperty(window,'length',{get:function(){return 0},set:function(){},configurable:false});
// Block Service Worker registration — SW intercepts break proxied requests
if(navigator.serviceWorker){
Object.defineProperty(navigator,'serviceWorker',{get:function(){return{register:function(){return Promise.reject()},getRegistration:function(){return Promise.resolve(undefined)},getRegistrations:function(){return Promise.resolve([])}}}});
}
// Neutralize document.domain assignments (used for frame-busting)
try{Object.defineProperty(document,'domain',{get:function(){return location.hostname},set:function(){},configurable:false});}catch(e){}
// Block visibility API checks (some sites pause in hidden iframes)
try{
Object.defineProperty(document,'hidden',{get:function(){return false},configurable:false});
Object.defineProperty(document,'visibilityState',{get:function(){return'visible'},configurable:false});
}catch(e){}
}catch(e){}
})();</script>`

// proxyInterceptScript is injected into proxied HTML to route sub-requests through /proxy.
// %s is replaced with the JSON-encoded base URL of the original page.
const proxyInterceptScript = `<script>(function(){
var B=%s,H=location.origin,P=H+'/proxy?url=';
function t(u){
if(!u||typeof u!=='string')return u;
if(u.startsWith(P)||u.startsWith('/proxy?url=')||u.startsWith('#')||u.startsWith('javascript:')||u.startsWith('data:')||u.startsWith('blob:'))return u;
try{var a=new URL(u,B).href;if(a.startsWith('http://')||a.startsWith('https://'))return P+encodeURIComponent(a);}catch(e){}
return u;
}

// Intercept fetch
var _f=window.fetch;window.fetch=function(i,o){
if(typeof i==='string')i=t(i);
else if(i instanceof Request)i=new Request(t(i.url),i);
return _f.call(this,i,o);
};

// Intercept XMLHttpRequest
var _o=XMLHttpRequest.prototype.open;
XMLHttpRequest.prototype.open=function(){
if(arguments.length>1&&typeof arguments[1]==='string')arguments[1]=t(arguments[1]);
return _o.apply(this,arguments);
};

// Intercept link clicks — navigate through proxy and notify parent of URL change
document.addEventListener('click',function(e){
var a=e.target.closest&&e.target.closest('a[href]');
if(!a)return;
var h=a.getAttribute('href');
if(!h||h.startsWith('#')||h.startsWith('javascript:'))return;
e.preventDefault();
try{var ru=new URL(h,B).href;window.__libroP.postMessage({libroNav:ru},'*');}catch(x){}
window.location.href=t(h);
},true);

// Intercept form submissions
document.addEventListener('submit',function(e){
var f=e.target,a=f.getAttribute('action');
if(a)f.setAttribute('action',t(a));
},true);

// Intercept window.open
var _w=window.open;window.open=function(u){
if(u)arguments[0]=t(u);
return _w.apply(this,arguments);
};

// Intercept dynamic image/script/link/iframe src/href assignments
function hookSetter(proto,prop,rewrite){
var desc=Object.getOwnPropertyDescriptor(proto,prop);
if(!desc||!desc.set)return;
var origSet=desc.set;
Object.defineProperty(proto,prop,{
get:desc.get,
set:function(v){return origSet.call(this,rewrite(v));},
configurable:true,enumerable:desc.enumerable
});
}
try{
hookSetter(HTMLImageElement.prototype,'src',t);
hookSetter(HTMLScriptElement.prototype,'src',t);
hookSetter(HTMLIFrameElement.prototype,'src',t);
hookSetter(HTMLSourceElement.prototype,'src',t);
hookSetter(HTMLSourceElement.prototype,'srcset',function(v){
if(!v)return v;
return v.split(',').map(function(s){var p=s.trim().split(/\s+/);p[0]=t(p[0]);return p.join(' ');}).join(', ');
});
hookSetter(HTMLImageElement.prototype,'srcset',function(v){
if(!v)return v;
return v.split(',').map(function(s){var p=s.trim().split(/\s+/);p[0]=t(p[0]);return p.join(' ');}).join(', ');
});
}catch(e){}

// Rewrite existing srcset attributes in the DOM once loaded
document.addEventListener('DOMContentLoaded',function(){
document.querySelectorAll('[srcset]').forEach(function(el){
var v=el.getAttribute('srcset');
if(v){
var nv=v.split(',').map(function(s){var p=s.trim().split(/\s+/);p[0]=t(p[0]);return p.join(' ');}).join(', ');
el.setAttribute('srcset',nv);
}
});
});

// Intercept History API to track URL changes for the address bar
var _ps=history.pushState,_rs=history.replaceState;
history.pushState=function(){
var r=_ps.apply(this,arguments);
try{window.__libroP.postMessage({libroNav:arguments[2]?new URL(arguments[2],B).href:B},'*');}catch(x){}
return r;
};
history.replaceState=function(){
var r=_rs.apply(this,arguments);
try{window.__libroP.postMessage({libroNav:arguments[2]?new URL(arguments[2],B).href:B},'*');}catch(x){}
return r;
};

// Intercept WebSocket to rewrite proxy URLs back to original
var _WS=window.WebSocket;
window.WebSocket=function(u,p){
// If the WS URL points to our proxy host, rewrite to the actual target
try{
var wu=new URL(u);
if(wu.hostname===location.hostname&&wu.port===location.port){
var orig=new URL(B);
wu.hostname=orig.hostname;wu.port=orig.port||'';wu.protocol=orig.protocol==='https:'?'wss:':'ws:';
u=wu.href;
}
}catch(e){}
return p!==undefined?new _WS(u,p):new _WS(u);
};
window.WebSocket.prototype=_WS.prototype;
window.WebSocket.CONNECTING=_WS.CONNECTING;
window.WebSocket.OPEN=_WS.OPEN;
window.WebSocket.CLOSING=_WS.CLOSING;
window.WebSocket.CLOSED=_WS.CLOSED;

// Intercept EventSource
var _ES=window.EventSource;
if(_ES){
window.EventSource=function(u,o){return new _ES(t(u),o);};
window.EventSource.prototype=_ES.prototype;
}

// Notify parent of current URL
try{window.__libroP.postMessage({libroNav:B},'*');}catch(x){}
})();</script>`

// serveOfflinePage renders a styled HTML page when the proxied target is unreachable.
func serveOfflinePage(w http.ResponseWriter, targetURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, offlinePageHTML, html.EscapeString(targetURL), html.EscapeString(targetURL))
}

const offlinePageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Offline</title>
<style>
  *,*::before,*::after{margin:0;padding:0;box-sizing:border-box}
  :root{
    --bg:#ffffff;--text:#6b7280;--heading:#9ca3af;
    --accent:#0d9488;--accent-light:rgba(13,148,136,.08);--accent-border:rgba(13,148,136,.18);
    --ring:rgba(13,148,136,.15);--dot:#0d9488;
    --icon-bg:linear-gradient(135deg,rgba(13,148,136,.1),rgba(16,185,129,.05));
    --icon-stroke:#0d9488;
  }
  .dark{
    --bg:#0c0e12;--text:#6b7280;--heading:#5b6270;
    --accent:#2dd4bf;--accent-light:rgba(20,184,166,.08);--accent-border:rgba(20,184,166,.12);
    --ring:rgba(20,184,166,.2);--dot:#2dd4bf;
    --icon-bg:linear-gradient(135deg,rgba(20,184,166,.12),rgba(16,185,129,.06));
    --icon-stroke:#2dd4bf;
  }
  @keyframes pulse-ring{
    0%%{transform:scale(.85);opacity:.4}
    50%%{transform:scale(1);opacity:.15}
    100%%{transform:scale(.85);opacity:.4}
  }
  @keyframes fade-up{
    from{opacity:0;transform:translateY(12px)}
    to{opacity:1;transform:translateY(0)}
  }
  @keyframes dot-blink{
    0%%,80%%,100%%{opacity:.25}
    40%%{opacity:1}
  }
  body{
    min-height:100vh;display:flex;align-items:center;justify-content:center;
    font-family:'SF Mono',SFMono-Regular,ui-monospace,'Cascadia Mono','Segoe UI Mono','Liberation Mono',Menlo,Monaco,Consolas,monospace;
    background:var(--bg);color:var(--text);overflow:hidden;
    transition:background .15s,color .15s;
  }
  .card{
    text-align:center;max-width:380px;padding:2.5rem 2rem;
    animation:fade-up .5s ease-out both;
  }
  .icon-wrap{position:relative;width:80px;height:80px;margin:0 auto 2rem}
  .icon-ring{
    position:absolute;inset:-8px;border-radius:50%%;
    border:1.5px solid var(--ring);
    animation:pulse-ring 2.8s ease-in-out infinite;
  }
  .icon-circle{
    width:80px;height:80px;border-radius:50%%;
    background:var(--icon-bg);
    border:1px solid var(--accent-border);
    display:flex;align-items:center;justify-content:center;
    position:relative;
  }
  .icon-circle svg{width:32px;height:32px;stroke:var(--icon-stroke);stroke-width:1.5;fill:none}
  h1{
    font-size:.8rem;font-weight:600;letter-spacing:.12em;text-transform:uppercase;
    color:var(--heading);margin-bottom:.75rem;
  }
  .url{
    font-size:.75rem;color:var(--accent);background:var(--accent-light);
    border:1px solid var(--accent-border);border-radius:6px;
    padding:.45rem .85rem;display:inline-block;margin-bottom:1.5rem;
    max-width:100%%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
    word-break:break-all;
  }
  .msg{font-size:.78rem;line-height:1.65;color:var(--text);margin-bottom:1.75rem}
  .retry-row{display:flex;align-items:center;justify-content:center;gap:.55rem}
  .dots span{
    display:inline-block;width:4px;height:4px;border-radius:50%%;background:var(--dot);
    animation:dot-blink 1.2s infinite;
  }
  .dots span:nth-child(2){animation-delay:.15s}
  .dots span:nth-child(3){animation-delay:.3s}
  .retry-text{font-size:.68rem;color:var(--text);letter-spacing:.04em}
  .countdown{font-variant-numeric:tabular-nums;color:var(--accent)}
</style>
</head>
<body>
<div class="card">
  <div class="icon-wrap">
    <div class="icon-ring"></div>
    <div class="icon-circle">
      <svg viewBox="0 0 24 24" stroke-linecap="round" stroke-linejoin="round">
        <path d="M1 1l22 22M16.72 11.06A10.94 10.94 0 0 1 19 12.55M5 12.55a10.94 10.94 0 0 1 5.17-2.39M10.71 5.05A16 16 0 0 1 22.56 9M1.42 9a15.91 15.91 0 0 1 4.7-2.88M8.53 16.11a6 6 0 0 1 6.95 0M12 20h.01"/>
      </svg>
    </div>
  </div>
  <h1>Application Offline</h1>
  <div class="url" title="%s">%s</div>
  <p class="msg">This service is not responding right now.<br>It may still be starting up or has stopped.</p>
  <div class="retry-row">
    <div class="dots"><span></span><span></span><span></span></div>
    <span class="retry-text">retrying in <span class="countdown" id="cd">5</span>s</span>
  </div>
</div>
<script>
(function(){
  function applyTheme(){
    try{
      var dark=window.parent.document.documentElement.classList.contains('dark');
      document.documentElement.classList.toggle('dark',dark);
    }catch(e){
      if(window.matchMedia&&window.matchMedia('(prefers-color-scheme:dark)').matches){
        document.documentElement.classList.add('dark');
      }
    }
  }
  applyTheme();
  var s=5,el=document.getElementById('cd');
  var t=setInterval(function(){
    s--;el.textContent=s;
    if(s<=0){clearInterval(t);location.reload();}
  },1000);
})();
</script>
</body>
</html>`
