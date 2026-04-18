package libro

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	desktopbundledata "libro/internal/desktopbundledata"
	"libro/internal/version"
)

func prepareBundledDesktop() (string, string, error) {
	zipPath, err := findBundledElectronZip()
	if err != nil {
		return "", "", err
	}

	baseDir, err := bundledDesktopBaseDir(zipPath)
	if err != nil {
		return "", "", err
	}
	readyFile := filepath.Join(baseDir, ".ready")
	if _, err := os.Stat(readyFile); err != nil {
		if err := extractBundledDesktop(baseDir, zipPath); err != nil {
			return "", "", err
		}
	}

	projectRoot := filepath.Join(baseDir, "app")
	electron := bundledElectronExecutable(filepath.Join(baseDir, "runtime"))
	if electron == "" {
		return "", "", fmt.Errorf("bundled Electron executable not found")
	}
	return projectRoot, electron, nil
}

func findBundledElectronZip() (string, error) {
	entries, err := fs.Glob(desktopbundledata.Files, "electron-runtime/*.zip")
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no bundled Electron runtime for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return entries[0], nil
}

func bundledDesktopBaseDir(zipPath string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = os.TempDir()
	}
	bundleID := strings.TrimSuffix(filepath.Base(zipPath), ".zip")
	return filepath.Join(cacheDir, "libro", "desktop", version.Version, bundleID), nil
}

func extractBundledDesktop(baseDir, zipPath string) error {
	tmpDir := fmt.Sprintf("%s.tmp-%d", baseDir, time.Now().UnixNano())
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractBundledApp(tmpDir); err != nil {
		return err
	}
	if err := extractBundledRuntimeZip(tmpDir, zipPath); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".ready"), []byte(version.Version+"\n"), 0o644); err != nil {
		return err
	}

	_ = os.RemoveAll(baseDir)
	if err := os.Rename(tmpDir, baseDir); err != nil {
		return err
	}
	return nil
}

func extractBundledApp(baseDir string) error {
	appDir := filepath.Join(baseDir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}

	return fs.WalkDir(desktopbundledata.Files, "electron-app", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "electron-app")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == "README.md" {
			return nil
		}

		target := filepath.Join(appDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyEmbeddedFile(desktopbundledata.Files, path, target)
	})
}

func extractBundledRuntimeZip(baseDir, zipPath string) error {
	data, err := fs.ReadFile(desktopbundledata.Files, zipPath)
	if err != nil {
		return err
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return err
	}

	for _, file := range reader.File {
		if err := extractZipEntry(runtimeDir, file); err != nil {
			return fmt.Errorf("extract %s: %w", file.Name, err)
		}
	}

	return nil
}

func extractZipEntry(destRoot string, file *zip.File) error {
	cleanName := filepath.Clean(file.Name)
	if cleanName == "." {
		return nil
	}
	if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
		return fmt.Errorf("unsafe path %q", file.Name)
	}

	target := filepath.Join(destRoot, filepath.FromSlash(cleanName))
	if file.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	mode := file.Mode()
	if mode&os.ModeSymlink != 0 {
		if runtime.GOOS == "windows" {
			return nil
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		linkTarget, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		_ = os.Remove(target)
		return os.Symlink(string(linkTarget), target)
	}

	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		perm := mode.Perm()
		if perm == 0 {
			perm = 0o755
		}
		if err := os.Chmod(target, perm); err != nil {
			return err
		}
	}

	return nil
}

func copyEmbeddedFile(fsys fs.FS, sourcePath, targetPath string) error {
	data, err := fs.ReadFile(fsys, sourcePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, data, 0o644)
}

func bundledElectronExecutable(runtimeDir string) string {
	var candidate string
	switch runtime.GOOS {
	case "windows":
		candidate = filepath.Join(runtimeDir, "electron.exe")
	case "darwin":
		candidate = filepath.Join(runtimeDir, "Electron.app", "Contents", "MacOS", "Electron")
	default:
		candidate = filepath.Join(runtimeDir, "electron")
	}

	if _, err := os.Stat(candidate); err != nil {
		return ""
	}
	return candidate
}
