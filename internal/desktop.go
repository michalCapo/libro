package libro

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenDesktop opens the libro UI in a Chromium --app mode window.
// When the browser window is closed, it sends on the returned channel
// so the process can shut down. If the browser can't be waited on
// (fallback path), the channel is never closed.
func OpenDesktop(url string) <-chan struct{} {
	done := make(chan struct{})

	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		if openAppMode(url, done) {
			return done
		}
	}

	// Fallback: default browser (can't track when it closes)
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
	return done
}

// openAppMode launches a Chromium-based browser in --app mode.
// When the window closes, it signals on done.
func openAppMode(url string, done chan struct{}) bool {
	browser := findDesktopBrowser()
	if browser == "" {
		return false
	}

	dataDir := filepath.Join(os.TempDir(), "libro-app")

	cmd := exec.Command(browser,
		"--app="+url,
		"--user-data-dir="+dataDir,
		"--disable-extensions",
		"--disable-default-apps",
		"--no-first-run",
		"--start-maximized",
	)
	if err := cmd.Start(); err != nil {
		log.Printf("[desktop] --app mode failed (%s): %v", browser, err)
		return false
	}

	go func() {
		cmd.Wait()
		close(done)
	}()

	return true
}

// findDesktopBrowser returns the path to the first available Chromium-based browser.
func findDesktopBrowser() string {
	switch runtime.GOOS {
	case "windows":
		return findDesktopBrowserWindows()
	case "linux":
		return findDesktopBrowserLinux()
	default:
		return ""
	}
}

func findDesktopBrowserWindows() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
	}

	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			log.Printf("[desktop] found browser: %s", path)
			return path
		}
	}
	return ""
}

func findDesktopBrowserLinux() string {
	names := []string{
		"google-chrome-stable",
		"google-chrome",
		"chromium-browser",
		"chromium",
		"microsoft-edge-stable",
		"microsoft-edge",
		"brave-browser",
	}

	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			log.Printf("[desktop] found browser: %s", path)
			return path
		}
	}
	return ""
}
