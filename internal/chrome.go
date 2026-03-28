package libro

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	r "github.com/michalCapo/g-sui/ui"
)

const chromeDebugPort = 9222

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool {
		origin := req.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	},
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 256 * 1024,
}

// ChromeManager manages a single headless Chromium instance with multiple tabs.
type ChromeManager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	started bool
	tabs    map[string]*chromeTab
}

type chromeTab struct {
	appID    string
	targetID string
	wsURL    string

	cdpConn *websocket.Conn
	cdpMu   sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan json.RawMessage
	pendMu  sync.Mutex

	clientMu sync.Mutex
	client   *websocket.Conn

	pumpRunning atomic.Bool
}

func NewChromeManager() *ChromeManager {
	return &ChromeManager{
		tabs: make(map[string]*chromeTab),
	}
}

func (cm *ChromeManager) ensureStarted() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.started {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", chromeDebugPort))
		if err == nil {
			resp.Body.Close()
			return nil
		}
		cm.started = false
		cm.cmd = nil
	}

	home, _ := os.UserHomeDir()
	profileDir := filepath.Join(home, ".config", "libro", "chrome-profile")
	os.MkdirAll(profileDir, 0755)

	chromeBin := findChromeBinary()
	if chromeBin == "" {
		return fmt.Errorf("chromium/chrome not found in PATH")
	}

	cmd := exec.Command(chromeBin,
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%d", chromeDebugPort),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-client-side-phishing-detection",
		"--disable-hang-monitor",
		"--disable-popup-blocking",
		"--disable-prompt-on-repost",
		"--disable-sync",
		"--disable-translate",
		"--metrics-recording-only",
		"--no-service-autorun",
		"--password-store=basic",
		// Anti-bot-detection flags
		"--disable-blink-features=AutomationControlled",
		"--disable-features=IsolateOrigins,site-per-process",
		"--disable-infobars",
		fmt.Sprintf("--user-data-dir=%s", profileDir),
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start chrome: %w", err)
	}

	cm.cmd = cmd
	cm.started = true
	log.Printf("chrome: starting %s (pid=%d)", chromeBin, cmd.Process.Pid)

	ready := false
	for range 50 {
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", chromeDebugPort))
		if err == nil {
			resp.Body.Close()
			ready = true
			break
		}
	}
	if !ready {
		cmd.Process.Kill()
		cm.started = false
		cm.cmd = nil
		return fmt.Errorf("chrome did not start within timeout")
	}

	log.Printf("chrome: ready on debug port %d", chromeDebugPort)

	go func() {
		cmd.Wait()
		cm.mu.Lock()
		cm.started = false
		cm.cmd = nil
		cm.mu.Unlock()
		log.Printf("chrome: process exited")
	}()

	return nil
}

func (cm *ChromeManager) getOrCreateTab(appID string) (*chromeTab, error) {
	cm.mu.Lock()
	if tab, ok := cm.tabs[appID]; ok {
		cm.mu.Unlock()
		if tab.pumpRunning.Load() {
			return tab, nil
		}
		// CDP died — reconnect
		log.Printf("chrome: reconnecting CDP for app %s", appID)
		if err := tab.reconnectCDP(); err != nil {
			cm.mu.Lock()
			delete(cm.tabs, appID)
			cm.mu.Unlock()
			return cm.createNewTab(appID)
		}
		return tab, nil
	}
	cm.mu.Unlock()
	return cm.createNewTab(appID)
}

func (cm *ChromeManager) createNewTab(appID string) (*chromeTab, error) {
	createURL := fmt.Sprintf("http://127.0.0.1:%d/json/new?about:blank", chromeDebugPort)
	req, err := http.NewRequest("PUT", createURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create tab: %w", err)
	}
	defer resp.Body.Close()

	var info struct {
		ID                   string `json:"id"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	cdpConn, _, err := websocket.DefaultDialer.Dial(info.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect CDP: %w", err)
	}
	cdpConn.SetReadLimit(50 * 1024 * 1024) // 50MB for screencast frames

	tab := &chromeTab{
		appID:    appID,
		targetID: info.ID,
		wsURL:    info.WebSocketDebuggerURL,
		cdpConn:  cdpConn,
		pending:  make(map[int64]chan json.RawMessage),
	}
	tab.pumpRunning.Store(true)

	cm.mu.Lock()
	cm.tabs[appID] = tab
	cm.mu.Unlock()

	go tab.readPump()

	log.Printf("chrome: tab created for app %s (target=%s)", appID, info.ID)
	return tab, nil
}

func (t *chromeTab) reconnectCDP() error {
	t.cdpMu.Lock()
	if t.cdpConn != nil {
		t.cdpConn.Close()
	}
	t.cdpMu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(t.wsURL, nil)
	if err != nil {
		return err
	}
	conn.SetReadLimit(50 * 1024 * 1024)

	t.cdpMu.Lock()
	t.cdpConn = conn
	t.cdpMu.Unlock()

	t.pendMu.Lock()
	t.pending = make(map[int64]chan json.RawMessage)
	t.pendMu.Unlock()

	t.pumpRunning.Store(true)
	go t.readPump()
	return nil
}

func (cm *ChromeManager) closeTab(appID string) {
	cm.mu.Lock()
	tab, ok := cm.tabs[appID]
	if !ok {
		cm.mu.Unlock()
		return
	}
	delete(cm.tabs, appID)
	cm.mu.Unlock()

	if tab.cdpConn != nil {
		tab.cdpConn.Close()
	}

	closeReq, err := http.NewRequest("PUT", fmt.Sprintf("http://127.0.0.1:%d/json/close/%s", chromeDebugPort, tab.targetID), nil)
	if err == nil {
		closeResp, err := http.DefaultClient.Do(closeReq)
		if err == nil {
			closeResp.Body.Close()
		}
	}
	log.Printf("chrome: tab closed for app %s", appID)
}

// sendCDP sends a CDP command and waits for response.
func (t *chromeTab) sendCDP(method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)

	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}

	ch := make(chan json.RawMessage, 1)
	t.pendMu.Lock()
	t.pending[id] = ch
	t.pendMu.Unlock()

	t.cdpMu.Lock()
	err := t.cdpConn.WriteJSON(msg)
	t.cdpMu.Unlock()

	if err != nil {
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(10 * time.Second):
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
		return nil, fmt.Errorf("CDP timeout: %s", method)
	}
}

// fireCDP sends a CDP command without waiting.
func (t *chromeTab) fireCDP(method string, params any) {
	id := t.nextID.Add(1)
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	t.cdpMu.Lock()
	t.cdpConn.WriteJSON(msg)
	t.cdpMu.Unlock()
}

// readPump reads CDP messages from Chrome.
func (t *chromeTab) readPump() {
	defer func() {
		t.pumpRunning.Store(false)
		log.Printf("chrome: readPump exited for app %s", t.appID)
	}()

	for {
		_, data, err := t.cdpConn.ReadMessage()
		if err != nil {
			log.Printf("chrome: CDP read error for %s: %v", t.appID, err)
			return
		}

		var peek struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(data, &peek) != nil {
			continue
		}

		if peek.ID > 0 {
			t.pendMu.Lock()
			if ch, ok := t.pending[peek.ID]; ok {
				ch <- peek.Result
				delete(t.pending, peek.ID)
			}
			t.pendMu.Unlock()
		} else if peek.Method != "" {
			t.handleCDPEvent(peek.Method, peek.Params)
		}
	}
}

func (t *chromeTab) handleCDPEvent(method string, params json.RawMessage) {
	switch method {
	case "Page.screencastFrame":
		var p struct {
			Data      string `json:"data"`
			SessionID int    `json:"sessionId"`
			Metadata  struct {
				DeviceWidth  float64 `json:"deviceWidth"`
				DeviceHeight float64 `json:"deviceHeight"`
			} `json:"metadata"`
		}
		json.Unmarshal(params, &p)

		// Ack to get next frame
		t.fireCDP("Page.screencastFrameAck", map[string]int{"sessionId": p.SessionID})

		// Forward frame to client
		t.clientMu.Lock()
		if t.client != nil {
			msg := fmt.Sprintf(`{"t":"f","d":"%s","w":%v,"h":%v}`,
				p.Data, p.Metadata.DeviceWidth, p.Metadata.DeviceHeight)
			t.client.WriteMessage(websocket.TextMessage, []byte(msg))
		}
		t.clientMu.Unlock()

	case "Page.frameNavigated":
		var p struct {
			Frame struct {
				URL      string `json:"url"`
				ParentID string `json:"parentId"`
			} `json:"frame"`
		}
		json.Unmarshal(params, &p)

		if p.Frame.ParentID == "" {
			t.clientMu.Lock()
			if t.client != nil {
				t.client.WriteJSON(map[string]string{"t": "nav", "url": p.Frame.URL})
			}
			t.clientMu.Unlock()
		}
	}
}

func handleChromeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/chrome/")
	appID := strings.TrimSuffix(path, "/")

	targetURL := req.URL.Query().Get("url")
	vw, _ := strconv.Atoi(req.URL.Query().Get("w"))
	vh, _ := strconv.Atoi(req.URL.Query().Get("h"))
	if vw <= 0 {
		vw = 1280
	}
	if vh <= 0 {
		vh = 800
	}

	// Upgrade to WebSocket
	ws, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("chrome: WS upgrade failed: %v", err)
		return
	}
	defer ws.Close()
	ws.SetReadLimit(1024 * 1024) // 1MB for input messages

	log.Printf("chrome: client connected app=%s url=%s %dx%d", appID, targetURL, vw, vh)

	if err := cm.ensureStarted(); err != nil {
		log.Printf("chrome: start failed: %v", err)
		return
	}

	tab, err := cm.getOrCreateTab(appID)
	if err != nil {
		log.Printf("chrome: tab failed: %v", err)
		return
	}

	// Register client
	tab.clientMu.Lock()
	tab.client = ws
	tab.clientMu.Unlock()

	// Setup Chrome tab
	tab.sendCDP("Page.enable", nil)
	tab.sendCDP("Runtime.enable", nil)

	// Inject stealth script before any page loads to defeat bot detection
	tab.sendCDP("Page.addScriptToEvaluateOnNewDocument", map[string]string{
		"source": stealthJS,
	})

	// Set a normal user-agent (remove "HeadlessChrome")
	tab.sendCDP("Emulation.setUserAgentOverride", map[string]string{
		"userAgent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	})

	tab.sendCDP("Emulation.setDeviceMetricsOverride", map[string]any{
		"width": vw, "height": vh, "deviceScaleFactor": 1, "mobile": false,
	})

	if targetURL != "" && targetURL != "about:blank" {
		tab.sendCDP("Page.navigate", map[string]string{"url": targetURL})
	}

	_, err = tab.sendCDP("Page.startScreencast", map[string]any{
		"format": "png", "quality": 100,
		"maxWidth": vw, "maxHeight": vh, "everyNthFrame": 1,
	})
	if err != nil {
		log.Printf("chrome: startScreencast failed: %v", err)
		return
	}
	log.Printf("chrome: screencast started for app %s", appID)

	// Read client input
	for {
		var msg map[string]any
		if err := ws.ReadJSON(&msg); err != nil {
			break
		}

		msgType, _ := msg["t"].(string)
		switch msgType {
		case "m":
			p := map[string]any{
				"type": msg["a"], "x": msg["x"], "y": msg["y"],
				"button": orDefault(msg["b"], "none"),
			}
			if cc, ok := msg["cc"]; ok {
				p["clickCount"] = cc
			}
			if mod, ok := msg["mod"]; ok {
				p["modifiers"] = mod
			}
			if msg["a"] == "mouseWheel" {
				p["deltaX"] = orDefault(msg["dx"], float64(0))
				p["deltaY"] = orDefault(msg["dy"], float64(0))
			}
			tab.fireCDP("Input.dispatchMouseEvent", p)

		case "k":
			p := map[string]any{"type": msg["a"]}
			for _, k := range []string{"key", "code", "text"} {
				if v, ok := msg[k]; ok {
					p[k] = v
				}
			}
			if mod, ok := msg["mod"]; ok {
				p["modifiers"] = mod
			}
			if wk, ok := msg["wk"]; ok {
				p["windowsVirtualKeyCode"] = wk
			}
			if nk, ok := msg["nk"]; ok {
				p["nativeVirtualKeyCode"] = nk
			}
			tab.fireCDP("Input.dispatchKeyEvent", p)

		case "r":
			nw, nh := intVal(msg["w"]), intVal(msg["h"])
			if nw > 0 && nh > 0 {
				tab.sendCDP("Emulation.setDeviceMetricsOverride", map[string]any{
					"width": nw, "height": nh, "deviceScaleFactor": 1, "mobile": false,
				})
				tab.fireCDP("Page.stopScreencast", nil)
				tab.sendCDP("Page.startScreencast", map[string]any{
					"format": "png", "quality": 100,
					"maxWidth": nw, "maxHeight": nh, "everyNthFrame": 1,
				})
			}

		case "nav":
			if u, ok := msg["url"].(string); ok && u != "" {
				tab.sendCDP("Page.navigate", map[string]string{"url": u})
			}

		case "back":
			tab.fireCDP("Runtime.evaluate", map[string]string{"expression": "history.back()"})
		case "fwd":
			tab.fireCDP("Runtime.evaluate", map[string]string{"expression": "history.forward()"})
		case "reload":
			tab.fireCDP("Page.reload", nil)
		}
	}

	tab.fireCDP("Page.stopScreencast", nil)
	tab.clientMu.Lock()
	if tab.client == ws {
		tab.client = nil
	}
	tab.clientMu.Unlock()
	log.Printf("chrome: client disconnected for app %s", appID)
}

func orDefault(val any, def any) any {
	if val == nil {
		return def
	}
	return val
}

func intVal(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func findChromeBinary() string {
	for _, name := range []string{"chromium-browser", "chromium", "google-chrome-stable", "google-chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// stealthJS is injected into every page to remove headless Chrome detection markers.
const stealthJS = `
// Remove webdriver flag
Object.defineProperty(navigator, 'webdriver', {get: () => false});

// Fix chrome object
window.chrome = {runtime: {}, loadTimes: function(){return{}}, csi: function(){return{}}};

// Fix permissions query
const origQuery = window.Permissions.prototype.query;
window.Permissions.prototype.query = function(params) {
  if (params.name === 'notifications') {
    return Promise.resolve({state: Notification.permission});
  }
  return origQuery.call(this, params);
};

// Fix plugins (headless has 0)
Object.defineProperty(navigator, 'plugins', {
  get: () => [1, 2, 3, 4, 5].map(() => ({
    description: '', filename: '', length: 0, name: ''
  }))
});

// Fix languages
Object.defineProperty(navigator, 'languages', {get: () => ['sk-SK', 'sk', 'en-US', 'en']});

// Remove CDP Runtime.enable markers
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Array;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Promise;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Symbol;
`

func registerChrome(app *r.App) {
	app.GET("/chrome/", handleChromeHTTP)
}
