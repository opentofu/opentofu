// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestSymbolLibraries(t *testing.T) {
	fixturePath := filepath.Join("testdata", "symbols")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	run := func(args ...string) tofuResult {
		stdout, stderr, err := tf.Run(args...)
		return tofuResult{t, stdout, stderr, err}
	}

	run("init").Success()
	run("plan", `-var=my_items=[""]`).Failure().StderrContains("One or more elements is empty")
	run("plan", `-var=my_items=["foo"]`).Success()
}
