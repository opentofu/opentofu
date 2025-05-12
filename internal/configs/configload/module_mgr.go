// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/opentofu/opentofu/internal/modsdir"
)

type moduleMgr struct {
	FS afero.Afero

	// CanInstall is true for a module manager that can support installation.
	//
	// This must be set only if FS is an afero.OsFs, because the installer
	// (which uses go-getter) is not aware of the virtual filesystem
	// abstraction and will always write into the "real" filesystem.
	CanInstall bool

	// Dir is the path where descendent modules are (or will be) installed.
	Dir string

	// manifest tracks the currently-installed modules for this manager.
	//
	// The loader may read this. Only the installer may write to it, and
	// after a set of updates are completed the installer must call
	// writeModuleManifestSnapshot to persist a snapshot of the manifest
	// to disk for use on subsequent runs.
	manifest modsdir.Manifest
}

func (m *moduleMgr) manifestSnapshotPath() string {
	return filepath.Join(m.Dir, modsdir.ManifestSnapshotFilename)
}

// readModuleManifestSnapshot loads a manifest snapshot from the filesystem.
func (m *moduleMgr) readModuleManifestSnapshot() error {
	r, err := m.FS.Open(m.manifestSnapshotPath())
	if err != nil {
		if os.IsNotExist(err) {
			// We'll treat a missing file as an empty manifest
			m.manifest = make(modsdir.Manifest)
			return nil
		}
		return err
	}

	m.manifest, err = modsdir.ReadManifestSnapshot(r)
	return err
}
