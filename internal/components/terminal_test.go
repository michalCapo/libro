package components

import (
	"net/http/httptest"
	"slices"
	"testing"

	"golang.org/x/net/websocket"
)

func TestMergeEnvironmentOverridesInheritedValues(t *testing.T) {
	got := mergeEnvironment([]string{"PATH=/bin", "OPENROUTER_API_KEY=old", "KEEP=yes"}, []string{"OPENROUTER_API_KEY=new", "MODEL=test", "MODEL=final"})
	want := []string{"PATH=/bin", "KEEP=yes", "OPENROUTER_API_KEY=new", "MODEL=final"}
	if !slices.Equal(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}

func TestTerminalSocketAcceptsOnlyLibroOrigin(t *testing.T) {
	for origin, ok := range map[string]bool{
		"http://localhost:8100": true,
		"http://localhost:3000": false,
		"https://example.com":   false,
		"":                      false,
	} {
		req := httptest.NewRequest("GET", "http://localhost:8100/terminal/ws/app-1", nil)
		req.Header.Set("Origin", origin)
		if err := sameOriginHandshake(&websocket.Config{Version: websocket.ProtocolVersionHybi13}, req); (err == nil) != ok {
			t.Errorf("origin %q accepted = %v, want %v", origin, err == nil, ok)
		}
	}
}
