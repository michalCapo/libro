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
			Tools []struct{ Name string }
		}
	}
	if err := json.Unmarshal([]byte(lines[2]), &discovery); err != nil {
		t.Fatal(err)
	}
	if len(discovery.Result.Tools) != 3 {
		t.Fatalf("got %d tools", len(discovery.Result.Tools))
	}
	for i, name := range []string{"application", "notes", "children"} {
		if discovery.Result.Tools[i].Name != name {
			t.Fatalf("tool %d = %s, want %s", i, discovery.Result.Tools[i].Name, name)
		}
	}
	if !strings.Contains(lines[3], `"isError":true`) {
		t.Fatal("unknown tool must fail")
	}
}
