package libro

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBackendAttachmentStorage(t *testing.T) {
	setupNoteControl(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LIBRO_INSTANCE", "")
	sm.Get("test").Apps = []Application{{ID: "browser", Type: AppTypeURL}}
	png, err := os.ReadFile("../winres/icon16.png")
	if err != nil {
		t.Fatal(err)
	}
	upload := func(id, origin string, images ...[]byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for _, image := range images {
			file, err := form.CreateFormFile("images", "../../same-name.png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write(image); err != nil {
				t.Fatal(err)
			}
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://libro.test/attachments?sid=test&id="+id, &body)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", form.FormDataContentType())
		response := httptest.NewRecorder()
		saveBackendAttachments(response, req)
		return response
	}
	response := upload("browser", "http://libro.test", png, png)
	if response.Code != http.StatusOK {
		t.Fatalf("upload status: %d", response.Code)
	}
	var result struct{ Paths []string }
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 2 || result.Paths[0] == result.Paths[1] {
		t.Fatal("duplicate attachment names overwrote each other")
	}
	for _, path := range result.Paths {
		content, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(content, png) {
			t.Fatal("attachment pixels changed")
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("attachment was not private")
		}
		info, err = os.Stat(filepath.Dir(path))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatal("attachment directory was not private")
		}
	}
	for _, test := range []struct {
		id, origin string
		images     [][]byte
		status     int
	}{
		{"missing", "http://libro.test", [][]byte{png}, http.StatusForbidden},
		{"browser", "http://foreign.test", [][]byte{png}, http.StatusForbidden},
		{"browser", "", [][]byte{png}, http.StatusForbidden},
		{"browser", "http://libro.test", nil, http.StatusBadRequest},
		{"browser", "http://libro.test", [][]byte{png, []byte("invalid PNG")}, http.StatusBadRequest},
	} {
		if response := upload(test.id, test.origin, test.images...); response.Code != test.status {
			t.Fatalf("invalid upload status: %d", response.Code)
		}
	}
	dir, _ := libroDataDir()
	batches, err := os.ReadDir(filepath.Join(dir, "attachments"))
	if err != nil || len(batches) != 1 {
		t.Fatal("failed upload left partial files")
	}
}
