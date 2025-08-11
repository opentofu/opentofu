// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/zclconf/go-cty/cty"
	ctymsgpack "github.com/zclconf/go-cty/cty/msgpack"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestLocalRun(t *testing.T) {
	configDir := "./testdata/empty"
	b := TestLocal(t)

	_, configLoader := initwd.MustLoadConfigForTests(t, configDir, "tests")

	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	stateLocker := clistate.NewLocker(0, views.NewStateLocker(arguments.ViewHuman, view))

	op := &backend.Operation{
		ConfigDir:    configDir,
		ConfigLoader: configLoader,
		Workspace:    backend.DefaultStateName,
		StateLocker:  stateLocker,
	}

	_, _, diags := b.LocalRun(context.Background(), op)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %s", diags.Err().Error())
	}

	// LocalRun() retains a lock on success
	assertBackendStateLocked(t, b)
}

func TestLocalRun_error(t *testing.T) {
	configDir := "./testdata/invalid"
	b := TestLocal(t)

	// This backend will return an error when asked to RefreshState, which
	// should then cause LocalRun to return with the state unlocked.
	b.Backend = backendWithStateStorageThatFailsRefresh{}

	_, configLoader := initwd.MustLoadConfigForTests(t, configDir, "tests")

	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	stateLocker := clistate.NewLocker(0, views.NewStateLocker(arguments.ViewHuman, view))

	op := &backend.Operation{
		ConfigDir:    configDir,
		ConfigLoader: configLoader,
		Workspace:    backend.DefaultStateName,
		StateLocker:  stateLocker,
	}

	_, _, diags := b.LocalRun(context.Background(), op)
	if !diags.HasErrors() {
		t.Fatal("unexpected success")
	}

	// LocalRun() unlocks the state on failure
	assertBackendStateUnlocked(t, b)
}

func TestLocalRun_cloudPlan(t *testing.T) {
	configDir := "./testdata/apply"
	b := TestLocal(t)

	_, configLoader := initwd.MustLoadConfigForTests(t, configDir, "tests")

	planPath := "./testdata/plan-bookmark/bookmark.json"

	planFile, err := planfile.OpenWrapped(planPath, encryption.PlanEncryptionDisabled())
	if err != nil {
		t.Fatalf("unexpected error reading planfile: %s", err)
	}

	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	stateLocker := clistate.NewLocker(0, views.NewStateLocker(arguments.ViewHuman, view))

	op := &backend.Operation{
		ConfigDir:    configDir,
		ConfigLoader: configLoader,
		PlanFile:     planFile,
		Workspace:    backend.DefaultStateName,
		StateLocker:  stateLocker,
	}

	_, _, diags := b.LocalRun(context.Background(), op)
	if !diags.HasErrors() {
		t.Fatal("unexpected success")
	}

	// LocalRun() unlocks the state on failure
	assertBackendStateUnlocked(t, b)
}

func TestLocalRun_stalePlan(t *testing.T) {
	configDir := "./testdata/apply"
	b := TestLocal(t)

	_, configLoader := initwd.MustLoadConfigForTests(t, configDir, "tests")

	// Write an empty state file with serial 3
	sf, err := os.Create(b.StatePath)
	if err != nil {
		t.Fatalf("unexpected error creating state file %s: %s", b.StatePath, err)
	}
	if err := statefile.Write(statefile.New(states.NewState(), "boop", 3), sf, encryption.StateEncryptionDisabled()); err != nil {
		t.Fatalf("unexpected error writing state file: %s", err)
	}

	// Refresh the state
	sm, err := b.StateMgr(t.Context(), "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := sm.RefreshState(t.Context()); err != nil {
		t.Fatalf("unexpected error refreshing state: %s", err)
	}

	// Create a minimal plan which also has state file serial 2, so is stale
	backendConfig := cty.ObjectVal(map[string]cty.Value{
		"path":          cty.NullVal(cty.String),
		"workspace_dir": cty.NullVal(cty.String),
	})
	backendConfigRaw, err := plans.NewDynamicValue(backendConfig, backendConfig.Type())
	if err != nil {
		t.Fatal(err)
	}
	plan := &plans.Plan{
		UIMode:  plans.NormalMode,
		Changes: plans.NewChanges(),
		Backend: plans.Backend{
			Type:   "local",
			Config: backendConfigRaw,
		},
		PrevRunState: states.NewState(),
		PriorState:   states.NewState(),
	}
	prevStateFile := statefile.New(plan.PrevRunState, "boop", 1)
	stateFile := statefile.New(plan.PriorState, "boop", 2)

	// Roundtrip through serialization as expected by the operation
	outDir := t.TempDir()
	defer os.RemoveAll(outDir)
	planPath := filepath.Join(outDir, "plan.tfplan")
	planfileArgs := planfile.CreateArgs{
		ConfigSnapshot:       configload.NewEmptySnapshot(),
		PreviousRunStateFile: prevStateFile,
		StateFile:            stateFile,
		Plan:                 plan,
	}
	if err := planfile.Create(planPath, planfileArgs, encryption.PlanEncryptionDisabled()); err != nil {
		t.Fatalf("unexpected error writing planfile: %s", err)
	}
	planFile, err := planfile.OpenWrapped(planPath, encryption.PlanEncryptionDisabled())
	if err != nil {
		t.Fatalf("unexpected error reading planfile: %s", err)
	}

	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	stateLocker := clistate.NewLocker(0, views.NewStateLocker(arguments.ViewHuman, view))

	op := &backend.Operation{
		ConfigDir:    configDir,
		ConfigLoader: configLoader,
		PlanFile:     planFile,
		Workspace:    backend.DefaultStateName,
		StateLocker:  stateLocker,
	}

	_, _, diags := b.LocalRun(context.Background(), op)
	if !diags.HasErrors() {
		t.Fatal("unexpected success")
	}

	// LocalRun() unlocks the state on failure
	assertBackendStateUnlocked(t, b)
}

func TestLocalRun_ephemeralVariablesLoadedCorrectlyIntoThePlan(t *testing.T) {
	configDir := "./testdata/apply-with-vars"
	b := TestLocal(t)

	_, configLoader, snap := initwd.MustLoadConfigWithSnapshot(t, configDir, "tests")

	var (
		backendConfigRaw plans.DynamicValue
		planPath         string
	)
	{ // create the state and backend config
		sf, err := os.Create(b.StatePath)
		if err != nil {
			t.Fatalf("unexpected error creating state file %s: %s", b.StatePath, err)
		}
		if err := statefile.Write(statefile.New(states.NewState(), "boop", 3), sf, encryption.StateEncryptionDisabled()); err != nil {
			t.Fatalf("unexpected error writing state file: %s", err)
		}

		// Refresh the state
		sm, err := b.StateMgr(t.Context(), "")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := sm.RefreshState(t.Context()); err != nil {
			t.Fatalf("unexpected error refreshing state: %s", err)
		}

		backendConfig := cty.ObjectVal(map[string]cty.Value{
			"path":          cty.NullVal(cty.String),
			"workspace_dir": cty.NullVal(cty.String),
		})
		backendConfigRaw, err = plans.NewDynamicValue(backendConfig, backendConfig.Type())
		if err != nil {
			t.Fatal(err)
		}
	}

	{ // create the plan
		plan := &plans.Plan{
			UIMode:  plans.NormalMode,
			Changes: plans.NewChanges(),
			Backend: plans.Backend{
				Type:   "local",
				Config: backendConfigRaw,
			},
			PrevRunState: states.NewState(),
			PriorState:   states.NewState(),
			VariableValues: map[string]plans.DynamicValue{
				"regular_var": encodeDynamicValueWithType(t, cty.StringVal("regular_var value"), cty.DynamicPseudoType),
			},
			EphemeralVariables: map[string]bool{"regular_var": false, "ephemeral_var": true},
		}

		outDir := t.TempDir()
		defer os.RemoveAll(outDir)
		planPath = filepath.Join(outDir, "plan.tfplan")
		planfileArgs := planfile.CreateArgs{
			ConfigSnapshot:       snap,
			PreviousRunStateFile: statefile.New(plan.PrevRunState, "boop", 1),
			StateFile:            statefile.New(plan.PriorState, "boop", 3),
			Plan:                 plan,
			DependencyLocks:      depsfile.NewLocks(),
		}
		if err := planfile.Create(planPath, planfileArgs, encryption.PlanEncryptionDisabled()); err != nil {
			t.Fatalf("unexpected error writing planfile: %s", err)
		}
	}
	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	stateLocker := clistate.NewLocker(0, views.NewStateLocker(arguments.ViewHuman, view))

	planFile, err := planfile.OpenWrapped(planPath, encryption.PlanEncryptionDisabled())
	if err != nil {
		t.Fatalf("unexpected error reading planfile: %s", err)
	}

	cases := map[string]struct {
		rootModuleCall configs.StaticModuleCall
		givenVars      map[string]backend.UnparsedVariableValue

		expectedDiags tfdiags.Diagnostics
	}{
		"ephemeral_var given again in during apply and injected into the plan": {
			rootModuleCall: configs.NewStaticModuleCall(addrs.RootModule, func(variable *configs.Variable) (cty.Value, hcl.Diagnostics) {
				switch variable.Name {
				case "ephemeral_var":
					return cty.StringVal("ephemeral_var value"), nil
				case "regular_var":
					return cty.StringVal("regular_var value"), nil
				}
				return cty.UnknownVal(cty.DynamicPseudoType), hcl.Diagnostics{}.Append(
					&hcl.Diagnostic{Summary: fmt.Sprintf("no variable value defined for %s", variable.Name)},
				)
			}, "", ""),
			givenVars: map[string]backend.UnparsedVariableValue{
				"ephemeral_var": unparsedInteractiveVariableValue{
					Name:     "ephemeral_var",
					RawValue: "ephemeral_var value",
				},
			},
			expectedDiags: nil,
		},
		"ephemeral_var not given in the apply command": {
			rootModuleCall: configs.NewStaticModuleCall(addrs.RootModule, func(variable *configs.Variable) (cty.Value, hcl.Diagnostics) {
				switch variable.Name {
				case "ephemeral_var":
					return cty.StringVal("ephemeral_var value"), nil
				case "regular_var":
					return cty.StringVal("regular_var value"), nil
				}
				return cty.UnknownVal(cty.DynamicPseudoType), hcl.Diagnostics{}.Append(
					&hcl.Diagnostic{Summary: fmt.Sprintf("no variable value defined for %s", variable.Name)},
				)
			}, "", ""),
			givenVars: map[string]backend.UnparsedVariableValue{
				// This being empty indicates that the variables have not been given into the args
			},
			expectedDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "No value for required variable",
				Detail:   fmt.Sprintf("Variable %q is configured as ephemeral. This type of variables need to be given a value during `tofu plan` and also during `tofu apply`.", "ephemeral_var"),
			}),
		},
		"regular_var given a different value in the apply compared with the one from plan": {
			rootModuleCall: configs.NewStaticModuleCall(addrs.RootModule, func(variable *configs.Variable) (cty.Value, hcl.Diagnostics) {
				switch variable.Name {
				case "ephemeral_var":
					return cty.StringVal("ephemeral_var value"), nil
				case "regular_var":
					return cty.StringVal("different value"), nil
				}
				return cty.UnknownVal(cty.DynamicPseudoType), hcl.Diagnostics{}.Append(
					&hcl.Diagnostic{Summary: fmt.Sprintf("no variable value defined for %s", variable.Name)},
				)
			}, "", ""),
			givenVars: map[string]backend.UnparsedVariableValue{
				"ephemeral_var": unparsedInteractiveVariableValue{
					Name:     "ephemeral_var",
					RawValue: "ephemeral_var value",
				},
				"regular_var": unparsedInteractiveVariableValue{
					Name:     "regular_var",
					RawValue: "different value",
				},
			},
			expectedDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Mismatch between input and plan variable value",
				Detail:   fmt.Sprintf(`Value saved in the plan file for variable %q is different from the one given to the current command.`, "regular_var"),
			}),
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			op := &backend.Operation{
				ConfigDir:       configDir,
				ConfigLoader:    configLoader,
				PlanFile:        planFile,
				Workspace:       backend.DefaultStateName,
				StateLocker:     stateLocker,
				RootCall:        tt.rootModuleCall,
				DependencyLocks: depsfile.NewLocks(),
				Variables:       tt.givenVars,
			}
			// always unlock state after test done
			defer func() {
				_ = op.StateLocker.Unlock()
			}()

			_, _, diags := b.LocalRun(context.Background(), op)
			if got, want := len(diags), len(tt.expectedDiags); got != want {
				t.Fatalf("expected to have %d diags but got %d", want, got)
			}
			for i, gotDiag := range diags {
				wantDiag := tt.expectedDiags[i]
				if diff := cmp.Diff(wantDiag.Description(), gotDiag.Description()); diff != "" {
					t.Errorf("different description of one of the diags.\ndiff:\n%s", diff)
				}
				if want, got := wantDiag.Severity(), gotDiag.Severity(); want != got {
					t.Errorf("different severity of one of the diags. want: %s; got %s", want, got)
				}
			}

			// LocalRun() unlocks the state on failure
			if len(diags) > 0 {
				assertBackendStateUnlocked(t, b)
			} else {
				assertBackendStateLocked(t, b)
			}
		})
	}
}

type backendWithStateStorageThatFailsRefresh struct {
}

var _ backend.Backend = backendWithStateStorageThatFailsRefresh{}

func (b backendWithStateStorageThatFailsRefresh) StateMgr(_ context.Context, workspace string) (statemgr.Full, error) {
	return &stateStorageThatFailsRefresh{}, nil
}

func (b backendWithStateStorageThatFailsRefresh) ConfigSchema() *configschema.Block {
	return &configschema.Block{}
}

func (b backendWithStateStorageThatFailsRefresh) PrepareConfig(in cty.Value) (cty.Value, tfdiags.Diagnostics) {
	return in, nil
}

func (b backendWithStateStorageThatFailsRefresh) Configure(context.Context, cty.Value) tfdiags.Diagnostics {
	return nil
}

func (b backendWithStateStorageThatFailsRefresh) DeleteWorkspace(_ context.Context, name string, force bool) error {
	return fmt.Errorf("unimplemented")
}

func (b backendWithStateStorageThatFailsRefresh) Workspaces(context.Context) ([]string, error) {
	return []string{"default"}, nil
}

type stateStorageThatFailsRefresh struct {
	locked bool
}

var _ statemgr.Full = (*stateStorageThatFailsRefresh)(nil)

func (s *stateStorageThatFailsRefresh) Lock(_ context.Context, info *statemgr.LockInfo) (string, error) {
	if s.locked {
		return "", fmt.Errorf("already locked")
	}
	s.locked = true
	return "locked", nil
}

func (s *stateStorageThatFailsRefresh) Unlock(_ context.Context, id string) error {
	if !s.locked {
		return fmt.Errorf("not locked")
	}
	s.locked = false
	return nil
}

func (s *stateStorageThatFailsRefresh) State() *states.State {
	return nil
}

func (s *stateStorageThatFailsRefresh) GetRootOutputValues(_ context.Context) (map[string]*states.OutputValue, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (s *stateStorageThatFailsRefresh) WriteState(*states.State) error {
	return fmt.Errorf("unimplemented")
}

func (s *stateStorageThatFailsRefresh) MutateState(fn func(*states.State) *states.State) error {
	return fmt.Errorf("unimplemented")
}

func (s *stateStorageThatFailsRefresh) RefreshState(_ context.Context) error {
	return fmt.Errorf("intentionally failing for testing purposes")
}

func (s *stateStorageThatFailsRefresh) PersistState(_ context.Context, schemas *tofu.Schemas) error {
	return fmt.Errorf("unimplemented")
}

func encodeDynamicValueWithType(t *testing.T, value cty.Value, ty cty.Type) []byte {
	data, err := ctymsgpack.Marshal(value, ty)
	if err != nil {
		t.Fatalf("failed to marshal cty msgpack value: %s", err)
	}
	return data
}
