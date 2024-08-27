// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"testing"

	"github.com/terramate-io/opentofulib/internal/e2e"
)

func TestOpenTofuProviderRead(t *testing.T) {
	// Ensure the tofu provider can correctly read a remote state

	t.Parallel()
	fixturePath := filepath.Join("testdata", "tf-provider")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	//// INIT
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	//// PLAN
	_, stderr, err = tf.Run("plan")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}
}
