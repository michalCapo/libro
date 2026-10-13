// Package desktopbundledata embeds the generated Electron app files, runtime zip and CLI tools used to build desktop bundles.
package desktopbundledata

import "embed"

// Files contains generated desktop app files, a single platform Electron runtime zip
// and, on Linux, the matching CLI tools archive from the ide project.
// The build scripts populate these directories before each target build.
//
//go:embed all:electron-app all:electron-runtime all:cli-tools
var Files embed.FS
