package mcptools

import (
	"embed"
	"io/fs"
)

//go:embed mcp-tools/*
var embedFS embed.FS

func GetEmbeddedFS() fs.FS {
	sub, err := fs.Sub(embedFS, "mcp-tools")
	if err != nil {
		// Return root if subdirectory doesn't exist
		return embedFS
	}
	return sub
}
