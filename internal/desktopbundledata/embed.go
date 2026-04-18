package desktopbundledata

import "embed"

// Files contains generated desktop app files and a single platform Electron runtime zip.
// The release script populates these directories before each target build.
//
//go:embed all:electron-app all:electron-runtime
var Files embed.FS
