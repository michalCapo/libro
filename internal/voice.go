package libro

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

const openRouterTranscriptionURL = "https://openrouter.ai/api/v1/audio/transcriptions"
const maxVoiceBytes = 44 + 16000*2*60
const voiceMissingKey = "Add an OpenRouter API key in Settings → OpenRouter to use voice typing."

type voiceStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

func voiceSnapshot() voiceStatus {
	if openRouterKey() == "" {
		return voiceStatus{State: "error", Message: voiceMissingKey}
	}
	return voiceStatus{State: "ready"}
}

func validateVoiceWAV(data []byte) error {
	if len(data) < 44+3200 || len(data) > maxVoiceBytes || (len(data)-44)%2 != 0 {
		return fmt.Errorf("record between 0.1 and 60 seconds")
	}
	if string(data[:4]) != "RIFF" || string(data[8:16]) != "WAVEfmt " || string(data[36:40]) != "data" ||
		binary.LittleEndian.Uint32(data[4:8]) != uint32(len(data)-8) || binary.LittleEndian.Uint32(data[16:20]) != 16 ||
		binary.LittleEndian.Uint16(data[20:22]) != 1 || binary.LittleEndian.Uint16(data[22:24]) != 1 ||
		binary.LittleEndian.Uint32(data[24:28]) != 16000 || binary.LittleEndian.Uint32(data[28:32]) != 32000 ||
		binary.LittleEndian.Uint16(data[32:34]) != 2 || binary.LittleEndian.Uint16(data[34:36]) != 16 ||
		binary.LittleEndian.Uint32(data[40:44]) != uint32(len(data)-44) {
		return fmt.Errorf("expected mono 16 kHz PCM audio")
	}
	return nil
}

// transcribeOpenRouter sends the recording to GPT-4o Transcribe with automatic language detection.
func transcribeOpenRouter(ctx context.Context, address, key string, data []byte) (string, error) {
	if err := validateVoiceWAV(data); err != nil {
		return "", err
	}
	payload := map[string]any{
		"model":       "openai/gpt-4o-transcribe",
		"input_audio": map[string]string{"data": base64.StdEncoding.EncodeToString(data), "format": "wav"},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "Libro")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenRouter is unreachable; check your connection and try again")
	}
	defer func() { _ = resp.Body.Close() }()
	response, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(response, &failure)
		return "", fmt.Errorf("OpenRouter transcription failed (HTTP %d): %s", resp.StatusCode, cleanVoiceText(failure.Error.Message))
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return "", fmt.Errorf("invalid OpenRouter response: %w", err)
	}
	return cleanVoiceText(result.Text), nil
}

// Dictation is one editable input, never terminal control characters or Enter.
func cleanVoiceText(text string) string {
	return strings.Join(strings.Fields(strings.Map(func(c rune) rune {
		if c < 32 || c == 127 {
			return ' '
		}
		return c
	}, text)), " ")
}

func registerVoiceRoutes(app *r.App) {
	app.GET("/voice/status", func(w http.ResponseWriter, req *http.Request) {
		if !authorizeVoice(w, req) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(voiceSnapshot())
	})
	app.POST("/voice/transcribe", func(w http.ResponseWriter, req *http.Request) {
		if !authorizeVoice(w, req) {
			return
		}
		key := openRouterKey()
		if key == "" {
			http.Error(w, voiceMissingKey, http.StatusConflict)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxVoiceBytes))
		if err != nil {
			http.Error(w, "Recording is too large", http.StatusBadRequest)
			return
		}
		if err := validateVoiceWAV(data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		text, err := transcribeOpenRouter(req.Context(), openRouterTranscriptionURL, key, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
}

func authorizeVoice(w http.ResponseWriter, req *http.Request) bool {
	if origin := req.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host != req.Host {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return false
		}
	}
	sm.mu.Lock()
	valid := sm.states[req.Header.Get("X-Libro-Session")] != nil
	sm.mu.Unlock()
	if !valid {
		http.Error(w, "invalid session", http.StatusForbidden)
	}
	return valid
}
