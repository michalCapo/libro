package components

import _ "embed"

//go:embed voice_input.js
var voiceInputScript string

// VoiceInputJS captures and inserts text into focused workspace or guest inputs.
func VoiceInputJS() string { return voiceInputScript }
