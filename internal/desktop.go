package libro

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// OpenDesktop opens the libro UI in an Electron window.
// When the Electron window is closed, it sends on the returned channel
// so the process can shut down.
func OpenDesktop(url string) <-chan struct{} {
	done := make(chan struct{})
	projectRoot := findProjectRoot()
	electron := ""
	if hasElectronProject(projectRoot) {
		electron = findElectron()
		if electron == "" && installElectron(projectRoot) {
			electron = findElectron()
		}
	}

	if electron == "" {
		bundledProjectRoot, bundledElectron, err := prepareBundledDesktop()
		if err == nil {
			projectRoot = bundledProjectRoot
			electron = bundledElectron
			log.Println("[desktop] Using bundled Electron runtime.")
		} else {
			log.Printf("[desktop] Bundled Electron unavailable: %v", err)
		}
	}

	if electron == "" || projectRoot == "" {
		if err := openBrowserWhenReady(url); err != nil {
			log.Printf("[desktop] Electron unavailable and browser fallback failed: %v", err)
		} else {
			log.Println("[desktop] Electron unavailable; opened Libro in the default browser.")
		}
		return done
	}

	// Unbind GNOME shortcuts Libro uses so they reach the app.
	unbindGnomeGlobalShortcuts()

	cmd := exec.Command(electron, projectRoot, "--gtk-version=3")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "LIBRO_PORT="+Port())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("[desktop] Electron failed: %v", err)
		return done
	}

	go func() {
		cmd.Wait()
		close(done)
	}()

	return done
}

func hasElectronProject(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "package.json")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "electron", "main.js")); err != nil {
		return false
	}
	return true
}

// installElectron runs npm install in the project root to install Electron.
func installElectron(projectRoot string) bool {
	npmBin, err := exec.LookPath("npm")
	if err != nil {
		log.Println("[desktop] npm not found, cannot install Electron")
		return false
	}

	// Only attempt if package.json exists
	if _, err := os.Stat(filepath.Join(projectRoot, "package.json")); err != nil {
		log.Println("[desktop] package.json not found, cannot install Electron")
		return false
	}

	log.Println("[desktop] Electron not found, installing...")
	cmd := exec.Command(npmBin, "install", "--no-fund", "--no-audit", "--omit=dev")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("[desktop] npm install failed: %v", err)
		return false
	}
	log.Println("[desktop] Electron installed")
	return true
}

func openBrowserWhenReady(url string) error {
	if err := waitForHTTPServer("127.0.0.1:"+Port(), 5*time.Second); err != nil {
		return err
	}
	return openBrowser(url)
}

func waitForHTTPServer(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server on %s did not become ready within %s", address, timeout)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// findElectron returns the path to a locally installed Electron binary.
// Never falls back to npx to avoid downloading ~200MB on every launch.
func findElectron() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".cmd"
	}

	// Check node_modules/.bin/electron relative to the executable
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), "node_modules", ".bin", "electron"+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check node_modules/.bin/electron relative to working directory
	if wd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(wd, "node_modules", ".bin", "electron"+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check in PATH (system-installed electron)
	if p, err := exec.LookPath("electron"); err == nil {
		return p
	}

	return ""
}

// findProjectRoot returns the directory containing package.json (project root).
func findProjectRoot() string {
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		if hasElectronProject(exeDir) {
			return exeDir
		}
	}
	// Fallback to working directory
	if wd, err := os.Getwd(); err == nil {
		if hasElectronProject(wd) {
			return wd
		}
	}
	return ""
}

// unbindGnomeGlobalShortcuts disables GNOME shortcuts that Libro remaps.
func unbindGnomeGlobalShortcuts() {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath("gsettings"); err != nil {
		return
	}
	// Disabled: Libro should not mutate the user's GNOME global shortcuts.
	// exec.Command("gsettings", "set", "org.gnome.desktop.wm.keybindings", "show-desktop", "['']").Run()
	// exec.Command("gsettings", "set", "org.gnome.desktop.wm.keybindings", "close", "['']").Run()
}
