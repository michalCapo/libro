package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openDesktop opens the libro UI in a Chromium --app mode window,
// creating a desktop-app-like experience (no URL bar, own taskbar entry).
func openDesktop(url string) {
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		if openAppMode(url) {
			return
		}
	}

	// Fallback: default browser
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

// openAppMode launches a Chromium-based browser in --app mode.
func openAppMode(url string) bool {
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
