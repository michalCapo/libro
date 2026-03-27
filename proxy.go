package main

import (
	"bytes"
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
// It follows redirects and skips TLS verification to maximize compatibility.
var proxyClient = &http.Client{
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

	// Forward relevant request headers (strip Accept-Encoding so we get uncompressed responses for injection)
	for key, vals := range req.Header {
		lower := strings.ToLower(key)
		if hopByHopHeaders[lower] || lower == "host" || lower == "accept-encoding" {
			continue
		}
		for _, v := range vals {
			outReq.Header.Add(key, v)
		}
	}
	outReq.Header.Set("Host", parsed.Host)

	resp, err := proxyClient.Do(outReq)
	if err != nil {
		serveOfflinePage(w, targetURL)
		return
	}
	defer resp.Body.Close()

	// Final URL after redirects (used as base for relative URL resolution)
	finalURL := resp.Request.URL.String()

	// Copy response headers, stripping frame-blocking and hop-by-hop headers
	for key, vals := range resp.Header {
		lower := strings.ToLower(key)
		if frameBlockHeaders[lower] || hopByHopHeaders[lower] {
			continue
		}
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}

	// For HTML responses, inject proxy interceptor to route sub-requests and navigation through the proxy
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "failed to read response", http.StatusBadGateway)
			return
		}
		body = injectProxyScript(body, finalURL)
		w.Header().Del("Content-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
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
// It also neutralizes common redirect-based frame-busting patterns.
const frameBustNeutralizer = `<script>(function(){
try{
// Save real parent reference before overriding (used for URL tracking postMessage)
window.__libroP=window.parent;
// Override top/parent to point to self so frame-detection checks fail
Object.defineProperty(window,'top',{get:function(){return window},configurable:false});
Object.defineProperty(window,'parent',{get:function(){return window},configurable:false});
// Neutralize location-based frame busting (top.location = ...)
// By making top === self, these become no-ops, but as extra safety:
var _loc=window.location;
Object.defineProperty(window,'frameElement',{get:function(){return null},configurable:false});
// Block attempts to detect cross-origin parent via window.length
Object.defineProperty(window,'length',{get:function(){return 0},set:function(){},configurable:false});
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
var _f=window.fetch;window.fetch=function(i,o){if(typeof i==='string')i=t(i);else if(i instanceof Request)i=new Request(t(i.url),i);return _f.call(this,i,o);};
var _o=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(){if(arguments.length>1&&typeof arguments[1]==='string')arguments[1]=t(arguments[1]);return _o.apply(this,arguments);};
document.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('a[href]');if(!a)return;var h=a.getAttribute('href');if(!h||h.startsWith('#')||h.startsWith('javascript:'))return;e.preventDefault();try{var ru=new URL(h,B).href;window.__libroP.postMessage({libroNav:ru},'*');}catch(x){}window.location.href=t(h);},true);
document.addEventListener('submit',function(e){var f=e.target,a=f.getAttribute('action');if(a)f.setAttribute('action',t(a));},true);
var _w=window.open;window.open=function(u){if(u)arguments[0]=t(u);return _w.apply(this,arguments);};
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
