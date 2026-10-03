package libro

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	r "github.com/michalCapo/g-sui/ui"
)

func TestVoiceLanguagePersistence(t *testing.T) {
	original := db
	var err error
	path := filepath.Join(t.TempDir(), "settings.db")
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	if got := voiceLanguage(); got != voiceSlovakEnglish {
		t.Fatalf("default language = %q", got)
	}
	for _, language := range []string{"sk", "en", "", voiceSlovakEnglish} {
		if err := setVoiceLanguage(language); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		db, err = sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if got := voiceLanguage(); got != language {
			t.Fatalf("saved %q, loaded %q", language, got)
		}
	}
	for _, language := range []string{"unknown", "sk-SK", "sk,en", "--whisper-task=translate"} {
		if err := setVoiceLanguage(language); err == nil {
			t.Fatalf("accepted language %q", language)
		}
		if got := voiceLanguage(); got != voiceSlovakEnglish {
			t.Fatalf("invalid save changed language to %q", got)
		}
	}
}

func TestVoiceSlovakEnglishTranscription(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("speech engine stub requires a POSIX shell")
	}
	for _, test := range []struct {
		name, language, detected, text, fallback, want, calls string
		failRetry                                             bool
	}{
		{"russian guess", voiceSlovakEnglish, "ru", "Это функция.", "sk", "Je to funkcia.", "\nsk\n", false},
		{"czech guess", voiceSlovakEnglish, "cs", "Je to funkce.", "sk", "Je to funkcia.", "\nsk\n", false},
		{"english", voiceSlovakEnglish, "en", "This is a feature.", "sk", "This is a feature.", "\n", false},
		{"slovak", voiceSlovakEnglish, "sk", "  Je to\n funkcia.\t", "sk", "Je to funkcia.", "\n", false},
		{"all languages", "", "ru", "Это функция.", "sk", "Это функция.", "\n", false},
		{"fixed slovak", "sk", "ru", "Это функция.", "sk", "Je to funkcia.", "sk\n", false},
		{"fixed english", "en", "ru", "Это функция.", "sk", "This is a feature.", "en\n", false},
		{"failed retry", voiceSlovakEnglish, "ru", "Это функция.", "sk", "", "\nsk\n", true},
		{"wrong retry language", voiceSlovakEnglish, "ru", "Это функция.", "ru", "", "\nsk\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Dir(voiceExecutable(dir))
			if err := os.MkdirAll(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			script := `#!/bin/sh
for arg do
  case "$arg" in
    --whisper-language=*) language=${arg#*=} ;;
    --whisper-task=transcribe) task=transcribe ;;
  esac
done
[ "$task" = transcribe ] && [ -f "$arg" ] || exit 1
printf '%s\n' "$language" >> calls
printf '%s' "$arg" > recording
cat "response-$language.json"
`
			if err := os.WriteFile(voiceExecutable(dir), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			for language, response := range map[string]voiceResult{
				"":   {Text: test.text, Language: test.detected},
				"sk": {Text: "Je to funkcia.", Language: test.fallback},
				"en": {Text: "This is a feature.", Language: "en"},
			} {
				if language == "sk" && test.failRetry {
					continue
				}
				data, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(bin, "response-"+language+".json"), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			text, err := transcribeVoice(context.Background(), dir, voiceTestWAV(1600), test.language)
			if (err != nil) != (test.want == "") || text != test.want {
				t.Fatalf("text = %q, err = %v; want %q", text, err, test.want)
			}
			calls, err := os.ReadFile(filepath.Join(bin, "calls"))
			if err != nil || string(calls) != test.calls {
				t.Fatalf("engine calls = %q, err = %v; want %q", calls, err, test.calls)
			}
			recording, err := os.ReadFile(filepath.Join(bin, "recording"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(string(recording)); !os.IsNotExist(err) {
				t.Fatalf("temporary recording remains: %v", err)
			}
		})
	}
}

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

func TestVoiceDownloadIntegrity(t *testing.T) {
	payload := "a pinned engine"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/missing" {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	for _, test := range []struct {
		path, hash string
		valid      bool
	}{{"/ok", hash, true}, {"/ok", "bad", false}, {"/missing", hash, false}} {
		err := downloadVoice(context.Background(), server.URL+test.path, filepath.Join(t.TempDir(), "download"), test.hash)
		if (err == nil) != test.valid {
			t.Fatalf("%+v: %v", test, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if downloadVoice(ctx, server.URL, filepath.Join(t.TempDir(), "download"), hash) == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestVoiceArchivePaths(t *testing.T) {
	for _, test := range []struct {
		name  string
		kind  byte
		valid bool
	}{
		{"engine/bin/sherpa-onnx-offline", tar.TypeReg, true},
		{"engine/../escape", tar.TypeReg, false},
		{"engine//absolute", tar.TypeReg, false},
		{"engine/bin\\escape", tar.TypeReg, false},
		{"engine/bin/link", tar.TypeSymlink, false},
		{"engine/bin/link", tar.TypeLink, false},
	} {
		var archive bytes.Buffer
		writer := tar.NewWriter(&archive)
		header := &tar.Header{Name: test.name, Typeflag: test.kind, Mode: 0o755}
		if test.kind == tar.TypeReg {
			header.Size = 5
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if test.kind == tar.TypeReg {
			_, _ = writer.Write([]byte("hello"))
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		err := extractVoiceFiles(tar.NewReader(&archive), t.TempDir())
		if (err == nil) != test.valid {
			t.Fatalf("%s: %v", test.name, err)
		}
	}
}

func TestVoiceRoutesRejectUnauthorizedAndInvalidAudio(t *testing.T) {
	originalSM, originalVoice := sm, voice
	sm = NewStateManager()
	sm.states["voice-test"] = &AppState{}
	voice = &voiceService{status: voiceStatus{State: "ready"}}
	t.Cleanup(func() { sm, voice = originalSM, originalVoice })
	app := r.NewApp()
	registerVoiceRoutes(app)
	for _, test := range []struct {
		sid, origin, path, method string
		want                      int
	}{
		{"", "", "/voice/status", "GET", 403},
		{"unknown", "", "/voice/setup", "POST", 403},
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
	if voice.busy {
		t.Fatal("invalid audio left transcription locked")
	}
}

// Opt-in smoke test using downloaded public engine/model archives. No microphone needed.
func TestVoiceRealTranscription(t *testing.T) {
	dir := os.Getenv("LIBRO_VOICE_TEST_ASSETS")
	if dir == "" {
		t.Skip("set LIBRO_VOICE_TEST_ASSETS to the extracted runtime/model directory")
	}
	sample, err := os.ReadFile(filepath.Join(dir, "model", "test_wavs", "0.wav"))
	if err != nil {
		t.Fatal(err)
	}
	text, err := transcribeVoice(context.Background(), dir, sample, voiceSlovakEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(text), "yellow lamps") {
		t.Fatalf("unexpected transcription: %q", text)
	}
}
