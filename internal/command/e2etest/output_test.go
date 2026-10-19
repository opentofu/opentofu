// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// TestModuleOutputTypeAnnotation verifies that a type annotation
func TestModuleOutputTypeAnnotation(t *testing.T) {
	t.Parallel()
	fixtureDir := filepath.Join("testdata", "output-typed")
	tf := e2e.NewBinary(t, tofuBin, fixtureDir)
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error on init, got error %v: %s", err, stderr)
	}
	stdout, stderr, err := tf.Run("plan", "-input=false", "-no-color")
	if err != nil {
		t.Fatalf("plan failed: %s\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `Changes to Outputs:
  + result = {
      + a = "1"
      + b = "2"
    }
`) {
		t.Fatalf("contained invalid type output: %s", stdout)
	}
}

// TestModuleOutputTypeMismatchError covers the failure path: the returned
// value cannot be converted to the annotated type.
func TestModuleOutputTypeMismatchError(t *testing.T) {
	t.Parallel()

	fixtureDir := filepath.Join("testdata", "invalid-output-typed")
	tf := e2e.NewBinary(t, tofuBin, fixtureDir)
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error on init, got error %v: %s", err, stderr)
	}
	_, stderr, err = tf.Run("plan", "-no-color", "-input=false")
	if err == nil {
		t.Fatalf("expected plan to fail on type mismatch, but it succeeded")
	}
	expectOutput := `Value for "output.result" does not match the type definition: a number is
required.
`
	if !strings.Contains(stderr, expectOutput) {
		t.Errorf("expected invalid output error, got %s", stderr)
	}

}
