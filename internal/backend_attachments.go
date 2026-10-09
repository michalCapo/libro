package libro

import (
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// Exact page pixels still come from Electron. The backend owns their storage.
func registerBackendAttachments(app interface {
	POST(string, http.HandlerFunc)
}) {
	app.POST("/attachments", saveBackendAttachments)
}

func saveBackendAttachments(w http.ResponseWriter, req *http.Request) {
	origin, err := url.Parse(req.Header.Get("Origin"))
	if err != nil || origin.Host != req.Host || origin.Scheme != "http" && origin.Scheme != "https" {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}
	backendActionMu.Lock()
	panel, _, _, found := sm.workspaceApp(inputSID(req.URL.Query().Get("sid")), req.URL.Query().Get("id"))
	backendActionMu.Unlock()
	if !found || panel.Type != AppTypeURL {
		http.Error(w, "select a browser panel", http.StatusForbidden)
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, 128<<20)
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid or oversized image attachments", http.StatusBadRequest)
		return
	}
	defer func() { _ = req.MultipartForm.RemoveAll() }()
	files := req.MultipartForm.File["images"]
	if len(files) == 0 || len(files) > 8 {
		http.Error(w, "attach between 1 and 8 images", http.StatusBadRequest)
		return
	}
	dir, err := libroDataDir()
	if err == nil {
		dir = filepath.Join(dir, "attachments")
		err = os.MkdirAll(dir, 0o700)
	}
	var batch string
	if err == nil {
		batch, err = os.MkdirTemp(dir, "page-")
	}
	if err != nil {
		http.Error(w, "could not create attachment storage", http.StatusInternalServerError)
		return
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(batch)
		}
	}()
	paths := make([]string, 0, len(files))
	for index, file := range files {
		path := filepath.Join(batch, fmt.Sprintf("image-%d.png", index+1))
		err := func() error {
			src, err := file.Open()
			if err != nil {
				return err
			}
			defer func() { _ = src.Close() }()
			config, err := png.DecodeConfig(src)
			if err != nil || config.Width > 20000 || config.Height > 20000 {
				return fmt.Errorf("invalid PNG attachment")
			}
			if _, err := src.Seek(0, io.SeekStart); err != nil {
				return err
			}
			dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(dst, src)
			if closeErr := dst.Close(); err == nil {
				err = closeErr
			}
			return err
		}()
		if err != nil {
			http.Error(w, "could not save image attachment", http.StatusBadRequest)
			return
		}
		paths = append(paths, path)
	}
	complete = true
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"paths": paths})
}
