package main

import (
	"embed"

	"libro/internal"
)

//go:embed assets/*
var assets embed.FS

func main() {
	libro.Run(assets)
}
