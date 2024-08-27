// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/terramate-io/opentofulib/internal/e2e"
)

func TestInitModuleArchive(t *testing.T) {
	t.Parallel()

	// this fetches a module archive from github
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "module-archive")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	if !strings.Contains(stdout, "OpenTofu has been successfully initialized!") {
		t.Errorf("success message is missing from output:\n%s", stdout)
	}
}
