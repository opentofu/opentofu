// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/plans"
)

// The tests in this file are for the following sequence:
// tofu init
// tofu plan

func TestPlanConsolidatedWarningsForDeprecatedMarks(t *testing.T) {
	t.Parallel()

	implicitFixturePath := filepath.Join("testdata", "consolidated-warnings-for-deprecated-marks")
	tf := e2e.NewBinary(t, tofuBin, implicitFixturePath)

	t.Run("consolidated warnings for deprecated marks", func(t *testing.T) {
		_, initErr, err := tf.Run("init")
		if err != nil {
			t.Fatalf("expected no errors on init, got error %v: %s", err, initErr)
		}

		planStdout, planErr, err := tf.Run("plan")
		if err != nil {
			t.Fatalf("expected no errors on plan, got error %v: %s", err, planErr)
		}

		expectedOutput := `
No changes. Your infrastructure matches the configuration.
╷
│ Warning: Variable marked as deprecated by the module author
│ 
│   on main.tf line 3, in module "call":
│    3:   input  = "test"
│ 
│ Variable "input" is marked as deprecated with the following message:
│ this is local deprecated
│ 
│ (and one more similar warning elsewhere)
╵
╷
│ Warning: Variable marked as deprecated by the module author
│ 
│   on main.tf line 4, in module "call":
│    4:   input2 = "test2"
│ 
│ Variable "input2" is marked as deprecated with the following message:
│ this is local deprecated2
│ 
│ (and one more similar warning elsewhere)
╵
╷
│ Warning: Value derived from a deprecated source
│ 
│   on main.tf line 14, in locals:
│   14:   i1 = module.call.modout1
│ 
│ This value is derived from module.call.modout1, which is deprecated with
│ the following message:
│ 
│ output deprecated
│ 
│ (and one more similar warning elsewhere)
╵
╷
│ Warning: Value derived from a deprecated source
│ 
│   on main.tf line 15, in locals:
│   15:   i2 = module.call.modout2
│ 
│ This value is derived from module.call.modout2, which is deprecated with
│ the following message:
│ 
│ output deprecated
│ 
│ (and one more similar warning elsewhere)
╵
`
		if diff := cmp.Diff(strings.TrimSpace(stripAnsi(planStdout)), strings.TrimSpace(stripAnsi(expectedOutput))); diff != "" {
			t.Errorf("wrong output.\n%s\ngot: %s\nexpected: %s", diff, stripAnsi(planStdout), stripAnsi(expectedOutput))
		}
	})
}

func TestPlanOnDeprecated(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("testdata", "deprecated-values")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	//// INIT
	_, stderr, err := tf.Run("init", "-input=false")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	//// PLAN
	stdout, stderr, err := tf.Run("plan")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}

	expected := []string{
		`Variable marked as deprecated by the module author`,
		`Variable "input" is marked as deprecated with the following message`,
		`This var is deprecated`,
		`Value derived from a deprecated source`,
		`This value is derived from module.call.output, which is deprecated with the`,
		`following message:`,
		`this output is deprecated`,
	}
	for _, want := range expected {
		if !strings.Contains(stdout, want) {
			t.Errorf("invalid plan output. expected to contain %q but it does not:\n%s", want, stdout)
		}
	}
}

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

		planStdout, planErr, err := tf.Run("plan")
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

// TestPlanShowWithEmbeddedSchema is a regression test for [the bug](https://github.com/opentofu/opentofu/issues/4622) where
// a configuration containing ephemeral is shown correctly in json format with the embedded schema in the plan.
func TestPlanShowWithEmbeddedSchema(t *testing.T) {
	t.Parallel()

	workdir := "testdata/plan-with-embedded-ephemeral-config"
	buildSimpleProvider(t, "6", workdir, "simple")
	tf := e2e.NewBinary(t, tofuBin, workdir)

	{ // INIT
		_, stderr, err := tf.Run("init", "-plugin-dir=cache")
		if err != nil {
			t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
		}
	}

	{ // PLAN
		stdout, stderr, err := tf.Run("plan", "-out=tfplan")
		if err != nil {
			t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
		}
		expectedChangesOutput := ``

		checker := outputEntriesChecker{
			outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral: Opening..."}, true},
			outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral: Open complete after"}, true},
			outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral: Closing..."}, true},
			outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral: Close complete after"}, true},
			outputCheckContains{[]string{`+ resource "simple_resource" "test_res"`}, true},
		}
		out := stripAnsi(stdout)

		if !strings.Contains(out, expectedChangesOutput) {
			t.Errorf("wrong plan output:\nstdout:%s\nstderr:%s", stdout, stderr)
			t.Log(cmp.Diff(out, expectedChangesOutput))
		}
		checker.check(t, "plan", out)

		// assert plan file content
		plan, err := tf.Plan("tfplan")
		if err != nil {
			t.Fatalf("failed to read the plan file: %s", err)
		}
		idx := slices.IndexFunc(plan.Changes.Resources, func(src *plans.ResourceInstanceChangeSrc) bool {
			return src.Addr.Resource.Resource.Mode == addrs.EphemeralResourceMode
		})
		if idx >= 0 {
			t.Fatalf("ephemeral resource found in the plan file. expected to have no ephemeral resource")
		}
	}
	{ // SHOW -json
		stdout, stderr, err := tf.Run("show", "-json", "tfplan")
		if err != nil {
			t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
		}
		// a better way to do this would be to enhance `jsonconfig` package with capabilities to unmarshal the generated output and do better checks on the structured data.
		// But to add such a complex logic strictly for this particular test does not worth right now. If we'll find more use cases where that would be useful
		// we can create that later and replace this assertion.
		checker := outputEntriesChecker{
			outputCheckNumberOfOccurrences{
				token:             `{"address":"ephemeral.simple_resource.test_ephemeral","mode":"ephemeral","type":"simple_resource","name":"test_ephemeral","provider_config_key":"simple"`,
				wantedOccurrences: 1,
			},
		}
		out := stripAnsi(stdout)
		checker.check(t, "show -json", out)
	}
	{ // SHOW (human)
		stdout, stderr, err := tf.Run("show", "tfplan")
		if err != nil {
			t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
		}
		checker := outputEntriesChecker{
			outputCheckNumberOfOccurrences{
				token:             `+ resource "simple_resource" "test_res"`,
				wantedOccurrences: 1,
			},
		}
		out := stripAnsi(stdout)
		checker.check(t, "show", out)
	}
}
