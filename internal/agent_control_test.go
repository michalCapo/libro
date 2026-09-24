package libro

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPDiscovery(t *testing.T) {
	input := strings.NewReader("{invalid}\n" + `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" + `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"browser"}}` + "\n")
	var output bytes.Buffer
	if err := RunMCP(input, &output); err != nil {
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
		t.Fatal("expected only application and issues tools")
	}
	application := discovery.Result.Tools[0]
	if application.Name != "application" || application.InputSchema.Properties["project"] != nil || len(application.InputSchema.Properties) != 1 || !strings.Contains(application.Description, "project settings") {
		t.Fatal("application tool must explain the configured project command")
	}
	if !strings.Contains(application.Description, "Do not launch a separate application server") || !strings.Contains(lines[1], "Do not launch a separate application server") {
		t.Fatal("MCP initialization and application discovery must explain process ownership")
	}
	issues := discovery.Result.Tools[1]
	if issues.Name != "issues" || issues.InputSchema.Properties["status"] == nil || issues.InputSchema.Properties["id"] == nil {
		t.Fatal("issues tool missing status or id")
	}
	if !strings.Contains(lines[3], `"isError":true`) {
		t.Fatal("unknown tool must fail")
	}
}
