package libro

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	r "github.com/michalCapo/g-sui/ui"
)

// openRouterKey prefers the key saved in Settings, then Libro's environment.
func openRouterKey() string {
	dbMu.Lock()
	var key string
	if db != nil {
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'openrouter_api_key'`).Scan(&key)
	}
	dbMu.Unlock()
	if key = strings.TrimSpace(key); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
}

func savedOpenRouterKey() bool {
	dbMu.Lock()
	defer dbMu.Unlock()
	var key string
	return db != nil && db.QueryRow(`SELECT value FROM settings WHERE key = 'openrouter_api_key'`).Scan(&key) == nil && key != ""
}

// The saved key itself never goes back to the browser.
func voiceSettingsState() map[string]any {
	return map[string]any{"savedKey": savedOpenRouterKey()}
}

// An empty key keeps the saved one; clearKey removes it.
func setVoiceSettings(key string, clearKey bool) error {
	key = strings.TrimSpace(key)
	if strings.ContainsFunc(key, unicode.IsControl) || len(key) > 512 {
		return fmt.Errorf("enter a valid OpenRouter API key")
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if clearKey {
		_, err = tx.Exec(`DELETE FROM settings WHERE key = 'openrouter_api_key'`)
	} else if key != "" {
		_, err = tx.Exec(`INSERT INTO settings (key,value) VALUES ('openrouter_api_key',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func renderVoiceSettings() *r.Node {
	return r.El("section", "").Render(
		r.El("h2", "ws-shortcut-heading").Text("OpenRouter"),
		r.Div("ws-settings-group").Render(
			r.Div("ws-settings-row").Render(
				r.Div("ws-settings-copy").Render(
					r.El("label", "").Attr("for", "openrouter-key").Text("API key"),
					r.P("").ID("openrouter-key-help").Text("Required for voice typing, which uses GPT-4o Transcribe. Leave empty to keep the saved key. Without a saved key, Libro uses OPENROUTER_API_KEY from its environment."),
				),
				r.Div("ws-agent-command-row").Render(
					r.Input("ws-agent-command").ID("openrouter-key").Attr("type", "password").Attr("autocomplete", "new-password").Attr("spellcheck", "false").Attr("aria-describedby", "openrouter-key-help"),
					r.Button("ws-launch").Attr("type", "button").OnClick(r.UnsafeJS("libroWorkspace.clearOpenRouterKey()")).Text("Remove"),
				),
			),
		),
	)
}
