package ui

import (
	"embed"
	"io/fs"
)

//go:embed static/* fonts/*
var embeddedFS embed.FS

// StaticFS returns the static files (HTML, CSS, JS)
func StaticFS() fs.FS {
	sub, _ := fs.Sub(embeddedFS, "static")
	return sub
}

// FontsFS returns the embedded font files
func FontsFS() fs.FS {
	sub, _ := fs.Sub(embeddedFS, "fonts")
	return sub
}
