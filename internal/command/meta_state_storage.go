// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"path/filepath"

	"github.com/opentofu/opentofu/internal/states/statestore"
)

const prototypeGranularStateStorageDir = "state-storage-prototype"

func (m *Meta) stateStorage() (statestore.Storage, error) {
	// For initial prototyping purposes we just always use the filesystem
	// implementation of storage for now.

	storagePath := filepath.Join(m.DataDir(), prototypeGranularStateStorageDir)
	err := os.MkdirAll(storagePath, os.ModePerm)
	if err != nil {
		return nil, err
	}
	return statestore.OpenFilesystemStorage(storagePath)
}
