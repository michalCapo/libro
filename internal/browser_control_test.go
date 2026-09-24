package libro

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBrowserMCPDiscovery(t *testing.T) {
	input := strings.NewReader("{invalid}\n" + `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" + `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"missing"}}` + "\n")
	var output bytes.Buffer
	if err := RunBrowserMCP(input, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d replies", len(lines))
	}
	var discovery struct {
		Result struct {
			Tools []struct {
				Name        string
				Description string
				InputSchema struct {
					Properties map[string]any
					Required   []string
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(lines[2]), &discovery); err != nil {
		t.Fatal(err)
	}
	if len(discovery.Result.Tools) != 3 {
		t.Fatal("browser tool missing")
	}
	application := discovery.Result.Tools[1]
	if application.Name != "application" || application.InputSchema.Properties["project"] == nil || !strings.Contains(application.Description, "project settings") {
		t.Fatal("application tool must explain the configured project command")
	}
	if !strings.Contains(application.Description, "Do not launch a separate application server") || !strings.Contains(lines[1], "Do not launch a separate application server") {
		t.Fatal("MCP initialization and application discovery must explain process ownership")
	}
	issues := discovery.Result.Tools[2]
	if issues.Name != "issues" || issues.InputSchema.Properties["status"] == nil || issues.InputSchema.Properties["id"] == nil {
		t.Fatal("issues tool missing status or id")
	}
	tool := discovery.Result.Tools[0]
	if tool.Name != "browser" || !strings.Contains(tool.Description, "thread's Libro browser panels") || tool.InputSchema.Properties["panel"] == nil {
		t.Fatal("tool must explain and target existing panels")
	}
	for _, field := range []string{"ref", "selector", "timeoutMs", "fullPage", "checked", "files", "values", "downloadId"} {
		if tool.InputSchema.Properties[field] == nil {
			t.Fatalf("browser tool missing %s parameter", field)
		}
	}
	if !strings.Contains(lines[3], `"isError":true`) {
		t.Fatal("unknown tool must fail")
	}
}

func TestBrowserCLIHelpDoesNotConnect(t *testing.T) {
	var output bytes.Buffer
	if err := RunBrowserCLI([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Never launch another browser") {
		t.Fatal("missing existing-panel guidance")
	}
	if err := RunBrowserCLI([]string{"not-json"}, &output); err == nil {
		t.Fatal("invalid command accepted")
	}
}

func TestBrowserScopeFollowsThreadOwner(t *testing.T) {
	original := sm
	sm = NewStateManager()
	t.Cleanup(func() { sm = original })
	sm.states["test"] = &AppState{
		ActiveProject: "first",
		Threads:       []Thread{{ID: "first", Project: "same"}, {ID: "second", Project: "same"}},
		Apps:          []Application{{ID: "browser"}, {ID: "agent"}},
		snapshots:     map[string]*projectSnapshot{"second": {Apps: []Application{{ID: "other"}}}},
	}
	first := browserScope("test", "browser")
	if first != browserScope("test", "agent") {
		t.Fatal("agent and browser must share their thread scope")
	}
	if first == browserScope("test", "other") {
		t.Fatal("threads in the same project must have separate profiles")
	}
	state := sm.states["test"]
	state.snapshots["first"] = &projectSnapshot{Apps: state.Apps}
	state.Apps = state.snapshots["second"].Apps
	state.ActiveProject = "second"
	if first != browserScope("test", "browser") {
		t.Fatal("scope changed when switching threads")
	}
	if first == browserScope("test", "missing") {
		t.Fatal("unknown panel inherited active scope")
	}
}

func TestOpenAgentBrowserInBackgroundThread(t *testing.T) {
	original := sm
	sm = NewStateManager()
	t.Cleanup(func() { sm = original })
	sm.states["test"] = &AppState{
		ActiveProject: "visible", Threads: []Thread{{ID: "visible"}, {ID: "background"}},
		Apps:      []Application{{ID: "visible-agent"}},
		snapshots: map[string]*projectSnapshot{"background": {Apps: []Application{{ID: "background-agent"}}}},
	}
	scope := browserScope("test", "background-agent")
	js, id, err := openAgentBrowser("test", scope, "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	state := sm.Get("test")
	if state.ActiveProject != "visible" || len(state.Apps) != 1 || state.SelectedIndex != 0 {
		t.Fatal("changed visible thread")
	}
	if browserScope("test", id) != scope {
		t.Fatal("browser created in wrong thread")
	}
	if !strings.Contains(js, "webview-"+id) {
		t.Fatal("browser was not rendered")
	}
	if _, _, err := openAgentBrowser("test", "unknown", "about:blank"); err == nil {
		t.Fatal("unknown scope accepted")
	}
	if _, _, err := openAgentBrowser("test", scope, "file:///private"); err == nil {
		t.Fatal("invalid URL accepted")
	}
}
