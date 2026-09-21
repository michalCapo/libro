package libro

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// ConfigureInstance selects persistent storage and the HTTP port before startup.
// Environment variables carry the selection to Electron and child terminals.
func ConfigureInstance(name, port string) error {
	if name != "" && !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`).MatchString(name) {
		return fmt.Errorf("invalid instance name: use 1–64 lowercase letters, digits, hyphens or underscores")
	}
	if port == "" {
		port = "8100"
		if name != "" {
			port = "8101"
		}
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("invalid port %q: expected 1–65535", port)
	}
	if err := os.Setenv("LIBRO_INSTANCE", name); err != nil {
		return err
	}
	return os.Setenv("LIBRO_PORT", strconv.Itoa(number))
}

func Port() string {
	if port := os.Getenv("LIBRO_PORT"); port != "" {
		return port
	}
	return "8100"
}

func instanceDir(base string) string {
	if name := os.Getenv("LIBRO_INSTANCE"); name != "" {
		return filepath.Join(base, "instances", name)
	}
	return base
}
