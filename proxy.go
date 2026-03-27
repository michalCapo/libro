package main

import (
	"crypto/tls"
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
		http.Error(w, "proxy request failed: "+err.Error(), http.StatusBadGateway)
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
