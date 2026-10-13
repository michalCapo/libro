package libro

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"libro/internal/components"
	desktopbundledata "libro/internal/desktopbundledata"
)

// installBundledTools unpacks the embedded ide tools (lazyrepo, lazydata,
// nvim, ...) and appends them to PATH. Appending keeps tools the user installed
// themselves first; bundled launchers still find their sibling nvim.
func installBundledTools() {
	dir, err := prepareBundledTools()
	if err != nil {
		log.Printf("bundled tools: %v", err)
		return
	}
	if dir != "" {
		_ = os.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+dir)
		components.BundledToolsDir = dir
	}
}

func prepareBundledTools() (string, error) {
	entries, err := fs.Glob(desktopbundledata.Files, "cli-tools/*.tar.gz")
	if err != nil || len(entries) == 0 {
		return "", err
	}
	data, err := fs.ReadFile(desktopbundledata.Files, entries[0])
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = os.TempDir()
	}
	parent := filepath.Join(cacheDir, "libro", "tools")
	dir := filepath.Join(parent, hex.EncodeToString(sum[:8]))
	if _, err := os.Stat(filepath.Join(dir, ".ready")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if err := extractToolsArchive(tmpDir, data); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".ready"), nil, 0o644); err != nil {
		return "", err
	}
	// Older tool versions are unused once this one is ready. Another Libro
	// starting at the same time may still be unpacking into its own .tmp- folder
	// or already use this version.
	if old, err := os.ReadDir(parent); err == nil {
		for _, entry := range old {
			if name := entry.Name(); !strings.HasPrefix(name, ".tmp-") && name != filepath.Base(dir) {
				_ = os.RemoveAll(filepath.Join(parent, entry.Name()))
			}
		}
	}
	if err := os.Rename(tmpDir, dir); err != nil {
		if _, statErr := os.Stat(filepath.Join(dir, ".ready")); statErr == nil {
			return dir, nil
		}
		return "", err
	}
	return dir, nil
}

// The ide release archive holds only top-level executables.
func extractToolsArchive(dir string, data []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if header.Typeflag != tar.TypeReg || !filepath.IsLocal(name) || strings.ContainsRune(name, filepath.Separator) {
			return fmt.Errorf("unexpected archive entry %q", header.Name)
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, archive); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}
