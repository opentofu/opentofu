// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
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

	//// INIT
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

	//// PLAN
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

	//// APPLY
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

	//// DESTROY
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

	//// INIT
	_, stderr, err := tf.Run("-chdir=subdir", "init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	//// PLAN
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

	//// APPLY
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

	//// DESTROY
	stdout, stderr, err = tf.Run("-chdir=subdir", "destroy", "-auto-approve")
	if err != nil {
		t.Fatalf("unexpected destroy error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 0 destroyed") {
		t.Errorf("incorrect destroy tally; want 0 destroyed:\n%s", stdout)
	}
}

// This test is checking the workflow of the ephemeral resources.
// Check also the configuration files for comments. The idea is that at the time of
// writing, the configuration was done in such a way to fail later when the
// marks will be introduced for ephemeral values. Therefore, this test will
// fail later and will require adjustments.
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

	skipIfCannotAccessNetwork(t)
	pluginVersionRunner := func(t *testing.T, testdataPath string, providerBuilderFunc func(*testing.T, string)) {
		tf := e2e.NewBinary(t, tofuBin, testdataPath)
		providerBuilderFunc(t, tf.WorkDir())

		{ //// INIT
			_, stderr, err := tf.Run("init", "-plugin-dir=cache")
			if err != nil {
				t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
			}
		}

		{ //// PLAN
			stdout, stderr, err := tf.Run("plan", "-out=tfplan")
			if err != nil {
				t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
			}
			// TODO ephemeral - this "value_wo" should be shown something like (write-only attribute). This will be handled during the work on the write-only attributes.
			// TODO ephemeral - "out_ephemeral" should fail later when the marking of the outputs is implemented fully, so that should not be visible in the output
			expectedChangesOutput := `OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create
 <= read (data resources)

OpenTofu will perform the following actions:

  # data.simple_resource.test_data2 will be read during apply
  # (depends on a resource or a module with changes pending)
 <= data "simple_resource" "test_data2" {
      + id    = (known after apply)
      + value = "test"
    }

  # simple_resource.test_res will be created
  + resource "simple_resource" "test_res" {
      + value = "test value"
    }

  # simple_resource.test_res_second_provider will be created
  + resource "simple_resource" "test_res_second_provider" {
      + value = "just a simple resource to ensure that the second provider it's working fine"
    }

Plan: 2 to add, 0 to change, 0 to destroy.

Changes to Outputs:
  + final_output  = "just a simple resource to ensure that the second provider it's working fine"
  + out_ephemeral = "rawvalue"`

			entriesChecker := &outputEntriesChecker{phase: "plan"}
			entriesChecker.addChecks(outputEntry{[]string{"data.simple_resource.test_data1: Reading..."}, true},
				outputEntry{[]string{"data.simple_resource.test_data1: Read complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Opening..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Open complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Opening..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Open complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Closing..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Close complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Closing..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Close complete after"}, true},
			)
			out := stripAnsi(stdout)

			if !strings.Contains(out, expectedChangesOutput) {
				t.Errorf("wrong plan output:\nstdout:%s\nstderr:%s", stdout, stderr)
			}
			entriesChecker.check(t, out)

			// assert plan file content
			plan, err := tf.Plan("tfplan")
			if err != nil {
				t.Fatalf("failed to read the plan file: %s", err)
			}
			idx := slices.IndexFunc(plan.Changes.Resources, func(src *plans.ResourceInstanceChangeSrc) bool {
				return src.Addr.Resource.Resource.Mode == addrs.EphemeralResourceMode
			})
			if idx < 0 {
				t.Fatalf("no ephemeral resource found in the plan file")
			}
			res := plan.Changes.Resources[idx]
			if res.Before != nil {
				t.Errorf("ephemeral resource %q from plan contains before value but it shouldn't: %s", res.Addr.String(), res.Before)
			}
			if res.After != nil {
				t.Errorf("ephemeral resource %q from plan contains after value but it shouldn't: %s", res.Addr.String(), res.After)
			}
			if got, want := res.Action, plans.Open; got != want {
				t.Errorf("ephemeral resource %q from plan contains wrong actions. want %q; got %q", res.Addr.String(), want, got)
			}
		}

		{ //// APPLY
			stdout, stderr, err := tf.Run("apply", "tfplan")
			if err != nil {
				t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
			}
			state, err := tf.LocalState()
			if err != nil {
				t.Fatalf("failed to read local state: %s", err)
			}
			expectedResources := map[string]bool{
				"data.simple_resource.test_data1":          true,
				"data.simple_resource.test_data2":          true,
				"simple_resource.test_res":                 true,
				"simple_resource.test_res_second_provider": true,
				"ephemeral.simple_resource.test_ephemeral": false,
			}
			for res, exists := range expectedResources {
				_, ok := state.RootModule().Resources[res]
				if ok != exists {
					t.Errorf("expected resource %q existence to be %t but got %t", res, exists, ok)
				}
			}

			expectedChangesOutput := `Apply complete! Resources: 2 added, 0 changed, 0 destroyed.`
			// NOTE: the non-required ones are dependent on the performance of the platform that this test is running on.
			// In CI, if we would make this as required, this test might be flaky.
			entriesChecker := outputEntriesChecker{phase: "apply"}
			entriesChecker.addChecks(
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Opening..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Open complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Opening..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Open complete after"}, true},
				outputEntry{[]string{"data.simple_resource.test_data2: Reading..."}, true},
				outputEntry{[]string{"data.simple_resource.test_data2: Read complete after"}, true},
				outputEntry{[]string{"simple_resource.test_res: Creating..."}, true},
				outputEntry{[]string{"simple_resource.test_res_second_provider: Creating..."}, true},
				outputEntry{[]string{"simple_resource.test_res_second_provider: Creation complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Renewing..."}, false},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Renew complete after"}, false},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Renewing..."}, false},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Renew complete after"}, false},
				outputEntry{[]string{"simple_resource.test_res: Creation complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Closing..."}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[0]: Close complete after"}, true},
				outputEntry{[]string{"ephemeral.simple_resource.test_ephemeral[1]: Closing..."}, true},
				outputEntry{[]string{"simple_resource.test_res: Provisioning with 'local-exec'..."}, true},
				outputEntry{[]string{
					`simple_resource.test_res (local-exec): Executing: ["/bin/sh" "-c" "echo \"visible test value\""]`,
					`simple_resource.test_res (local-exec): Executing: ["cmd" "/C" "echo \"visible test value\""]`,
				}, true},
				outputEntry{[]string{
					`simple_resource.test_res (local-exec): visible test value`,
					`simple_resource.test_res (local-exec): \"visible test value\"`,
				}, true},
				outputEntry{[]string{"simple_resource.test_res (local-exec): (output suppressed due to ephemeral value in config)"}, true},
			)
			out := stripAnsi(stdout)

			if !strings.Contains(out, expectedChangesOutput) {
				t.Errorf("wrong apply output:\nstdout:%s\nstderr%s", stdout, stderr)
			}
			entriesChecker.check(t, out)
		}
		{ //// DESTROY
			stdout, stderr, err := tf.Run("destroy", "-auto-approve")
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

type outputEntry struct {
	variants []string
	required bool
}

func (oe outputEntry) in(out string) bool {
	for _, v := range oe.variants {
		if strings.Contains(out, v) {
			return true
		}
	}
	return false
}

func (oe outputEntry) String() string {
	return `"` + strings.Join(oe.variants, `" OR "`) + `"`
}

type outputEntriesChecker struct {
	entries []outputEntry
	phase   string
}

func (oec *outputEntriesChecker) addChecks(entries ...outputEntry) {
	oec.entries = append(oec.entries, entries...)
}

func (oec *outputEntriesChecker) check(t *testing.T, contentToCheckIn string) {
	for _, entry := range oec.entries {
		if entry.in(contentToCheckIn) {
			continue
		}
		if entry.required {
			t.Errorf("%s output does not contain required content %s\nout:%s", oec.phase, entry.String(), contentToCheckIn)
		} else {
			// We don't want to fail the test for outputs that are performance and time dependent
			// as the renew status updates
			t.Logf("%s output does not contain %s\nout:%s", oec.phase, entry.String(), contentToCheckIn)
		}
	}
}
