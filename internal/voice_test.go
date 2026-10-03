package libro

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	r "github.com/michalCapo/g-sui/ui"
)

func voiceTestWAV(samples int) []byte {
	data := make([]byte, 44+samples*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 16000)
	binary.LittleEndian.PutUint32(data[28:], 32000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(samples*2))
	return data
}

func TestVoiceAudioBounds(t *testing.T) {
	for _, samples := range []int{1600, 16000, 60 * 16000} {
		if err := validateVoiceWAV(voiceTestWAV(samples)); err != nil {
			t.Fatal(err)
		}
	}
	for _, data := range [][]byte{nil, voiceTestWAV(1599), voiceTestWAV(60*16000 + 1), append(voiceTestWAV(1600), 0)} {
		if validateVoiceWAV(data) == nil {
			t.Fatal("accepted invalid length")
		}
	}
	for _, offset := range []int{0, 4, 8, 16, 20, 22, 24, 28, 32, 34, 36, 40} {
		data := voiceTestWAV(16000)
		data[offset] ^= 1
		if validateVoiceWAV(data) == nil {
			t.Fatalf("accepted invalid header at %d", offset)
		}
	}
}

func TestVoiceRoutesRejectUnauthorizedAndInvalidAudio(t *testing.T) {
	originalSM := sm
	sm = NewStateManager()
	sm.states["voice-test"] = &AppState{}
	t.Cleanup(func() { sm = originalSM })
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	app := r.NewApp()
	registerVoiceRoutes(app)
	for _, test := range []struct {
		sid, origin, path, method string
		want                      int
	}{
		{"", "", "/voice/status", "GET", 403},
		{"unknown", "", "/voice/transcribe", "POST", 403},
		{"voice-test", "https://evil.example", "/voice/transcribe", "POST", 403},
		{"voice-test", "http://example.com", "/voice/status", "GET", 200},
		{"voice-test", "", "/voice/transcribe", "POST", 400},
	} {
		req := httptest.NewRequest(test.method, test.path, strings.NewReader("invalid WAV"))
		req.Header.Set("X-Libro-Session", test.sid)
		req.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, req)
		if w.Code != test.want {
			t.Fatalf("%+v: %d %s", test, w.Code, w.Body.String())
		}
	}
	t.Setenv("OPENROUTER_API_KEY", "")
	req := httptest.NewRequest("POST", "/voice/transcribe", bytes.NewReader(voiceTestWAV(1600)))
	req.Header.Set("X-Libro-Session", "voice-test")
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Settings → OpenRouter") {
		t.Fatalf("missing key: %d %s", w.Code, w.Body.String())
	}
}

func TestVoiceOpenRouterTranscription(t *testing.T) {
	audio := voiceTestWAV(1600)
	var got struct {
		Model      string            `json:"model"`
		InputAudio map[string]string `json:"input_audio"`
		Language   *string           `json:"language"`
	}
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", req.Header.Get("Authorization"))
		}
		got.Language = nil
		_ = json.NewDecoder(req.Body).Decode(&got)
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"text":"Vytvor pull request.\nEnter"}`))
	}))
	defer server.Close()
	text, err := transcribeOpenRouter(context.Background(), server.URL, "test-key", audio)
	if err != nil || text != "Vytvor pull request. Enter" {
		t.Fatalf("text = %q, err = %v", text, err)
	}
	if got.Model != "openai/gpt-4o-transcribe" || got.InputAudio["format"] != "wav" || got.InputAudio["data"] != base64.StdEncoding.EncodeToString(audio) || got.Language != nil {
		t.Fatalf("request = %+v", got)
	}
	status = http.StatusUnauthorized
	if _, err := transcribeOpenRouter(context.Background(), server.URL, "test-key", audio); err == nil || !strings.Contains(err.Error(), "Invalid key") {
		t.Fatalf("err = %v", err)
	}
}
