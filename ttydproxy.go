package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// ttydNavScript is injected into proxied ttyd HTML pages so that
// Meta+H / Meta+L keyboard shortcuts work even when the terminal
// iframe has focus.  It sends a postMessage to the parent window
// which the main keyboard handler picks up.
const ttydNavScript = `<script>
document.addEventListener('keydown',function(e){
if(e.metaKey&&(e.key==='h'||e.key==='H')){e.preventDefault();window.parent.postMessage({type:'libro-nav',dir:'left'},'*');}
if(e.metaKey&&(e.key==='l'||e.key==='L')){e.preventDefault();window.parent.postMessage({type:'libro-nav',dir:'right'},'*');}
});
</script>`

func registerTtydProxy(app *r.App) {
	app.GET("/ttyd/", handleTtydProxy)
}

// parseTtydPath extracts port and remainder from a ttyd proxy path.
// Input: /ttyd/{port}/rest/of/path → port, /rest/of/path
func parseTtydPath(urlPath string) (int, string, error) {
	path := strings.TrimPrefix(urlPath, "/ttyd/")
	slashIdx := strings.Index(path, "/")
	var portStr, remainder string
	if slashIdx >= 0 {
		portStr = path[:slashIdx]
		remainder = path[slashIdx:]
	} else {
		portStr = path
		remainder = "/"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("invalid port: %s", portStr)
	}
	return port, remainder, nil
}

func handleTtydProxy(w http.ResponseWriter, req *http.Request) {
	port, remainder, err := parseTtydPath(req.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// WebSocket upgrade — proxy at TCP level for reliability
	if isWebSocketUpgrade(req) {
		proxyWebSocket(w, req, port, remainder)
		return
	}

	// Regular HTTP — use ReverseProxy with script injection
	target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	rawQuery := req.URL.RawQuery

	proxy := &httputil.ReverseProxy{
		Director: func(outReq *http.Request) {
			outReq.URL.Scheme = target.Scheme
			outReq.URL.Host = target.Host
			outReq.URL.Path = remainder
			outReq.URL.RawQuery = rawQuery
			outReq.Host = target.Host
		},
		ModifyResponse: func(resp *http.Response) error {
			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				return nil
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return err
			}

			modified := injectNavScript(body)
			resp.Body = io.NopCloser(bytes.NewReader(modified))
			resp.ContentLength = int64(len(modified))
			resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Content-Security-Policy")

			return nil
		},
	}

	proxy.ServeHTTP(w, req)
}

func isWebSocketUpgrade(req *http.Request) bool {
	for _, v := range req.Header["Upgrade"] {
		if strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
}

// proxyWebSocket hijacks the client connection and pipes raw bytes
// to/from the ttyd backend, which is more reliable than relying on
// httputil.ReverseProxy for WebSocket upgrades.
func proxyWebSocket(w http.ResponseWriter, req *http.Request, port int, path string) {
	backend := fmt.Sprintf("localhost:%d", port)

	backConn, err := net.Dial("tcp", backend)
	if err != nil {
		http.Error(w, "ttyd backend unavailable", http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		backConn.Close()
		http.Error(w, "websocket hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		backConn.Close()
		return
	}

	// Reconstruct the original HTTP upgrade request for the backend.
	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s%s HTTP/1.1\r\n", req.Method, path, queryString(req))
	req.Header.Set("Host", backend)
	_ = req.Header.Write(&reqBuf)
	reqBuf.WriteString("\r\n")
	_, _ = backConn.Write(reqBuf.Bytes())

	// Flush any buffered data from the client to backend
	if clientBuf.Reader.Buffered() > 0 {
		buffered := make([]byte, clientBuf.Reader.Buffered())
		_, _ = clientBuf.Read(buffered)
		_, _ = backConn.Write(buffered)
	}

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() { io.Copy(backConn, clientConn); done <- struct{}{} }()
	go func() { io.Copy(clientConn, backConn); done <- struct{}{} }()
	<-done
	clientConn.Close()
	backConn.Close()
}

func queryString(req *http.Request) string {
	if req.URL.RawQuery != "" {
		return "?" + req.URL.RawQuery
	}
	return ""
}

// injectNavScript inserts the keyboard navigation script before </body>,
// or appends it at the end if no closing body tag is found.
func injectNavScript(body []byte) []byte {
	script := []byte(ttydNavScript)
	idx := bytes.LastIndex(bytes.ToLower(body), []byte("</body>"))
	if idx >= 0 {
		result := make([]byte, 0, len(body)+len(script))
		result = append(result, body[:idx]...)
		result = append(result, script...)
		result = append(result, body[idx:]...)
		return result
	}
	return append(body, script...)
}
