package libro

import (
	"fmt"
	"slices"

	r "github.com/michalCapo/g-sui/ui"
)

const voiceSlovakEnglish = "sk-en"

// Keep all-language detection available alongside the Slovak/English modes.
var voiceLanguages = []struct{ Code, Name string }{
	{voiceSlovakEnglish, "Slovak + English (automatic)"},
	{"", "Auto-detect all languages"},
	{"sk", "Slovak"},
	{"en", "English"},
}

func validVoiceLanguage(language string) bool {
	return slices.ContainsFunc(voiceLanguages, func(item struct{ Code, Name string }) bool { return item.Code == language })
}

func voiceLanguage() string {
	dbMu.Lock()
	defer dbMu.Unlock()
	language := voiceSlovakEnglish
	if db != nil {
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'voice_language'`).Scan(&language)
	}
	if !validVoiceLanguage(language) {
		return voiceSlovakEnglish
	}
	return language
}

func setVoiceLanguage(language string) error {
	if !validVoiceLanguage(language) {
		return fmt.Errorf("choose a supported dictation language")
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err := db.Exec(`INSERT INTO settings (key,value) VALUES ('voice_language',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, language)
	return err
}

func renderVoiceSettings() *r.Node {
	options := make([]*r.Node, 0, len(voiceLanguages))
	for _, language := range voiceLanguages {
		options = append(options, r.El("option", "").Attr("value", language.Code).Text(language.Name))
	}
	return r.El("section", "").Render(
		r.El("h2", "ws-shortcut-heading").Text("Voice typing"),
		r.Div("ws-settings-group").Render(
			r.Div("ws-settings-row").Render(
				r.Div("ws-settings-copy").Render(
					r.El("label", "").Attr("for", "voice-language").Text("Dictation language"),
					r.P("").ID("voice-language-help").Text("Use Slovak and English without switching. Choose another option for other languages. Applies across projects."),
				),
				r.El("select", "ws-settings-select").ID("voice-language").Attr("aria-describedby", "voice-language-help").Render(options...),
			),
		),
	)
}
