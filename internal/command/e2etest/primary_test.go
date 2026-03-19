// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/zclconf/go-cty/cty"
)

// The tests in this file are for the "primary workflow", which includes
// variants of the following sequence, with different details:
// tofu init
// tofu plan
// tofu apply
// tofu destroy

func TestPrimarySeparatePlan(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "full-workflow-null")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// INIT
	stdout, stderr, err := tf.Run("init")
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

	// PLAN
	stdout, stderr, err = tf.Run("plan", "-out=tfplan")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "1 to add, 0 to change, 0 to destroy") {
		t.Errorf("incorrect plan tally; want 1 to add:\n%s", stdout)
	}

	if !strings.Contains(stdout, "Saved the plan to: tfplan") {
		t.Errorf("missing \"Saved the plan to...\" message in plan output\n%s", stdout)
	}
	if !strings.Contains(stdout, "tofu apply \"tfplan\"") {
		t.Errorf("missing next-step instruction in plan output\n%s", stdout)
	}

	plan, err := tf.Plan("tfplan")
	if err != nil {
		t.Fatalf("failed to read plan file: %s", err)
	}

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

	// APPLY
	stdout, stderr, err = tf.Run("apply", "tfplan")
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

	// DESTROY
	stdout, stderr, err = tf.Run("destroy", "-auto-approve")
	if err != nil {
		t.Fatalf("unexpected destroy error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 1 destroyed") {
		t.Errorf("incorrect destroy tally; want 1 destroyed:\n%s", stdout)
	}

	state, err = tf.LocalState()
	if err != nil {
		t.Fatalf("failed to read state file after destroy: %s", err)
	}

	stateResources = state.RootModule().Resources
	if len(stateResources) != 0 {
		t.Errorf("wrong resources in state after destroy; want none, but still have:%s", spew.Sdump(stateResources))
	}

}

func TestPrimaryChdirOption(t *testing.T) {
	t.Parallel()

	// This test case does not include any provider dependencies, so it's
	// safe to run it even when network access is disallowed.

	fixturePath := filepath.Join("testdata", "chdir-option")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// INIT
	_, stderr, err := tf.Run("-chdir=subdir", "init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	// PLAN
	stdout, stderr, err := tf.Run("-chdir=subdir", "plan", "-out=tfplan")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}

	if want := "You can apply this plan to save these new output values"; !strings.Contains(stdout, want) {
		t.Errorf("missing expected message for an outputs-only plan\ngot:\n%s\n\nwant substring: %s", stdout, want)
	}

	if !strings.Contains(stdout, "Saved the plan to: tfplan") {
		t.Errorf("missing \"Saved the plan to...\" message in plan output\n%s", stdout)
	}
	if !strings.Contains(stdout, "tofu apply \"tfplan\"") {
		t.Errorf("missing next-step instruction in plan output\n%s", stdout)
	}

	// The saved plan is in the subdirectory because -chdir switched there
	plan, err := tf.Plan("subdir/tfplan")
	if err != nil {
		t.Fatalf("failed to read plan file: %s", err)
	}

	diffResources := plan.Changes.Resources
	if len(diffResources) != 0 {
		t.Errorf("incorrect diff in plan; want no resource changes, but have:\n%s", spew.Sdump(diffResources))
	}

	// APPLY
	stdout, stderr, err = tf.Run("-chdir=subdir", "apply", "tfplan")
	if err != nil {
		t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 0 added, 0 changed, 0 destroyed") {
		t.Errorf("incorrect apply tally; want 0 added:\n%s", stdout)
	}

	// The state file is in subdir because -chdir changed the current working directory.
	state, err := tf.StateFromFile("subdir/terraform.tfstate")
	if err != nil {
		t.Fatalf("failed to read state file: %s", err)
	}

	gotOutput := state.RootModule().OutputValues["cwd"]
	wantOutputValue := cty.StringVal(filepath.ToSlash(tf.Path())) // path.cwd returns the original path, because path.root is how we get the overridden path
	if gotOutput == nil || !wantOutputValue.RawEquals(gotOutput.Value) {
		t.Errorf("incorrect value for cwd output\ngot: %#v\nwant Value: %#v", gotOutput, wantOutputValue)
	}

	gotOutput = state.RootModule().OutputValues["root"]
	wantOutputValue = cty.StringVal(filepath.ToSlash(tf.Path("subdir"))) // path.root is a relative path, but the text fixture uses abspath on it.
	if gotOutput == nil || !wantOutputValue.RawEquals(gotOutput.Value) {
		t.Errorf("incorrect value for root output\ngot: %#v\nwant Value: %#v", gotOutput, wantOutputValue)
	}

	if len(state.RootModule().Resources) != 0 {
		t.Errorf("unexpected resources in state")
	}

	// DESTROY
	stdout, stderr, err = tf.Run("-chdir=subdir", "destroy", "-auto-approve")
	if err != nil {
		t.Fatalf("unexpected destroy error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 0 destroyed") {
		t.Errorf("incorrect destroy tally; want 0 destroyed:\n%s", stdout)
	}
}

// This test is checking the workflow of the ephemeral resources.
// Check also the configuration files for comments.
//
// We want to validate that the plan file, state file and the output contain
// only the things that are needed:
//   - The plan file needs to contain **only** the stubs of the ephemeral resources
//     and not the values that it generated. This is needed for `tofu apply planfile`
//     to be able to generate the execution node graphs correctly.
//   - The state file must not contain the ephemeral resources changes.
//   - The output should contain no changes related to ephemeral resources, but only
//     the status update of their execution.
func TestEphemeralWorkflowAndOutput(t *testing.T) {
	t.Parallel()

	pluginVersionRunner := func(t *testing.T, testdataPath string, providerBuilderFunc func(*testing.T, string)) {
		tf := e2e.NewBinary(t, tofuBin, testdataPath)
		providerBuilderFunc(t, tf.WorkDir())

		{ // INIT
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			_, stderr, err := tf.RunCtx(ctx, "init", "-plugin-dir=cache")
			if err != nil {
				t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
			}
		}

		{ // PLAN
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			stdout, stderr, err := tf.RunCtx(ctx, "plan", "-out=tfplan", `-var=simple_input=plan_val`, `-var=ephemeral_input=ephemeral_val`)
			if err != nil {
				t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
			}
			expectedChangesOutput := `OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create
 <= read (data resources)

OpenTofu will perform the following actions:

  # simple_resource.test_res will be created
  + resource "simple_resource" "test_res" {
      + id       = (known after apply)
      + value    = "test value"
      + value_wo = (write-only attribute)
    }

  # simple_resource.test_res_second_provider will be created
  + resource "simple_resource" "test_res_second_provider" {
      + id       = (known after apply)
      + value    = "just a simple resource to ensure that the second provider it's working fine"
      + value_wo = (write-only attribute)
    }

  # module.call.data.simple_resource.deferred_data will be read during apply
  # (depends on a resource or a module with changes pending)
 <= data "simple_resource" "deferred_data" {
      + id    = (known after apply)
      + value = "hardcoded"
    }

Plan: 2 to add, 0 to change, 0 to destroy.

Changes to Outputs:
  + final_output = "just a simple resource to ensure that the second provider it's working fine"`

			checker := outputEntriesChecker{
				outputCheckContains{[]string{"data.simple_resource.test_data1: Reading..."}, true},
				outputCheckContains{[]string{"data.simple_resource.test_data1: Read complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Opening..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Open complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Opening..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Open complete after"}, true},
				outputCheckContains{[]string{"module.call.ephemeral.simple_resource.deferred_ephemeral: Deferred due to unknown configuration"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Closing..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Close complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Closing..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Close complete after"}, true},
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
			// variables check
			varDynVal, ok := plan.VariableValues["simple_input"]
			if !ok {
				t.Fatalf("expected the %q to exist but it does not", "simple_input")
			}
			varVal, diags := varDynVal.Decode(cty.DynamicPseudoType)
			if diags != nil {
				t.Fatalf("expected no diags from decoding the variable value but got one: %s", diags)
			}
			expectedVal := cty.StringVal("plan_val")
			if expectedVal.Equals(varVal).False() {
				t.Errorf("unexpected value saved in the plan object. expected: %s; got: %s", expectedVal.GoString(), varVal.GoString())
			}
			// ephemeral variables are to be missing from plan.VariableValues but to be found in plan.EphemeralVariables
			varDynVal, ok = plan.VariableValues["ephemeral_input"]
			if ok {
				t.Errorf("expected variable %q to be missing from plan.VariableValues but got %s", "ephemeral_input", varDynVal)
			}
			// ensure that plan.EphemeralVariables is registered as expected
			if plan.EphemeralVariables == nil {
				t.Errorf("plan.EphemeralVariables is meant to be initialised when reading the plan since the empty value variables are marked as ephemeral=true")
			}
			expectedEphemeralVariables := map[string]bool{
				"ephemeral_input": true,
				"simple_input":    false,
			}
			if diff := cmp.Diff(plan.EphemeralVariables, expectedEphemeralVariables); diff != "" {
				t.Errorf("invalid content of plan.EphemeralVariables: %s", diff)
			}
		}

		{ // APPLY with wrong variables
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			expectedToContain := `╷ Error: Mismatch between input and plan variable value  Value saved in the plan file for variable "simple_input" is different from the one given to the current command.╵`
			expectedErr := fmt.Errorf("exit status 1")
			_, stderr, err := tf.RunCtx(ctx, "apply", `-var=simple_input=different_from_the_plan_one`, `-var=ephemeral_input=ephemeral_val`, "tfplan")
			if err == nil {
				t.Fatalf("expected an error but got nothing")
			}
			if got, want := err.Error(), expectedErr.Error(); got != want {
				t.Fatalf("expected err %q but got %q", want, got)
			}
			cleanStderr := SanitizeStderr(stderr)
			if cleanStderr != expectedToContain {
				t.Errorf("expected an error message but didn't get it.\nexpected:\n%s\n\ngot:\n%s\n", expectedToContain, cleanStderr)
			}
		}

		{ // APPLY with no ephemeral variable value
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			expectedToContain := "╷ Error: No value for required variable    on main.tf line 15:   15: variable \"ephemeral_input\" {  Variable \"ephemeral_input\" is configured as ephemeral. This type of variables need to be given a value during `tofu plan` and also during `tofu apply`.╵"
			expectedErr := fmt.Errorf("exit status 1")
			_, stderr, err := tf.RunCtx(ctx, "apply", `-var=simple_input=plan_val`, "tfplan")
			if err == nil {
				t.Fatalf("expected an error but got nothing")
			}
			if got, want := err.Error(), expectedErr.Error(); got != want {
				t.Fatalf("expected err %q but got %q", want, got)
			}
			cleanStderr := SanitizeStderr(stderr)
			if cleanStderr != expectedToContain {
				t.Errorf("expected an error message but didn't get it.\nexpected:\n%s\n\ngot:\n%s\n", expectedToContain, cleanStderr)
			}
		}
		{ // APPLY
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			stdout, stderr, err := tf.RunCtx(ctx, "apply", `-var=simple_input=plan_val`, `-var=ephemeral_input=ephemeral_val`, "tfplan")
			if err != nil {
				t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
			}
			state, err := tf.LocalState()
			if err != nil {
				t.Fatalf("failed to read local state: %s", err)
			}
			expectedResources := map[string]bool{
				"data.simple_resource.test_data1":                true,
				"module.call.data.simple_resource.deferred_data": true,
				"simple_resource.test_res":                       true,
				"simple_resource.test_res_second_provider":       true,
				"ephemeral.simple_resource.test_ephemeral":       false,
			}
			for res, exists := range expectedResources {
				addr := mustResourceInstanceAddr(t, res)
				mod := state.Module(addr.Module)
				if mod == nil {
					t.Errorf("state misses state for module %q as checking the state for %q", addr.Module, addr)
					continue
				}
				resStateExists := mod.ResourceInstance(addr.Resource) != nil
				if resStateExists != exists {
					t.Errorf("expected resource %q existence to be %t but got %t", res, exists, resStateExists)
				}
			}

			expectedChangesOutput := `Apply complete! Resources: 2 added, 0 changed, 0 destroyed.`
			// NOTE: the non-required ones are dependent on the performance of the platform that this test is running on.
			// In CI, if we would make this as required, this test might be flaky.
			checker := outputEntriesChecker{
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Opening..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Open complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Opening..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Open complete after"}, true},
				outputCheckContains{[]string{"module.call.ephemeral.simple_resource.deferred_ephemeral: Opening..."}, true},
				outputCheckContains{[]string{"module.call.ephemeral.simple_resource.deferred_ephemeral: Open complete after"}, true},
				outputCheckContains{[]string{"module.call.data.simple_resource.deferred_data: Reading..."}, true},
				outputCheckContains{[]string{"module.call.data.simple_resource.deferred_data: Read complete after"}, true},
				outputCheckContains{[]string{"simple_resource.test_res: Creating..."}, true},
				outputCheckContains{[]string{"simple_resource.test_res_second_provider: Creating..."}, true},
				outputCheckContains{[]string{"simple_resource.test_res_second_provider: Creation complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Renewing..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Renew complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Renewing..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Renew complete after"}, true},
				outputCheckContains{[]string{"simple_resource.test_res: Creation complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Closing..."}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Close complete after"}, true},
				outputCheckContains{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Closing..."}, true},
				outputCheckContains{[]string{"simple_resource.test_res: Provisioning with 'local-exec'..."}, true},
				outputCheckContains{[]string{
					`simple_resource.test_res (local-exec): Executing: ["/bin/sh" "-c" "echo \"visible test value\""]`,
					`simple_resource.test_res (local-exec): Executing: ["cmd" "/C" "echo \"visible test value\""]`,
				}, true},
				outputCheckContains{[]string{
					`simple_resource.test_res (local-exec): visible test value`,
					`simple_resource.test_res (local-exec): \"visible test value\"`,
				}, true},
				outputCheckContains{[]string{"simple_resource.test_res (local-exec): (output suppressed due to ephemeral value in config)"}, true},
			}
			out := stripAnsi(stdout)

			if !strings.Contains(out, expectedChangesOutput) {
				t.Errorf("wrong apply output:\nstdout:%s\nstderr%s", stdout, stderr)
				t.Log(cmp.Diff(out, expectedChangesOutput))
			}
			checker.check(t, "apply", out)
		}
		{ // DESTROY
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			stdout, stderr, err := tf.RunCtx(ctx, "destroy", `-var=simple_input=plan_val`, `-var=ephemeral_input=ephemeral_val`, "-auto-approve")
			if err != nil {
				t.Fatalf("unexpected destroy error: %s\nstderr:\n%s", err, stderr)
			}

			if !strings.Contains(stdout, "Resources: 2 destroyed") {
				t.Errorf("incorrect destroy tally; want 2 destroyed:\n%s", stdout)
			}

			state, err := tf.LocalState()
			if err != nil {
				t.Fatalf("failed to read state file after destroy: %s", err)
			}

			stateResources := state.RootModule().Resources
			if len(stateResources) != 0 {
				t.Errorf("wrong resources in state after destroy; want none, but still have:%s", spew.Sdump(stateResources))
			}
		}
	}

	cases := map[string]struct {
		protoBinBuilder func(t *testing.T, workdir string)
	}{
		"proto version 5": {
			protoBinBuilder: func(t *testing.T, workdir string) {
				buildSimpleProvider(t, "5", workdir, "simple")
			},
		},
		"proto version 6": {
			protoBinBuilder: func(t *testing.T, workdir string) {
				buildSimpleProvider(t, "6", workdir, "simple")
			},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pluginVersionRunner(t, "testdata/ephemeral-workflow", tt.protoBinBuilder)
		})
	}
}

func TestEphemeralRepetitionData(t *testing.T) {
	t.Parallel()

	tf := e2e.NewBinary(t, tofuBin, "testdata/ephemeral-repetition")
	buildSimpleProvider(t, "6", tf.WorkDir(), "simple")
	exec := func(
		t *testing.T,
		chdir string,
		expectDestroyError bool,
		expectedPlanOutput []outputCheck,
		expectedApplyOutput []outputCheck,
		expectedDestroyOutput []outputCheck,
	) {
		{ // INIT
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			_, stderr, err := tf.RunCtx(ctx, "-chdir="+chdir, "init", "-plugin-dir=../cache")
			if err != nil {
				t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
			}
		}

		{ // PLAN
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			stdout, stderr, err := tf.RunCtx(ctx, "-chdir="+chdir, "plan")
			combined := fmt.Sprintf("%s\n\n%s", stripAnsi(stdout), stripAnsi(stderr))
			if err == nil {
				t.Errorf("expected to have an error during plan but got nothing. output:\n%s", combined)
			}
			entriesChecker := outputEntriesChecker(expectedApplyOutput)
			entriesChecker.check(t, "plan", combined)
		}
		{ // APPLY
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			stdout, stderr, err := tf.RunCtx(ctx, "-chdir="+chdir, "apply", "-auto-approve")
			combined := fmt.Sprintf("%s\n\n%s", stripAnsi(stdout), stripAnsi(stderr))
			if err == nil {
				t.Errorf("expected to have an error during apply but got nothing. output:\n%s", combined)
			}
			entriesChecker := outputEntriesChecker(expectedApplyOutput)
			entriesChecker.check(t, "apply", combined)
		}
		{ // DESTROY
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			stdout, stderr, err := tf.RunCtx(ctx, "-chdir="+chdir, "destroy", "-auto-approve")
			combined := fmt.Sprintf("%s\n\n%s", stripAnsi(stdout), stripAnsi(stderr))
			if !expectDestroyError && err != nil {
				t.Errorf("expected to have no error during destroy. got %q instead. output:\n%s", err, combined)
			} else if expectDestroyError && err == nil {
				t.Errorf("expected to have an error during destroy. got null instead. output:\n%s", combined)
			}
			entriesChecker := outputEntriesChecker(expectedDestroyOutput)
			entriesChecker.check(t, "destroy", combined)
		}
	}
	cases := map[string]struct {
		chdir                 string
		expectDestroyError    bool
		expectedPlanOutput    []outputCheck
		expectedApplyOutput   []outputCheck
		expectedDestroyOutput []outputCheck
	}{
		"lifecycle.enabled": {
			chdir:              "enabled",
			expectDestroyError: false,
			expectedPlanOutput: []outputCheck{
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Opening...`}, true},
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Open complete after`}, true},
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Closing...`}, true},
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Close complete after`}, true},
				outputCheckContains{[]string{"Error: Invalid enabled argument"}, true},
				outputCheckContains{[]string{`on main.tf line 20, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 27, in resource "simple_resource" "res"`}, true},
			},
			expectedApplyOutput: []outputCheck{
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Opening...`}, true},
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Open complete after`}, true},
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Closing...`}, true},
				outputCheckContains{[]string{`ephemeral.simple_resource.res: Close complete after`}, true},
				outputCheckContains{[]string{"Error: Invalid enabled argument"}, true},
				outputCheckContains{[]string{`on main.tf line 20, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 27, in resource "simple_resource" "res"`}, true},
			},
			expectedDestroyOutput: []outputCheck{
				outputCheckContains{[]string{`No changes. No objects need to be destroyed.`}, true},
			},
		},
		"count": {
			chdir:              "count",
			expectDestroyError: true,
			expectedPlanOutput: []outputCheck{
				outputCheckContains{[]string{"Error: Invalid count argument"}, true},
				outputCheckContains{[]string{`on main.tf line 18, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 23, in resource "simple_resource" "res"`}, true},
				outputCheckDoesNotContain{`ephemeral "simple_resource" "res"`, true},
				outputCheckDoesNotContain{`ephemeral.simple_resource.res: Opening...`, true},
			},
			expectedApplyOutput: []outputCheck{
				outputCheckContains{[]string{"Error: Invalid count argument"}, true},
				outputCheckContains{[]string{`on main.tf line 18, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 23, in resource "simple_resource" "res"`}, true},
				outputCheckDoesNotContain{`ephemeral "simple_resource" "res"`, true},
				outputCheckDoesNotContain{`ephemeral.simple_resource.res: Opening...`, true},
			},
			expectedDestroyOutput: []outputCheck{
				outputCheckContains{[]string{"Error: Invalid count argument"}, true},
				outputCheckContains{[]string{`on main.tf line 18, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 23, in resource "simple_resource" "res"`}, true},
				outputCheckDoesNotContain{`ephemeral "simple_resource" "res"`, true},
				outputCheckDoesNotContain{`ephemeral.simple_resource.res: Opening...`, true},
			},
		},
		"for_each": {
			chdir:              "for_each",
			expectDestroyError: true,
			expectedPlanOutput: []outputCheck{
				outputCheckContains{[]string{"Error: Invalid for_each argument"}, true},
				outputCheckContains{[]string{`on main.tf line 19, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 24, in resource "simple_resource" "res"`}, true},
				outputCheckDoesNotContain{`ephemeral "simple_resource" "res"`, true},
				outputCheckDoesNotContain{`ephemeral.simple_resource.res: Opening...`, true},
			},
			expectedApplyOutput: []outputCheck{
				outputCheckContains{[]string{"Error: Invalid for_each argument"}, true},
				outputCheckContains{[]string{`on main.tf line 19, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 24, in resource "simple_resource" "res"`}, true},
				outputCheckDoesNotContain{`ephemeral "simple_resource" "res"`, true},
				outputCheckDoesNotContain{`ephemeral.simple_resource.res: Opening...`, true},
			},
			expectedDestroyOutput: []outputCheck{
				outputCheckContains{[]string{"Error: Invalid for_each argument"}, true},
				outputCheckContains{[]string{`on main.tf line 19, in data "simple_resource" "res"`}, true},
				outputCheckContains{[]string{`on main.tf line 24, in resource "simple_resource" "res"`}, true},
				outputCheckDoesNotContain{`ephemeral "simple_resource" "res"`, true},
				outputCheckDoesNotContain{`ephemeral.simple_resource.res: Opening...`, true},
			},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			exec(
				t,
				tt.chdir,
				tt.expectDestroyError,
				tt.expectedPlanOutput,
				tt.expectedApplyOutput,
				tt.expectedDestroyOutput,
			)
		})
	}

}

// This function builds and moves to a directory called "cache" inside the workdir,
// the version of the provider passed as argument.
// Instead of using this function directly, the pre-configured functions buildV5TestProvider and
// buildV6TestProvider can be used.
func buildSimpleProvider(t *testing.T, version string, workdir string, buildOutName string) {
	if !canRunGoBuild {
		// We're running in a separate-build-then-run context, so we can't
		// currently execute this test which depends on being able to build
		// new executable at runtime.
		//
		// (See the comment on canRunGoBuild's declaration for more information.)
		t.Skip("can't run without building a new provider executable")
	}

	var (
		providerBinFileName string
		implPkgName         string
	)
	switch version {
	case "5":
		providerBinFileName = "simple"
		implPkgName = "provider-simple"
	case "6":
		providerBinFileName = "simple6"
		implPkgName = "provider-simple-v6"
	default:
		t.Fatalf("invalid version for simple provider")
	}
	if buildOutName != "" {
		providerBinFileName = buildOutName
	}
	providerBuildOutDir := filepath.Join(workdir, fmt.Sprintf("terraform-provider-%s", providerBinFileName))
	providerTmpBinPath := e2e.GoBuild(fmt.Sprintf("github.com/opentofu/opentofu/internal/%s/main", implPkgName), providerBuildOutDir)

	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"
	}

	// Move the provider binaries into a directory that we will point tofu
	// to using the -plugin-dir cli flag.
	platform := getproviders.CurrentPlatform.String()
	hashiDir := "cache/registry.opentofu.org/hashicorp/"
	providerCacheDir := filepath.Join(workdir, hashiDir, fmt.Sprintf("%s/0.0.1/", providerBinFileName), platform)
	if err := os.MkdirAll(providerCacheDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	providerFinalBinaryFilePath := filepath.Join(workdir, hashiDir, fmt.Sprintf("%s/0.0.1/", providerBinFileName), platform, fmt.Sprintf("terraform-provider-%s", providerBinFileName)) + extension
	if err := os.Rename(providerTmpBinPath, providerFinalBinaryFilePath); err != nil {
		t.Fatal(err)
	}
}

type outputCheckContains struct {
	variants []string
	strict   bool
}

func (oe outputCheckContains) check(t *testing.T, hint, in string) {
	for _, v := range oe.variants {
		if strings.Contains(in, v) {
			return
		}
	}
	if oe.strict {
		t.Errorf("[%s] output does not contain required content %s\nout:%s", hint, oe.String(), in)
	} else {
		// We don't want to fail the test for outputs that are performance and time dependent
		// as the renew status updates
		t.Logf("[%s] output does not contain %s\nout:%s", hint, oe.String(), in)
	}
}

func (oe outputCheckContains) String() string {
	return `"` + strings.Join(oe.variants, `" OR "`) + `"`
}

type outputCheckDoesNotContain struct {
	token  string
	strict bool
}

func (oe outputCheckDoesNotContain) check(t *testing.T, hint, in string) {
	if !strings.Contains(in, oe.token) {
		return
	}
	if oe.strict {
		t.Errorf("[%s] output contains %q but it shouldn't\nout:%s", hint, oe.token, in)
	} else {
		t.Logf("[%s] output contains %q but it shouldn't\nout:%s", hint, oe.token, in)
	}
}

type outputCheck interface {
	check(t *testing.T, hint, target string)
}

type outputEntriesChecker []outputCheck

func (oec outputEntriesChecker) check(t *testing.T, phase string, contentToCheckIn string) {
	for _, entry := range oec {
		entry.check(t, phase, contentToCheckIn)
	}
}

func mustResourceInstanceAddr(t *testing.T, s string) addrs.AbsResourceInstance {
	addr, diags := addrs.ParseAbsResourceInstanceStr(s)
	if diags.HasErrors() {
		t.Fatalf("failed to parse resource address %q: %s", s, diags.Err())
	}
	return addr
}
