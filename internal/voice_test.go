package libro

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	text, err := transcribeVoice(context.Background(), dir, sample)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(text), "yellow lamps") {
		t.Fatalf("unexpected transcription: %q", text)
	}
}
