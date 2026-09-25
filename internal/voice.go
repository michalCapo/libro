package libro

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

const voiceVersion = "v1.13.8"
const voiceModel = "sherpa-onnx-whisper-tiny"
const voiceModelHash = "c46116994e539aa165266d96b325252728429c12535eb9d8b6a2b10f129e66b1"
const maxVoiceBytes = 44 + 16000*2*60

type voiceAsset struct{ Name, Hash string }

// Pin both the engine and its checksums; never execute an unchecked download.
var voiceRuntimes = map[string]voiceAsset{
	"linux/amd64":   {"linux-x64-shared-no-tts", "d0f96c8b65c6cd0974fada22737e337de81bc8cd2abbec2e39caf358b1eec5fc"},
	"linux/arm64":   {"linux-aarch64-shared-cpu", "4e3734f82bc1379fd91f219f5869c7e9d03b7a4f7561907d8abca4849c51a789"},
	"darwin/amd64":  {"osx-x64-shared-no-tts", "03fd4cffd98b239d74b9253c270ff637661adb4d51a5a6c9e1f7486e48306db3"},
	"darwin/arm64":  {"osx-arm64-shared-no-tts", "91b96512c4fa1960f8a9ed5360a6c8dda53a4b5015d0590244f14086a234557a"},
	"windows/amd64": {"win-x64-shared-MT-Release-no-tts", "4b0a94f7b5c606b1b64a19a831c2127559e4b3d34e195465ebc7be73d9ed4783"},
	"windows/arm64": {"win-arm64-shared-MT-Release-no-tts", "29a864324e658bef2a8b83bd3e12adae8b415a5a232d83902030e8c6efa37dbc"},
}

type voiceStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type voiceService struct {
	mu     sync.Mutex
	status voiceStatus
	busy   bool
}

var voice = &voiceService{status: voiceStatus{State: "idle", Message: "Preparing voice typing…"}}

func voiceDir() (string, error) {
	base, err := libroBaseDataDir()
	return filepath.Join(base, "voice", voiceVersion+"-"+runtime.GOOS+"-"+runtime.GOARCH), err
}

func voiceExecutable(dir string) string {
	name := "sherpa-onnx-offline"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "runtime", "bin", name)
}

func voiceInstalled(dir string) bool {
	for _, file := range []string{".ready", filepath.Join("runtime", "bin", filepath.Base(voiceExecutable(dir))), filepath.Join("model", "tiny-encoder.int8.onnx"), filepath.Join("model", "tiny-decoder.int8.onnx"), filepath.Join("model", "tiny-tokens.txt")} {
		if info, err := os.Stat(filepath.Join(dir, file)); err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

func (v *voiceService) snapshot() voiceStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.status
}

func (v *voiceService) report(state, message string) {
	v.mu.Lock()
	v.status = voiceStatus{State: state, Message: message}
	v.mu.Unlock()
}

func (v *voiceService) prepare() {
	v.mu.Lock()
	if v.status.State == "installing" || v.status.State == "ready" {
		v.mu.Unlock()
		return
	}
	v.status = voiceStatus{State: "installing", Message: "Downloading voice typing…"}
	v.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		if err := installVoice(ctx, func(message string) { v.report("installing", message) }); err != nil {
			v.report("error", "Voice setup failed. Click the microphone to retry. "+err.Error())
			return
		}
		v.report("ready", "Press to start voice typing")
	}()
}

// InstallVoice prepares the same local assets used by first-launch setup.
func InstallVoice(ctx context.Context) error {
	return installVoice(ctx, func(message string) { fmt.Println(message) })
}

func installVoice(ctx context.Context, progress func(string)) error {
	dir, err := voiceDir()
	if err != nil {
		return err
	}
	if voiceInstalled(dir) {
		return nil
	}
	asset, ok := voiceRuntimes[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return fmt.Errorf("voice typing is unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".voice-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	engine := "sherpa-onnx-" + voiceVersion + "-" + asset.Name
	for _, item := range []struct{ label, tag, name, hash, target string }{
		{"speech engine", voiceVersion, engine, asset.Hash, "runtime"},
		{"speech model", "asr-models", voiceModel, voiceModelHash, "model"},
	} {
		progress("Downloading " + item.label + "…")
		archive := filepath.Join(tmp, item.target+".tar.bz2")
		address := "https://github.com/k2-fsa/sherpa-onnx/releases/download/" + item.tag + "/" + item.name + ".tar.bz2"
		if err := downloadVoice(ctx, address, archive, item.hash); err != nil {
			return err
		}
		progress("Installing " + item.label + "…")
		if err := extractVoice(archive, filepath.Join(tmp, item.target)); err != nil {
			return err
		}
		_ = os.Remove(archive)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".ready"), []byte(voiceVersion), 0o600); err != nil {
		return err
	}
	if !voiceInstalled(tmp) {
		return fmt.Errorf("voice download is missing required files")
	}
	// Concurrent Libro instances may finish setup first. Publish only complete installs.
	if voiceInstalled(dir) {
		return nil
	}
	// Repair an incomplete installation without publishing partially extracted files.
	if _, err := os.Stat(dir); err == nil {
		previous := tmp + ".previous"
		if err := os.Rename(dir, previous); err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(previous) }()
	}
	if err := os.Rename(tmp, dir); err != nil {
		if voiceInstalled(dir) {
			return nil
		}
		return err
	}
	return nil
}

func downloadVoice(ctx context.Context, address, dest, checksum string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download voice assets: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("voice download returned HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	const limit = 200 << 20
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, limit+1))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > limit || fmt.Sprintf("%x", hash.Sum(nil)) != checksum {
		return fmt.Errorf("voice download checksum mismatch; retry setup")
	}
	return nil
}

func extractVoice(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return extractVoiceFiles(tar.NewReader(bzip2.NewReader(f)), dir)
}

func extractVoiceFiles(reader *tar.Reader, dir string) error {
	var total int64
	for {
		h, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// Only regular files are needed. Reject links and paths outside the archive root.
		_, rel, found := strings.Cut(h.Name, "/")
		if !found || rel == "" || h.Typeflag == tar.TypeDir {
			continue
		}
		if !filepath.IsLocal(rel) || strings.Contains(rel, "\\") {
			return fmt.Errorf("unsafe voice archive path")
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsupported voice archive entry %s", h.Name)
		}
		total += h.Size
		if h.Size < 0 || total > 1<<30 {
			return fmt.Errorf("voice archive too large")
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, reader)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
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

func transcribeVoice(ctx context.Context, dir string, data []byte) (string, error) {
	if err := validateVoiceWAV(data); err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "libro-voice-*.wav")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	model := filepath.Join(dir, "model")
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, voiceExecutable(dir), "--whisper-encoder="+filepath.Join(model, "tiny-encoder.int8.onnx"), "--whisper-decoder="+filepath.Join(model, "tiny-decoder.int8.onnx"), "--tokens="+filepath.Join(model, "tiny-tokens.txt"), "--whisper-task=transcribe", "--num-threads=2", f.Name())
	cmd.Dir = filepath.Dir(voiceExecutable(dir))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("transcription failed; please try again: %w", err)
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &result); err != nil {
		return "", fmt.Errorf("invalid speech engine response: %w", err)
	}
	// Dictation is one editable input, never terminal control characters or Enter.
	return strings.Join(strings.Fields(strings.Map(func(c rune) rune {
		if c < 32 || c == 127 {
			return ' '
		}
		return c
	}, result.Text)), " "), nil
}

func registerVoiceRoutes(app *r.App) {
	app.GET("/voice/status", func(w http.ResponseWriter, req *http.Request) {
		if !authorizeVoice(w, req) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(voice.snapshot())
	})
	app.POST("/voice/setup", func(w http.ResponseWriter, req *http.Request) {
		if !authorizeVoice(w, req) {
			return
		}
		voice.prepare()
		w.WriteHeader(http.StatusAccepted)
	})
	app.POST("/voice/transcribe", func(w http.ResponseWriter, req *http.Request) {
		if !authorizeVoice(w, req) {
			return
		}
		voice.mu.Lock()
		if voice.status.State != "ready" || voice.busy {
			voice.mu.Unlock()
			http.Error(w, "Voice typing is not ready. Please try again shortly.", http.StatusConflict)
			return
		}
		voice.busy = true
		voice.mu.Unlock()
		defer func() { voice.mu.Lock(); voice.busy = false; voice.mu.Unlock() }()
		data, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxVoiceBytes))
		if err != nil {
			http.Error(w, "Recording is too large", http.StatusBadRequest)
			return
		}
		if err := validateVoiceWAV(data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dir, err := voiceDir()
		if err != nil {
			http.Error(w, "Cannot access voice files", http.StatusInternalServerError)
			return
		}
		text, err := transcribeVoice(req.Context(), dir, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
