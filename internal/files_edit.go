package libro

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Files edits run under backendActionMu, so a version check and its write
// cannot interleave with another Libro action.

const fileSaveLimit = 1024 * 1024

// editablePath rejects the root and any "..", which could name the root
// (a/..) or reach a different folder through a link.
func editablePath(path string) error {
	if path == "" || !filepath.IsLocal(path) || slices.Contains(strings.Split(filepath.ToSlash(path), "/"), "..") || filepath.Clean(path) == "." {
		return fmt.Errorf("path must stay inside the files root")
	}
	return nil
}

// saveProjectFile writes text when the file still has the version the editor
// loaded. An empty version overwrites (:w!).
func saveProjectFile(rootPath, path, text, version string) (fileResult, error) {
	if err := editablePath(path); err != nil {
		return fileResult{Path: path}, err
	}
	if len(text) > fileSaveLimit {
		return fileResult{Path: path}, fmt.Errorf("file is too large to save (limit %d MB)", fileSaveLimit/(1024*1024))
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return fileResult{Path: path}, err
	}
	defer func() { _ = root.Close() }()
	perm := fs.FileMode(0o644)
	info, err := root.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if version != "" {
			return fileResult{Path: path}, fmt.Errorf("file was deleted on disk; use :w! to write it again")
		}
	case err != nil:
		return fileResult{Path: path}, err
	case !info.Mode().IsRegular():
		return fileResult{Path: path}, fmt.Errorf("only regular files can be saved")
	case version != "" && fileVersion(info) != version:
		return fileResult{Path: path}, fmt.Errorf("file changed on disk; use :w! to overwrite or :e! to reload")
	default:
		perm = info.Mode().Perm()
	}
	target := path
	if link, linkErr := root.Lstat(path); linkErr == nil && link.Mode()&fs.ModeSymlink != 0 {
		// Replace the file the link points to, so the link stays a link.
		if target, err = linkTarget(rootPath, path); err != nil {
			return fileResult{Path: path}, err
		}
	}
	// Replace the file only after the new content is complete.
	temp := filepath.Join(filepath.Dir(target), fmt.Sprintf(".%s.%d.libro-save", filepath.Base(target), time.Now().UnixNano()))
	err = writeRootFile(root, temp, perm, text)
	if err == nil {
		err = root.Rename(temp, target)
	}
	if err != nil {
		_ = root.Remove(temp)
		return fileResult{Path: path}, err
	}
	return readProjectFile(rootPath, path)
}

// linkTarget returns the file a link inside the root points to, relative to the root.
func linkTarget(rootPath, path string) (string, error) {
	file, err := projectFileToOpen(rootPath, path)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(rootPath)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return "", err
	}
	return filepath.Rel(root, file)
}

func writeRootFile(root *os.Root, path string, perm fs.FileMode, text string) error {
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = f.WriteString(text)
	if err == nil {
		// The umask can narrow the mode of a new file.
		err = f.Chmod(perm)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// createProjectEntry creates a file or folder and any missing parent folders.
func createProjectEntry(rootPath, path string, directory bool) error {
	if err := editablePath(path); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if parent := filepath.Dir(path); parent != "." {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if directory {
		return root.Mkdir(path, 0o755)
	}
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func renameProjectEntry(rootPath, path, target string) error {
	if err := editablePath(path); err != nil {
		return err
	}
	if err := editablePath(target); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	from, err := root.Lstat(path)
	if err != nil {
		return err
	}
	// Rename replaces files on some systems. A case-only rename on a
	// case-insensitive disk finds the same file, which is fine.
	if to, err := root.Lstat(target); err == nil && !os.SameFile(from, to) {
		return fmt.Errorf("%s already exists", filepath.ToSlash(target))
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if parent := filepath.Dir(target); parent != "." {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	return root.Rename(path, target)
}

// deleteProjectEntry removes a file, a link or a folder with its content.
func deleteProjectEntry(rootPath, path string) error {
	if err := editablePath(path); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if _, err := root.Lstat(path); err != nil {
		return err
	}
	return root.RemoveAll(path)
}
