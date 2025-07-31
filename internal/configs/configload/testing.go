// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"testing"
)

// NewLoaderForTests is a variant of NewLoader that is intended to be more
// convenient for unit tests.
//
// The loader's modules directory is a separate temporary directory created
// for each call. The temporary directory is deleted automatically at the
// conclusion of the test.
//
// In the case of any errors, t.Fatal (or similar) will be called to halt
// execution of the test, so the calling test does not need to handle errors
// itself.
func NewLoaderForTests(t testing.TB) *Loader {
	t.Helper()

	modulesDir := t.TempDir()
	loader, err := NewLoader(&Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatalf("failed to create config loader: %s", err)
		return nil
	}
	return loader
}
