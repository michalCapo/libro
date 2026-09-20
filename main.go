// Command libro is the Libro CLI entry point that boots the HTTP server and optional desktop window.
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
	if len(os.Args) > 1 && (os.Args[1] == "browser" || os.Args[1] == "browser-mcp") {
		var err error
		if os.Args[1] == "browser-mcp" {
			err = libro.RunBrowserMCP(os.Stdin, os.Stdout)
		} else {
			err = libro.RunBrowserCLI(os.Args[2:], os.Stdout)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
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
			libro.CleanupRuntime()
			libro.CloseDB()
			os.Exit(0)
		}()
	}

	libro.Run(assets)
}
