package main

import (
	"embed"
	"fmt"
	"os"

	libro "libro/internal"
	"libro/internal/version"
)

//go:embed assets/*
var assets embed.FS

func main() {
	// Handle --version flag
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println("libro", version.Version)
			return
		}
	}

	// Desktop mode is default — use --no-desktop to skip opening the browser window
	desktop := true
	for _, arg := range os.Args[1:] {
		if arg == "--no-desktop" {
			desktop = false
		}
	}

	if desktop {
		go func() {
			done := libro.OpenDesktop("http://localhost:" + libro.Port())

			// When the browser window closes, exit the process
			<-done
			os.Exit(0)
		}()
	}

	libro.Run(assets)
}
