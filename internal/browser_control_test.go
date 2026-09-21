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
	if len(discovery.Result.Tools) != 2 {
		t.Fatal("browser tool missing")
	}
	application := discovery.Result.Tools[1]
	if application.Name != "application" || application.InputSchema.Properties["project"] == nil || !strings.Contains(application.Description, "project settings") {
		t.Fatal("application tool must explain the configured project command")
	}
	tool := discovery.Result.Tools[0]
	if tool.Name != "browser" || !strings.Contains(tool.Description, "existing Libro browser panel") || tool.InputSchema.Properties["panel"] == nil {
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
