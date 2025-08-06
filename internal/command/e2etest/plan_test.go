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

func TestPlanOnMultipleDeprecatedMarksSliceBug(t *testing.T) {
	t.Parallel()

	// Test for [the bug](https://github.com/opentofu/opentofu/issues/3104) where modifying
	// pathMarks slice during iteration would cause slice bounds errors when multiple
	// deprecated marks exist
	fixturePath := filepath.Join("testdata", "multiple-deprecated-marks-slice-bug")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	t.Run("multiple deprecated marks slice bug", func(t *testing.T) {
		_, initErr, err := tf.Run("init")
		if err != nil {
			t.Fatalf("expected no errors on init, got error %v: %s", err, initErr)
		}

		planStdout, planErr, err := tf.Run("plan", "-consolidate-warnings=false")
		if err != nil {
			t.Fatalf("expected no errors on plan, got error %v: %s", err, planErr)
		}

		// Should not crash and should show deprecation warnings for all outputs
		expectedContents := []string{
			"Changes to Outputs:",
			"trigger = {",
			"Value derived from a deprecated source",
			"Use new_out1",
			"Use new_out2",
			"Use new_out3",
		}

		// Strip ANSI codes for consistent testing
		cleanOutput := stripAnsi(planStdout)
		for _, want := range expectedContents {
			if !strings.Contains(cleanOutput, want) {
				t.Errorf("plan output missing expected content %q:\n%s", want, cleanOutput)
			}
		}
	})
}
