package libro

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	r "github.com/michalCapo/g-sui/ui"
)

type voiceOption struct{ Code, Name string }

// GPT-4o Transcribe is most accurate when told the language.
var voiceLanguages = []voiceOption{
	{"", "Auto-detect"},
	{"sk", "Slovak"},
	{"en", "English"},
}

func validVoiceOption(options []voiceOption, value string) bool {
	return slices.ContainsFunc(options, func(item voiceOption) bool { return item.Code == value })
}

func voiceSetting(key string, options []voiceOption) string {
	dbMu.Lock()
	defer dbMu.Unlock()
	value := options[0].Code
	if db != nil {
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	}
	if !validVoiceOption(options, value) {
		return options[0].Code
	}
	return value
}

func voiceLanguage() string { return voiceSetting("voice_language", voiceLanguages) }

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
	return map[string]any{"language": voiceLanguage(), "savedKey": savedOpenRouterKey()}
}

// An empty key keeps the saved one; clearKey removes it.
func setVoiceSettings(language, key string, clearKey bool) error {
	key = strings.TrimSpace(key)
	if !validVoiceOption(voiceLanguages, language) {
		return fmt.Errorf("choose a supported dictation language")
	}
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
	if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('voice_language',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, language); err != nil {
		return err
	}
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

func renderVoiceSelect(id, label, help string, options []voiceOption) *r.Node {
	nodes := make([]*r.Node, 0, len(options))
	for _, option := range options {
		nodes = append(nodes, r.El("option", "").Attr("value", option.Code).Text(option.Name))
	}
	return r.Div("ws-settings-row").Render(
		r.Div("ws-settings-copy").Render(
			r.El("label", "").Attr("for", id).Text(label),
			r.P("").ID(id+"-help").Text(help),
		),
		r.El("select", "ws-settings-select").ID(id).Attr("aria-describedby", id+"-help").Render(nodes...),
	)
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
		r.El("h2", "ws-shortcut-heading").Text("Voice typing"),
		r.Div("ws-settings-group").Render(
			renderVoiceSelect("voice-language", "Dictation language", "Auto-detect handles mixed Slovak and English; choose a language for best accuracy.", voiceLanguages),
		),
	)
}
