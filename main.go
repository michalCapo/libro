// Command libro is the Libro CLI entry point that boots the HTTP server and optional desktop window.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"os"
	"time"

	libro "libro/internal"
	"libro/internal/version"
)

//go:embed assets/*
var assets embed.FS

func main() {
	flags := flag.NewFlagSet("libro", flag.ExitOnError)
	dev := flags.Bool("dev", false, "Run an isolated development instance")
	instance := flags.String("instance", os.Getenv("LIBRO_INSTANCE"), "Instance name (separate data and browser profile)")
	port := flags.String("port", os.Getenv("LIBRO_PORT"), "HTTP port (default 8100, or 8101 for named instances)")
	noDesktop := flags.Bool("no-desktop", false, "Run without a desktop window")
	showVersion := flags.Bool("version", false, "Print version")
	flags.BoolVar(showVersion, "v", false, "Print version")
	_ = flags.Parse(os.Args[1:])
	if *showVersion {
		fmt.Println("libro", version.Version)
		return
	}
	if *dev {
		*instance = "dev"
	}
	// A new profile must not reuse the parent terminal's instance port.
	explicitPort := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			explicitPort = true
		}
	})
	if *instance != os.Getenv("LIBRO_INSTANCE") && !explicitPort {
		*port = ""
	}
	if err := libro.ConfigureInstance(*instance, *port); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	args := flags.Args()
	var err error
	if len(args) > 0 {
		switch args[0] {
		case "voice":
			if len(args) != 2 || args[1] != "install" {
				err = fmt.Errorf("usage: libro voice install")
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
				defer cancel()
				err = libro.InstallVoice(ctx)
			}
		case "issues":
			err = libro.RunIssuesCLI(args[1:], os.Stdout)
		case "application":
			err = libro.RunApplicationCLI(args[1:], os.Stdout)
		case "mcp":
			err = libro.RunMCP(os.Stdin, os.Stdout)
		default:
			err = fmt.Errorf("unknown command: %s", args[0])
		}
	} else {
		err = libro.Run(assets, !*noDesktop)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}
