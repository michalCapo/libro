package main

import (
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
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

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

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
    background:#0c0e12;color:#a1a7b4;overflow:hidden;
  }
  .card{
    text-align:center;max-width:380px;padding:2.5rem 2rem;
    animation:fade-up .5s ease-out both;
  }
  .icon-wrap{position:relative;width:80px;height:80px;margin:0 auto 2rem}
  .icon-ring{
    position:absolute;inset:-8px;border-radius:50%%;
    border:1.5px solid rgba(20,184,166,.2);
    animation:pulse-ring 2.8s ease-in-out infinite;
  }
  .icon-circle{
    width:80px;height:80px;border-radius:50%%;
    background:linear-gradient(135deg,rgba(20,184,166,.12),rgba(16,185,129,.06));
    border:1px solid rgba(20,184,166,.18);
    display:flex;align-items:center;justify-content:center;
    position:relative;
  }
  .icon-circle svg{width:32px;height:32px;stroke:#2dd4bf;stroke-width:1.5;fill:none}
  h1{
    font-size:.8rem;font-weight:600;letter-spacing:.12em;text-transform:uppercase;
    color:#5b6270;margin-bottom:.75rem;
  }
  .url{
    font-size:.75rem;color:#2dd4bf;background:rgba(20,184,166,.08);
    border:1px solid rgba(20,184,166,.12);border-radius:6px;
    padding:.45rem .85rem;display:inline-block;margin-bottom:1.5rem;
    max-width:100%%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
    word-break:break-all;
  }
  .msg{font-size:.78rem;line-height:1.65;color:#6b7280;margin-bottom:1.75rem}
  .retry-row{display:flex;align-items:center;justify-content:center;gap:.55rem}
  .dots span{
    display:inline-block;width:4px;height:4px;border-radius:50%%;background:#2dd4bf;
    animation:dot-blink 1.2s infinite;
  }
  .dots span:nth-child(2){animation-delay:.15s}
  .dots span:nth-child(3){animation-delay:.3s}
  .retry-text{font-size:.68rem;color:#4b5563;letter-spacing:.04em}
  .countdown{font-variant-numeric:tabular-nums;color:#2dd4bf}
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
  var s=5,el=document.getElementById('cd');
  var t=setInterval(function(){
    s--;el.textContent=s;
    if(s<=0){clearInterval(t);location.reload();}
  },1000);
})();
</script>
</body>
</html>`
