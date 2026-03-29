package libro

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenDesktop opens the libro UI in an Electron window.
// When the Electron window is closed, it sends on the returned channel
// so the process can shut down.
func OpenDesktop(url string) <-chan struct{} {
	done := make(chan struct{})

	electron := findElectron()
	if electron == "" {
		projectRoot := findProjectRoot()
		if installElectron(projectRoot) {
			electron = findElectron()
		}
		if electron == "" {
			log.Println("[desktop] Electron not found. Run 'npm install' in the libro directory.")
			return done
		}
	}

	// Unbind GNOME's Super+D (show-desktop) so it reaches the app
	unbindGnomeSuperD()

	// Find project root (where package.json / electron/main.js live)
	projectRoot := findProjectRoot()

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
		if _, err := os.Stat(filepath.Join(exeDir, "package.json")); err == nil {
			return exeDir
		}
	}
	// Fallback to working directory
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// unbindGnomeSuperD disables the GNOME show-desktop shortcut (Super+D)
// so it can be used by the app. The binding is restored when not needed.
func unbindGnomeSuperD() {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath("gsettings"); err != nil {
		return
	}
	exec.Command("gsettings", "set", "org.gnome.desktop.wm.keybindings", "show-desktop", "['']").Run()
}
