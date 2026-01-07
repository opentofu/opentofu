// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestMultipleRunBlocks(t *testing.T) {
	timeout := time.After(5 * time.Second)
	type testResult struct {
		stdout string
		stderr string
		err    error
	}
	done := make(chan *testResult)

	go func() {
		fixturePath := filepath.Join("testdata", "multiple-run-blocks")
		tf := e2e.NewBinary(t, tofuBin, fixturePath)
		stdout, stderr, err := tf.Run("test")
		done <- &testResult{
			stdout: stdout,
			stderr: stderr,
			err:    err,
		}
	}()

	select {
	case <-timeout:
		t.Fatal("timed out")
	case result := <-done:
		if result.err != nil {
			t.Errorf("unexpected error: %s", result.err)
		}

		if result.stderr != "" {
			t.Errorf("unexpected stderr output:\n%s", result.stderr)
		}

		if !strings.Contains(result.stdout, "30 passed") {
			t.Errorf("success message is missing from output:\n%s", result.stdout)
		}
	}
}

func TestMocksAndOverrides(t *testing.T) {
	// This test fetches providers from registry.
	skipIfCannotAccessNetwork(t)

	tf := e2e.NewBinary(t, tofuBin, filepath.Join("testdata", "overrides-in-tests"))

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error on 'init': %v", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output on 'init':\n%s", stderr)
	}
	if stdout == "" {
		t.Errorf("expected some output on 'init', got nothing")
	}

	stdout, stderr, err = tf.Run("test")
	if err != nil {
		t.Errorf("unexpected error on 'test': %v", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output on 'test':\n%s", stderr)
	}
	if !strings.Contains(stdout, "15 passed, 0 failed") {
		t.Errorf("output doesn't have expected success string:\n%s", stdout)
	}
}

// TestMockProviderComputedBlockCleanup ensures we don't regress
// a fix for this issue https://github.com/opentofu/opentofu/issues/3644
//
// The bug occurs when:
// 1. A resource has lifecycle { ignore_changes = [block] } on a BLOCK (not a simple attribute)
// 2. mock_provider is used
// 3. Cleanup/destroy runs after apply
// The cleanup fails with "Config value can not be specified for computed field"
func TestMockProviderComputedBlockCleanup(t *testing.T) {
	// This test fetches providers from registry.
	skipIfCannotAccessNetwork(t)

	tf := e2e.NewBinary(t, tofuBin, filepath.Join("testdata", "mock-computed-block-cleanup"))

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error on 'init': %v", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output on 'init':\n%s", stderr)
	}
	if stdout == "" {
		t.Errorf("expected some output on 'init', got nothing")
	}

	stdout, stderr, err = tf.Run("test")
	if err != nil {
		if strings.Contains(stdout, "Config value can not be specified for computed field") {
			t.Errorf("Bug reproduced: mock provider fails with computed field error.\n"+
				"This is the bug from https://github.com/opentofu/opentofu/issues/3644\n"+
				"stdout:\n%s", stdout)
			return
		}
		t.Errorf("unexpected error on 'test': %v\nstderr:\n%s\nstdout:\n%s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "1 passed, 0 failed") {
		t.Errorf("output doesn't have expected success string:\n%s", stdout)
	}
}
