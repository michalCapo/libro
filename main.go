package main

import (
	"embed"
	"fmt"
	"os"
	"time"

	"libro/internal"
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

	// Handle --desktop flag: start server then open app window
	desktop := false
	for _, arg := range os.Args[1:] {
		if arg == "--desktop" {
			desktop = true
		}
	}

	if desktop {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openDesktop("http://localhost:1439")
		}()
	}

	libro.Run(assets)
}
