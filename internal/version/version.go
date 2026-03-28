package version

import (
	"os"
	"strings"
)

// Version is set at build time via -ldflags "-X libro/internal/version.Version=X.X.X"
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	if b, err := os.ReadFile("VERSION"); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			Version = v
		}
	}
}
