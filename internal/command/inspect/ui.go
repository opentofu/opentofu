// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package inspect

import (
	"embed"
	"io/fs"
	"net/http"
)

// Embed the built React app
//go:embed ui/dist/*
var uiFS embed.FS

// GetUIFileSystem returns the embedded UI filesystem
// This strips the "ui/dist" prefix so files can be served from root
func GetUIFileSystem() http.FileSystem {
	// Get the subdirectory without the "ui/dist" prefix
	sub, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		// If the dist directory doesn't exist (during development),
		// return an empty filesystem
		return http.Dir(".")
	}
	
	return http.FS(sub)
}