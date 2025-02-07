// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfversion "github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"
)

var (
	equateEmpty   = cmpopts.EquateEmpty()
	typeComparer  = cmp.Comparer(cty.Type.Equals)
	valueComparer = cmp.Comparer(cty.Value.RawEquals)
	valueTrans    = cmp.Transformer("hcl2shim", hcl2shim.ConfigValueFromHCL2)
)

func TestNewContextRequiredVersion(t *testing.T) {
	cases := []struct {
		Name    string
		Version string
		Value   string
		Err     bool
	}{
		{
			"no requirement",
			"0.1.0",
			"",
			false,
		},

		{
			"doesn't match",
			"0.1.0",
			"> 0.6.0",
			true,
		},

		{
			"matches",
			"0.7.0",
			"> 0.6.0",
			false,
		},

		{
			"prerelease doesn't match with inequality",
			"0.8.0",
			"> 0.7.0-beta",
			true,
		},

		{
			"prerelease doesn't match with equality",
			"0.7.0",
			"0.7.0-beta",
			true,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			// Reset the version for the tests
			old := tfversion.SemVer
			tfversion.SemVer = version.Must(version.NewVersion(tc.Version))
			defer func() { tfversion.SemVer = old }()

			mod := testModule(t, "context-required-version")
			if tc.Value != "" {
				constraint, err := version.NewConstraint(tc.Value)
				if err != nil {
					t.Fatalf("can't parse %q as version constraint", tc.Value)
				}
				mod.Module.CoreVersionConstraints = append(mod.Module.CoreVersionConstraints, configs.VersionConstraint{
					Required: constraint,
				})
			}
			c, diags := NewContext(&ContextOpts{})
			if diags.HasErrors() {
				t.Fatalf("unexpected NewContext errors: %s", diags.Err())
			}

			diags = c.Validate(context.Background(), mod)
			if diags.HasErrors() != tc.Err {
				t.Fatalf("err: %s", diags.Err())
			}
		})
	}
}

func TestNewContextRequiredVersion_child(t *testing.T) {
	mod := testModuleInline(t, map[string]string{
		"main.tf": `
module "child" {
  source = "./child"
}
`,
		"child/main.tf": `
terraform {}
`,
	})

	cases := map[string]struct {
		Version    string
		Constraint string
		Err        bool
	}{
		"matches": {
			"0.5.0",
			">= 0.5.0",
			false,
		},
		"doesn't match": {
			"0.4.0",
			">= 0.5.0",
			true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Reset the version for the tests
			old := tfversion.SemVer
			tfversion.SemVer = version.Must(version.NewVersion(tc.Version))
			defer func() { tfversion.SemVer = old }()

			if tc.Constraint != "" {
				constraint, err := version.NewConstraint(tc.Constraint)
				if err != nil {
					t.Fatalf("can't parse %q as version constraint", tc.Constraint)
				}
				child := mod.Children["child"]
				child.Module.CoreVersionConstraints = append(child.Module.CoreVersionConstraints, configs.VersionConstraint{
					Required: constraint,
				})
			}
			c, diags := NewContext(&ContextOpts{})
			if diags.HasErrors() {
				t.Fatalf("unexpected NewContext errors: %s", diags.Err())
			}

			diags = c.Validate(context.Background(), mod)
			if diags.HasErrors() != tc.Err {
				t.Fatalf("err: %s", diags.Err())
			}
		})
	}
}

func TestContext_missingPlugins(t *testing.T) {
	ctx, diags := NewContext(&ContextOpts{})
	assertNoDiagnostics(t, diags)

	configSrc := `
terraform {
	required_providers {
		explicit = {
			source = "example.com/foo/beep"
		}
		builtin = {
			source = "terraform.io/builtin/nonexist"
		}
	}
}

resource "implicit_thing" "a" {
	provisioner "nonexist" {
	}
}

resource "implicit_thing" "b" {
	provider = implicit2
}
`

	cfg := testModuleInline(t, map[string]string{
		"main.tf": configSrc,
	})

	// Validate and Plan are the two entry points where we explicitly verify
	// the available plugins match what the configuration needs. For other
	// operations we typically fail more deeply in OpenTofu Core, with
	// potentially-less-helpful error messages, because getting there would
	// require doing some pretty weird things that aren't common enough to
	// be worth the complexity to check for them.

	validateDiags := ctx.Validate(context.Background(), cfg)
	_, planDiags := ctx.Plan(context.Background(), cfg, nil, DefaultPlanOpts)

	tests := map[string]tfdiags.Diagnostics{
		"validate": validateDiags,
		"plan":     planDiags,
	}

	for testName, gotDiags := range tests {
		t.Run(testName, func(t *testing.T) {
			var wantDiags tfdiags.Diagnostics
			wantDiags = wantDiags.Append(
				tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provider",
					"This configuration requires built-in provider terraform.io/builtin/nonexist, but that provider isn't available in this OpenTofu version.",
				),
				tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provider",
					"This configuration requires provider example.com/foo/beep, but that provider isn't available. You may be able to install it automatically by running:\n  tofu init",
				),
				tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provider",
					"This configuration requires provider registry.opentofu.org/hashicorp/implicit, but that provider isn't available. You may be able to install it automatically by running:\n  tofu init",
				),
				tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provider",
					"This configuration requires provider registry.opentofu.org/hashicorp/implicit2, but that provider isn't available. You may be able to install it automatically by running:\n  tofu init",
				),
				tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provisioner plugin",
					`This configuration requires provisioner plugin "nonexist", which isn't available. If you're intending to use an external provisioner plugin, you must install it manually into one of the plugin search directories before running OpenTofu.`,
				),
			)
			assertDiagnosticsMatch(t, gotDiags, wantDiags)
		})
	}
}

func testContext2(t testing.TB, opts *ContextOpts) *Context {
	t.Helper()

	ctx, diags := NewContext(opts)
	if diags.HasErrors() {
		t.Fatalf("failed to create test context\n\n%s\n", diags.Err())
	}

	ctx.encryption = encryption.Disabled()

	return ctx
}

func testApplyFn(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
	resp.NewState = req.PlannedState
	if req.PlannedState.IsNull() {
		resp.NewState = cty.NullVal(req.PriorState.Type())
		return
	}

	planned := req.PlannedState.AsValueMap()
	if planned == nil {
		planned = map[string]cty.Value{}
	}

	id, ok := planned["id"]
	if !ok || id.IsNull() || !id.IsKnown() {
		planned["id"] = cty.StringVal("foo")
	}

	// our default schema has a computed "type" attr
	if ty, ok := planned["type"]; ok && !ty.IsNull() {
		planned["type"] = cty.StringVal(req.TypeName)
	}

	if cmp, ok := planned["compute"]; ok && !cmp.IsNull() {
		computed := cmp.AsString()
		if val, ok := planned[computed]; ok && !val.IsKnown() {
			planned[computed] = cty.StringVal("computed_value")
		}
	}

	for k, v := range planned {
		if k == "unknown" {
			// "unknown" should cause an error
			continue
		}

		if !v.IsKnown() {
			switch k {
			case "type":
				planned[k] = cty.StringVal(req.TypeName)
			default:
				planned[k] = cty.NullVal(v.Type())
			}
		}
	}

	resp.NewState = cty.ObjectVal(planned)
	return
}

func testDiffFn(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
	var planned map[string]cty.Value

	// this is a destroy plan
	if req.ProposedNewState.IsNull() {
		resp.PlannedState = req.ProposedNewState
		resp.PlannedPrivate = req.PriorPrivate
		return resp
	}

	if !req.ProposedNewState.IsNull() {
		planned = req.ProposedNewState.AsValueMap()
	}
	if planned == nil {
		planned = map[string]cty.Value{}
	}

	// id is always computed for the tests
	if id, ok := planned["id"]; ok && id.IsNull() {
		planned["id"] = cty.UnknownVal(cty.String)
	}

	// the old tests have require_new replace on every plan
	if _, ok := planned["require_new"]; ok {
		resp.RequiresReplace = append(resp.RequiresReplace, cty.Path{cty.GetAttrStep{Name: "require_new"}})
	}

	for k := range planned {
		requiresNewKey := "__" + k + "_requires_new"
		_, ok := planned[requiresNewKey]
		if ok {
			resp.RequiresReplace = append(resp.RequiresReplace, cty.Path{cty.GetAttrStep{Name: requiresNewKey}})
		}
	}

	if v, ok := planned["compute"]; ok && !v.IsNull() {
		k := v.AsString()
		unknown := cty.UnknownVal(cty.String)
		if strings.HasSuffix(k, ".#") {
			k = k[:len(k)-2]
			unknown = cty.UnknownVal(cty.List(cty.String))
		}
		planned[k] = unknown
	}

	if t, ok := planned["type"]; ok && t.IsNull() {
		planned["type"] = cty.UnknownVal(cty.String)
	}

	resp.PlannedState = cty.ObjectVal(planned)
	return
}

func testProvider(prefix string) *MockProvider {
	p := new(MockProvider)
	p.GetProviderSchemaResponse = testProviderSchema(prefix)

	return p
}

func testProvisioner() *MockProvisioner {
	p := new(MockProvisioner)
	p.GetSchemaResponse = provisioners.GetSchemaResponse{
		Provisioner: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"command": {
					Type:     cty.String,
					Optional: true,
				},
				"order": {
					Type:     cty.String,
					Optional: true,
				},
				"when": {
					Type:     cty.String,
					Optional: true,
				},
			},
		},
	}
	return p
}

func checkStateString(t *testing.T, state *states.State, expected string) {
	t.Helper()
	actual := strings.TrimSpace(state.String())
	expected = strings.TrimSpace(expected)

	if actual != expected {
		t.Fatalf("incorrect state\ngot:\n%s\n\nwant:\n%s", actual, expected)
	}
}

// Test helper that gives a function 3 seconds to finish, assumes deadlock and
// fails test if it does not.
func testCheckDeadlock(t *testing.T, f func()) {
	t.Helper()
	timeout := make(chan bool, 1)
	done := make(chan bool, 1)
	go func() {
		time.Sleep(3 * time.Second)
		timeout <- true
	}()
	go func(f func(), done chan bool) {
		defer func() { done <- true }()
		f()
	}(f, done)
	select {
	case <-timeout:
		t.Fatalf("timed out! probably deadlock")
	case <-done:
		// ok
	}
}

func testProviderSchema(name string) *providers.GetProviderSchemaResponse {
	return getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"region": {
					Type:     cty.String,
					Optional: true,
				},
				"foo": {
					Type:     cty.String,
					Optional: true,
				},
				"value": {
					Type:     cty.String,
					Optional: true,
				},
				"root": {
					Type:     cty.Number,
					Optional: true,
				},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			name + "_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"ami": {
						Type:     cty.String,
						Optional: true,
					},
					"dep": {
						Type:     cty.String,
						Optional: true,
					},
					"num": {
						Type:     cty.Number,
						Optional: true,
					},
					"require_new": {
						Type:     cty.String,
						Optional: true,
					},
					"var": {
						Type:     cty.String,
						Optional: true,
					},
					"foo": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
					"bar": {
						Type:     cty.String,
						Optional: true,
					},
					"compute": {
						Type:     cty.String,
						Optional: true,
						Computed: false,
					},
					"compute_value": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
					"output": {
						Type:     cty.String,
						Optional: true,
					},
					"write": {
						Type:     cty.String,
						Optional: true,
					},
					"instance": {
						Type:     cty.String,
						Optional: true,
					},
					"vpc_id": {
						Type:     cty.String,
						Optional: true,
					},
					"type": {
						Type:     cty.String,
						Computed: true,
					},

					// Generated by testDiffFn if compute = "unknown" is set in the test config
					"unknown": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
			name + "_eip": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"instance": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			name + "_resource": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
					"sensitive_value": {
						Type:      cty.String,
						Sensitive: true,
						Optional:  true,
					},
					"random": {
						Type:     cty.String,
						Optional: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"nesting_single": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"value":           {Type: cty.String, Optional: true},
								"sensitive_value": {Type: cty.String, Optional: true, Sensitive: true},
							},
						},
						Nesting: configschema.NestingSingle,
					},
				},
			},
			name + "_ami_list": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
					"ids": {
						Type:     cty.List(cty.String),
						Optional: true,
						Computed: true,
					},
				},
			},
			name + "_remote_state": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
					},
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
					"output": {
						Type:     cty.Map(cty.String),
						Computed: true,
					},
				},
			},
			name + "_file": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
					},
					"template": {
						Type:     cty.String,
						Optional: true,
					},
					"rendered": {
						Type:     cty.String,
						Computed: true,
					},
					"__template_requires_new": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			name + "_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"foo": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
				},
			},
			name + "_remote_state": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
					},
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
					"output": {
						Type:     cty.Map(cty.String),
						Optional: true,
					},
				},
			},
			name + "_file": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
					},
					"template": {
						Type:     cty.String,
						Optional: true,
					},
					"rendered": {
						Type:     cty.String,
						Computed: true,
					},
				},
			},
			name + "_sensitive_data_source": {
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:      cty.String,
						Optional:  true,
						Sensitive: true,
					},
				},
			},
		},
	})
}

// contextOptsForPlanViaFile is a helper that creates a temporary plan file,
// then reads it back in again and produces a ContextOpts object containing the
// planned changes, prior state and config from the plan file.
//
// This is intended for testing the separated plan/apply workflow in a more
// convenient way than spelling out all of these steps every time. Normally
// only the command and backend packages need to deal with such things, but
// our context tests try to exercise lots of stuff at once and so having them
// round-trip things through on-disk files is often an important part of
// fully representing an old bug in a regression test.
func contextOptsForPlanViaFile(t *testing.T, configSnap *configload.Snapshot, plan *plans.Plan) (*ContextOpts, *configs.Config, *plans.Plan, error) {
	dir := t.TempDir()

	// We'll just create a dummy statefile.File here because we're not going
	// to run through any of the codepaths that care about Lineage/Serial/etc
	// here anyway.
	stateFile := &statefile.File{
		State: plan.PriorState,
	}
	prevStateFile := &statefile.File{
		State: plan.PrevRunState,
	}

	// To make life a little easier for test authors, we'll populate a simple
	// backend configuration if they didn't set one, since the backend is
	// usually dealt with in a calling package and so tests in this package
	// don't really care about it.
	if plan.Backend.Config == nil {
		cfg, err := plans.NewDynamicValue(cty.EmptyObjectVal, cty.EmptyObject)
		if err != nil {
			panic(fmt.Sprintf("NewDynamicValue failed: %s", err)) // shouldn't happen because we control the inputs
		}
		plan.Backend.Type = "local"
		plan.Backend.Config = cfg
		plan.Backend.Workspace = "default"
	}

	filename := filepath.Join(dir, "tfplan")
	err := planfile.Create(filename, planfile.CreateArgs{
		ConfigSnapshot:       configSnap,
		PreviousRunStateFile: prevStateFile,
		StateFile:            stateFile,
		Plan:                 plan,
	}, encryption.PlanEncryptionDisabled())
	if err != nil {
		return nil, nil, nil, err
	}

	pr, err := planfile.Open(filename, encryption.PlanEncryptionDisabled())
	if err != nil {
		return nil, nil, nil, err
	}

	config, diags := pr.ReadConfig(configs.RootModuleCallForTesting())
	if diags.HasErrors() {
		return nil, nil, nil, diags.Err()
	}

	plan, err = pr.ReadPlan()
	if err != nil {
		return nil, nil, nil, err
	}

	// Note: This has grown rather silly over the course of ongoing refactoring,
	// because ContextOpts is no longer actually responsible for carrying
	// any information from a plan file and instead all of the information
	// lives inside the config and plan objects. We continue to return a
	// silly empty ContextOpts here just to keep all of the calling tests
	// working.
	return &ContextOpts{}, config, plan, nil
}

// legacyPlanComparisonString produces a string representation of the changes
// from a plan and a given state together, as was formerly produced by the
// String method of tofu.Plan.
//
// This is here only for compatibility with existing tests that predate our
// new plan and state types, and should not be used in new tests. Instead, use
// a library like "cmp" to do a deep equality check and diff on the two
// data structures.
func legacyPlanComparisonString(state *states.State, changes *plans.Changes) string {
	return fmt.Sprintf(
		"DIFF:\n\n%s\n\nSTATE:\n\n%s",
		legacyDiffComparisonString(changes),
		state.String(),
	)
}

// legacyDiffComparisonString produces a string representation of the changes
// from a planned changes object, as was formerly produced by the String method
// of tofu.Diff.
//
// This is here only for compatibility with existing tests that predate our
// new plan types, and should not be used in new tests. Instead, use a library
// like "cmp" to do a deep equality check and diff on the two data structures.
func legacyDiffComparisonString(changes *plans.Changes) string {
	// The old string representation of a plan was grouped by module, but
	// our new plan structure is not grouped in that way and so we'll need
	// to preprocess it in order to produce that grouping.
	type ResourceChanges struct {
		Current *plans.ResourceInstanceChangeSrc
		Deposed map[states.DeposedKey]*plans.ResourceInstanceChangeSrc
	}
	byModule := map[string]map[string]*ResourceChanges{}
	resourceKeys := map[string][]string{}
	var moduleKeys []string
	for _, rc := range changes.Resources {
		if rc.Action == plans.NoOp {
			// We won't mention no-op changes here at all, since the old plan
			// model we are emulating here didn't have such a concept.
			continue
		}
		moduleKey := rc.Addr.Module.String()
		if _, exists := byModule[moduleKey]; !exists {
			moduleKeys = append(moduleKeys, moduleKey)
			byModule[moduleKey] = make(map[string]*ResourceChanges)
		}
		resourceKey := rc.Addr.Resource.String()
		if _, exists := byModule[moduleKey][resourceKey]; !exists {
			resourceKeys[moduleKey] = append(resourceKeys[moduleKey], resourceKey)
			byModule[moduleKey][resourceKey] = &ResourceChanges{
				Deposed: make(map[states.DeposedKey]*plans.ResourceInstanceChangeSrc),
			}
		}

		if rc.DeposedKey == states.NotDeposed {
			byModule[moduleKey][resourceKey].Current = rc
		} else {
			byModule[moduleKey][resourceKey].Deposed[rc.DeposedKey] = rc
		}
	}
	sort.Strings(moduleKeys)
	for _, ks := range resourceKeys {
		sort.Strings(ks)
	}

	var buf bytes.Buffer

	for _, moduleKey := range moduleKeys {
		rcs := byModule[moduleKey]
		var mBuf bytes.Buffer

		for _, resourceKey := range resourceKeys[moduleKey] {
			rc := rcs[resourceKey]

			crud := "UPDATE"
			if rc.Current != nil {
				switch rc.Current.Action {
				case plans.DeleteThenCreate:
					crud = "DESTROY/CREATE"
				case plans.CreateThenDelete:
					crud = "CREATE/DESTROY"
				case plans.Delete:
					crud = "DESTROY"
				case plans.Create:
					crud = "CREATE"
				}
			} else {
				// We must be working on a deposed object then, in which
				// case destroying is the only possible action.
				crud = "DESTROY"
			}

			extra := ""
			if rc.Current == nil && len(rc.Deposed) > 0 {
				extra = " (deposed only)"
			}

			fmt.Fprintf(
				&mBuf, "%s: %s%s\n",
				crud, resourceKey, extra,
			)

			attrNames := map[string]bool{}
			var oldAttrs map[string]string
			var newAttrs map[string]string
			if rc.Current != nil {
				if before := rc.Current.Before; before != nil {
					ty, err := before.ImpliedType()
					if err == nil {
						val, err := before.Decode(ty)
						if err == nil {
							oldAttrs = hcl2shim.FlatmapValueFromHCL2(val)
							for k := range oldAttrs {
								attrNames[k] = true
							}
						}
					}
				}
				if after := rc.Current.After; after != nil {
					ty, err := after.ImpliedType()
					if err == nil {
						val, err := after.Decode(ty)
						if err == nil {
							newAttrs = hcl2shim.FlatmapValueFromHCL2(val)
							for k := range newAttrs {
								attrNames[k] = true
							}
						}
					}
				}
			}
			if oldAttrs == nil {
				oldAttrs = make(map[string]string)
			}
			if newAttrs == nil {
				newAttrs = make(map[string]string)
			}

			attrNamesOrder := make([]string, 0, len(attrNames))
			keyLen := 0
			for n := range attrNames {
				attrNamesOrder = append(attrNamesOrder, n)
				if len(n) > keyLen {
					keyLen = len(n)
				}
			}
			sort.Strings(attrNamesOrder)

			for _, attrK := range attrNamesOrder {
				v := newAttrs[attrK]
				u := oldAttrs[attrK]

				if v == hcl2shim.UnknownVariableValue {
					v = "<computed>"
				}
				// NOTE: we don't support <sensitive> here because we would
				// need schema to do that. Excluding sensitive values
				// is now done at the UI layer, and so should not be tested
				// at the core layer.

				updateMsg := ""
				// TODO: Mark " (forces new resource)" in updateMsg when appropriate.

				fmt.Fprintf(
					&mBuf, "  %s:%s %#v => %#v%s\n",
					attrK,
					strings.Repeat(" ", keyLen-len(attrK)),
					u, v,
					updateMsg,
				)
			}
		}

		if moduleKey == "" { // root module
			buf.Write(mBuf.Bytes())
			buf.WriteByte('\n')
			continue
		}

		fmt.Fprintf(&buf, "%s:\n", moduleKey)
		s := bufio.NewScanner(&mBuf)
		for s.Scan() {
			buf.WriteString(fmt.Sprintf("  %s\n", s.Text()))
		}
	}

	return buf.String()
}

// assertNoDiagnostics fails the test in progress (using t.Fatal) if the given
// diagnostics is non-empty.
func assertNoDiagnostics(t testing.TB, diags tfdiags.Diagnostics) {
	t.Helper()
	if len(diags) == 0 {
		return
	}
	logDiagnostics(t, diags)
	t.FailNow()
}

// assertNoDiagnostics fails the test in progress (using t.Fatal) if the given
// diagnostics has any errors.
func assertNoErrors(t testing.TB, diags tfdiags.Diagnostics) {
	t.Helper()
	if !diags.HasErrors() {
		return
	}
	logDiagnostics(t, diags)
	t.FailNow()
}

// assertDiagnosticsMatch fails the test in progress (using t.Fatal) if the
// two sets of diagnostics don't match after being normalized using the
// "ForRPC" processing step, which eliminates the specific type information
// and HCL expression information of each diagnostic.
//
// assertDiagnosticsMatch sorts the two sets of diagnostics in the usual way
// before comparing them, though diagnostics only have a partial order so that
// will not totally normalize the ordering of all diagnostics sets.
func assertDiagnosticsMatch(t testing.TB, got, want tfdiags.Diagnostics) {
	got = got.ForRPC()
	want = want.ForRPC()
	got.Sort()
	want.Sort()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("wrong diagnostics\n%s", diff)
	}
}

// logDiagnostics is a test helper that logs the given diagnostics to to the
// given testing.T using t.Log, in a way that is hopefully useful in debugging
// a test. It does not generate any errors or fail the test. See
// assertNoDiagnostics and assertNoErrors for more specific helpers that can
// also fail the test.
func logDiagnostics(t testing.TB, diags tfdiags.Diagnostics) {
	t.Helper()
	for _, diag := range diags {
		desc := diag.Description()
		rng := diag.Source()

		var severity string
		switch diag.Severity() {
		case tfdiags.Error:
			severity = "ERROR"
		case tfdiags.Warning:
			severity = "WARN"
		default:
			severity = "???" // should never happen
		}

		if subj := rng.Subject; subj != nil {
			if desc.Detail == "" {
				t.Logf("[%s@%s] %s", severity, subj.StartString(), desc.Summary)
			} else {
				t.Logf("[%s@%s] %s: %s", severity, subj.StartString(), desc.Summary, desc.Detail)
			}
		} else {
			if desc.Detail == "" {
				t.Logf("[%s] %s", severity, desc.Summary)
			} else {
				t.Logf("[%s] %s: %s", severity, desc.Summary, desc.Detail)
			}
		}
	}
}

const testContextRefreshModuleStr = `
aws_instance.web: (tainted)
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/aws"]

module.child:
  aws_instance.web:
    ID = new
    provider = provider["registry.opentofu.org/hashicorp/aws"]
`

const testContextRefreshOutputStr = `
aws_instance.web:
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
  foo = bar

Outputs:

foo = bar
`

const testContextRefreshOutputPartialStr = `
<no state>
`

const testContextRefreshTaintedStr = `
aws_instance.web: (tainted)
  ID = foo
  provider = provider["registry.opentofu.org/hashicorp/aws"]
`
