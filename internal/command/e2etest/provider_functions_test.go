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

func TestFunction_Simple(t *testing.T) {
	// This test reaches out to registry.opentofu.org to download the
	// test functions provider, so it can only run if network access is allowed
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "functions")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// tofu init
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	_, stderr, err = tf.Run("plan", "-out=fnplan")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	plan, err := tf.Plan("fnplan")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if len(plan.Changes.Outputs) != 1 {
		t.Fatalf("expected 1 outputs, got %d", len(plan.Changes.Outputs))
	}
	for _, out := range plan.Changes.Outputs {
		if !strings.Contains(string(out.After), "Hello Functions") {
			t.Fatalf("unexpected plan output: %s", string(out.After))
		}
	}
}

func TestFunction_ProviderDefinedFunctionWithoutConfigure(t *testing.T) {
	// This test reaches out to registry.opentofu.org to download the
	// test functions provider, so it can only run if network access is allowed
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "functions_aws")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// tofu init
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	_, stderr, err = tf.Run("plan", "-out=fnplan")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr output:\n%s", stderr)
	}

	plan, err := tf.Plan("fnplan")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if len(plan.Changes.Outputs) != 1 {
		t.Fatalf("expected 1 outputs, got %d", len(plan.Changes.Outputs))
	}

	for _, out := range plan.Changes.Outputs {
		if !strings.Contains(string(out.After), "arn:aws:s3:::bucket-prod") {
			t.Fatalf("unexpected plan output: %s", string(out.After))
		}
	}
}

func TestFunction_Error(t *testing.T) {
	// This test reaches out to registry.opentofu.org to download the
	// test functions provider, so it can only run if network access is allowed
	skipIfCannotAccessNetwork(t)
	fixturePath := filepath.Join("testdata", "functions-error")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// tofu init
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	// tofu plan -out=fnplan
	_, stderr, err = tf.Run("plan", "-out=fnplan")
	if err == nil {
		t.Errorf("expected error: %s", err)
	}
	if !strings.Contains(stderr, "Call to function \"provider::example::error\" failed") {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}
}
