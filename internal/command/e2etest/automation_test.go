// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/plans"
)

// TestPlanApplyInAutomation runs through the "main case" of init, plan, apply
// using the specific command line options suggested in the guide.
func TestPlanApplyInAutomation(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "full-workflow-null")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// We advertise that _any_ non-empty value works, so we'll test something
	// unconventional here.
	tf.AddEnv("TF_IN_AUTOMATION=yes-please")

	//// INIT
	stdout, stderr, err := tf.Run("init", "-input=false")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	// Make sure we actually downloaded the plugins, rather than picking up
	// copies that might be already installed globally on the system.
	if !strings.Contains(stdout, "Installing hashicorp/template v") {
		t.Errorf("template provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}
	if !strings.Contains(stdout, "Installing hashicorp/null v") {
		t.Errorf("null provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}

	//// PLAN
	stdout, stderr, err = tf.Run("plan", "-out=tfplan", "-input=false")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "1 to add, 0 to change, 0 to destroy") {
		t.Errorf("incorrect plan tally; want 1 to add:\n%s", stdout)
	}

	// Because we're running with TF_IN_AUTOMATION set, we should not see
	// any mention of the plan file in the output.
	if strings.Contains(stdout, "tfplan") {
		t.Errorf("unwanted mention of \"tfplan\" file in plan output\n%s", stdout)
	}

	plan, err := tf.Plan("tfplan")
	if err != nil {
		t.Fatalf("failed to read plan file: %s", err)
	}

	// stateResources := plan.Changes.Resources
	diffResources := plan.Changes.Resources
	if len(diffResources) != 1 {
		t.Errorf("incorrect number of resources in plan")
	}

	expected := map[string]plans.Action{
		"null_resource.test": plans.Create,
	}

	for _, r := range diffResources {
		expectedAction, ok := expected[r.Addr.String()]
		if !ok {
			t.Fatalf("unexpected change for %q", r.Addr)
		}
		if r.Action != expectedAction {
			t.Fatalf("unexpected action %q for %q", r.Action, r.Addr)
		}
	}

	//// APPLY
	stdout, stderr, err = tf.Run("apply", "-input=false", "tfplan")
	if err != nil {
		t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 1 added, 0 changed, 0 destroyed") {
		t.Errorf("incorrect apply tally; want 1 added:\n%s", stdout)
	}

	state, err := tf.LocalState()
	if err != nil {
		t.Fatalf("failed to read state file: %s", err)
	}

	stateResources := state.RootModule().Resources
	var gotResources []string
	for n := range stateResources {
		gotResources = append(gotResources, n)
	}
	sort.Strings(gotResources)

	wantResources := []string{
		"data.template_file.test",
		"null_resource.test",
	}

	if !reflect.DeepEqual(gotResources, wantResources) {
		t.Errorf("wrong resources in state\ngot: %#v\nwant: %#v", gotResources, wantResources)
	}
}

// TestAutoApplyInAutomation tests the scenario where the caller skips creating
// an explicit plan and instead forces automatic application of changes.
func TestAutoApplyInAutomation(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "full-workflow-null")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// We advertise that _any_ non-empty value works, so we'll test something
	// unconventional here.
	tf.AddEnv("TF_IN_AUTOMATION=very-much-so")

	//// INIT
	stdout, stderr, err := tf.Run("init", "-input=false")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	// Make sure we actually downloaded the plugins, rather than picking up
	// copies that might be already installed globally on the system.
	if !strings.Contains(stdout, "Installing hashicorp/template v") {
		t.Errorf("template provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}
	if !strings.Contains(stdout, "Installing hashicorp/null v") {
		t.Errorf("null provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}

	//// APPLY
	stdout, stderr, err = tf.Run("apply", "-input=false", "-auto-approve")
	if err != nil {
		t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 1 added, 0 changed, 0 destroyed") {
		t.Errorf("incorrect apply tally; want 1 added:\n%s", stdout)
	}

	state, err := tf.LocalState()
	if err != nil {
		t.Fatalf("failed to read state file: %s", err)
	}

	stateResources := state.RootModule().Resources
	var gotResources []string
	for n := range stateResources {
		gotResources = append(gotResources, n)
	}
	sort.Strings(gotResources)

	wantResources := []string{
		"data.template_file.test",
		"null_resource.test",
	}

	if !reflect.DeepEqual(gotResources, wantResources) {
		t.Errorf("wrong resources in state\ngot: %#v\nwant: %#v", gotResources, wantResources)
	}
}

// TestPlanOnlyInAutomation tests the scenario of creating a "throwaway" plan,
// which we recommend as a way to verify a pull request.
func TestPlanOnlyInAutomation(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "full-workflow-null")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// We advertise that _any_ non-empty value works, so we'll test something
	// unconventional here.
	tf.AddEnv("TF_IN_AUTOMATION=verily")

	//// INIT
	stdout, stderr, err := tf.Run("init", "-input=false")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	// Make sure we actually downloaded the plugins, rather than picking up
	// copies that might be already installed globally on the system.
	if !strings.Contains(stdout, "Installing hashicorp/template v") {
		t.Errorf("template provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}
	if !strings.Contains(stdout, "Installing hashicorp/null v") {
		t.Errorf("null provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}

	//// PLAN
	stdout, stderr, err = tf.Run("plan", "-input=false")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "1 to add, 0 to change, 0 to destroy") {
		t.Errorf("incorrect plan tally; want 1 to add:\n%s", stdout)
	}

	// Because we're running with TF_IN_AUTOMATION set, we should not see
	// any mention of the "tofu apply" command in the output.
	if strings.Contains(stdout, "tofu apply") {
		t.Errorf("unwanted mention of \"tofu apply\" in plan output\n%s", stdout)
	}

	if tf.FileExists("tfplan") {
		t.Error("plan file was created, but was not expected")
	}
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
