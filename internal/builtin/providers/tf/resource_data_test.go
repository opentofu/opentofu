// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	"bytes"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func TestManagedDataValidate(t *testing.T) {
	cfg := map[string]cty.Value{
		"input":            cty.NullVal(cty.DynamicPseudoType),
		"output":           cty.NullVal(cty.DynamicPseudoType),
		"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
		"id":               cty.NullVal(cty.String),
	}

	// empty
	req := providers.ValidateResourceConfigRequest{
		TypeName: "terraform_data",
		Config:   cty.ObjectVal(cfg),
	}

	resp := validateDataStoreResourceConfig(req)
	if resp.Diagnostics.HasErrors() {
		t.Error("empty config error:", resp.Diagnostics.ErrWithWarnings())
	}

	// invalid computed values
	cfg["output"] = cty.StringVal("oops")
	req.Config = cty.ObjectVal(cfg)

	resp = validateDataStoreResourceConfig(req)
	if !resp.Diagnostics.HasErrors() {
		t.Error("expected error")
	}

	msg := resp.Diagnostics.Err().Error()
	if !strings.Contains(msg, "attribute is read-only") {
		t.Error("unexpected error", msg)
	}
}

func TestManagedDataUpgradeState(t *testing.T) {
	schema := dataStoreResourceSchema()
	ty := schema.Block.ImpliedType()

	state := cty.ObjectVal(map[string]cty.Value{
		"input":  cty.StringVal("input"),
		"output": cty.StringVal("input"),
		"triggers_replace": cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"),
		}),
		"id": cty.StringVal("not-quite-unique"),
	})

	jsState, err := ctyjson.Marshal(state, ty)
	if err != nil {
		t.Fatal(err)
	}

	// empty
	req := providers.UpgradeResourceStateRequest{
		TypeName:     "terraform_data",
		RawStateJSON: jsState,
	}

	resp := upgradeDataStoreResourceState(req)
	if resp.Diagnostics.HasErrors() {
		t.Error("upgrade state error:", resp.Diagnostics.ErrWithWarnings())
	}

	if !resp.UpgradedState.RawEquals(state) {
		t.Errorf("prior state was:\n%#v\nupgraded state is:\n%#v\n", state, resp.UpgradedState)
	}
}

func TestManagedDataMovedState(t *testing.T) {
	nullSchema := nullResourceSchema()
	nullTy := nullSchema.Block.ImpliedType()

	state := cty.ObjectVal(map[string]cty.Value{
		"triggers": cty.MapVal(map[string]cty.Value{
			"examplekey": cty.StringVal("value"),
		}),
		"id": cty.StringVal("not-quite-unique"),
	})

	jsState, err := ctyjson.Marshal(state, nullTy)
	if err != nil {
		t.Fatal(err)
	}

	// empty request should fail
	req := providers.MoveResourceStateRequest{}

	resp := moveDataStoreResourceState(req)
	if !resp.Diagnostics.HasErrors() {
		t.Fatalf("expected error, got %#v", resp)
	}

	// valid request
	req = providers.MoveResourceStateRequest{
		TargetTypeName:  "terraform_data",
		SourceTypeName:  "null_resource",
		SourcePrivate:   []byte("PRIVATE"),
		SourceStateJSON: jsState,
	}

	resp = moveDataStoreResourceState(req)

	expectedState := cty.ObjectVal(map[string]cty.Value{
		"triggers_replace": cty.ObjectVal(map[string]cty.Value{
			"examplekey": cty.StringVal("value"),
		}),
		"id":     cty.StringVal("not-quite-unique"),
		"input":  cty.NullVal(cty.DynamicPseudoType),
		"output": cty.NullVal(cty.DynamicPseudoType),
	})
	if !resp.TargetState.RawEquals(expectedState) {
		t.Errorf("prior state was:\n%#v\nmoved state is:\n%#v\n", expectedState, resp.TargetState)
	}

	if !bytes.Equal(resp.TargetPrivate, req.SourcePrivate) {
		t.Error("expected private data to be copied")
	}

}
func TestManagedDataRead(t *testing.T) {
	req := providers.ReadResourceRequest{
		TypeName: "terraform_data",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"input":  cty.StringVal("input"),
			"output": cty.StringVal("input"),
			"triggers_replace": cty.ListVal([]cty.Value{
				cty.StringVal("a"), cty.StringVal("b"),
			}),
			"id": cty.StringVal("not-quite-unique"),
		}),
	}

	resp := readDataStoreResourceState(req)
	if resp.Diagnostics.HasErrors() {
		t.Fatal("unexpected error", resp.Diagnostics.ErrWithWarnings())
	}

	if !resp.NewState.RawEquals(req.PriorState) {
		t.Errorf("prior state was:\n%#v\nnew state is:\n%#v\n", req.PriorState, resp.NewState)
	}
}

func TestManagedDataPlan(t *testing.T) {
	schema := dataStoreResourceSchema().Block
	ty := schema.ImpliedType()

	for name, tc := range map[string]struct {
		prior    cty.Value
		proposed cty.Value
		planned  cty.Value
	}{
		"create": {
			prior: cty.NullVal(ty),
			proposed: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.NullVal(cty.DynamicPseudoType),
				"output":           cty.NullVal(cty.DynamicPseudoType),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.NullVal(cty.String),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.NullVal(cty.DynamicPseudoType),
				"output":           cty.NullVal(cty.DynamicPseudoType),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.UnknownVal(cty.String).RefineNotNull(),
			}),
		},

		"create-typed-null-input": {
			prior: cty.NullVal(ty),
			proposed: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.NullVal(cty.String),
				"output":           cty.NullVal(cty.DynamicPseudoType),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.NullVal(cty.String),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.NullVal(cty.String),
				"output":           cty.NullVal(cty.String),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.UnknownVal(cty.String).RefineNotNull(),
			}),
		},

		"create-output": {
			prior: cty.NullVal(ty),
			proposed: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.NullVal(cty.DynamicPseudoType),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.NullVal(cty.String),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.UnknownVal(cty.String),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.UnknownVal(cty.String).RefineNotNull(),
			}),
		},

		"update-input": {
			prior: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			proposed: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.UnknownVal(cty.List(cty.String)),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.UnknownVal(cty.List(cty.String)),
				"output":           cty.UnknownVal(cty.List(cty.String)),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
		},

		"update-trigger": {
			prior: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			proposed: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.StringVal("new-value"),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.UnknownVal(cty.String),
				"triggers_replace": cty.StringVal("new-value"),
				"id":               cty.UnknownVal(cty.String).RefineNotNull(),
			}),
		},

		"update-input-trigger": {
			prior: cty.ObjectVal(map[string]cty.Value{
				"input":  cty.StringVal("input"),
				"output": cty.StringVal("input"),
				"triggers_replace": cty.MapVal(map[string]cty.Value{
					"key": cty.StringVal("value"),
				}),
				"id": cty.StringVal("not-quite-unique"),
			}),
			proposed: cty.ObjectVal(map[string]cty.Value{
				"input":  cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"output": cty.StringVal("input"),
				"triggers_replace": cty.MapVal(map[string]cty.Value{
					"key": cty.StringVal("new value"),
				}),
				"id": cty.StringVal("not-quite-unique"),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":  cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"output": cty.UnknownVal(cty.List(cty.String)),
				"triggers_replace": cty.MapVal(map[string]cty.Value{
					"key": cty.StringVal("new value"),
				}),
				"id": cty.UnknownVal(cty.String).RefineNotNull(),
			}),
		},
	} {
		t.Run("plan-"+name, func(t *testing.T) {
			req := providers.PlanResourceChangeRequest{
				TypeName:         "terraform_data",
				PriorState:       tc.prior,
				ProposedNewState: tc.proposed,
			}

			resp := planDataStoreResourceChange(req)
			if resp.Diagnostics.HasErrors() {
				t.Fatal(resp.Diagnostics.ErrWithWarnings())
			}

			if !resp.PlannedState.RawEquals(tc.planned) {
				t.Errorf("expected:\n%#v\ngot:\n%#v\n", tc.planned, resp.PlannedState)
			}
		})
	}
}

func TestManagedDataApply(t *testing.T) {
	testUUIDHook = func() string {
		return "not-quite-unique"
	}
	defer func() {
		testUUIDHook = nil
	}()

	schema := dataStoreResourceSchema().Block
	ty := schema.ImpliedType()

	for name, tc := range map[string]struct {
		prior   cty.Value
		planned cty.Value
		state   cty.Value
	}{
		"create": {
			prior: cty.NullVal(ty),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.NullVal(cty.DynamicPseudoType),
				"output":           cty.NullVal(cty.DynamicPseudoType),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.UnknownVal(cty.String),
			}),
			state: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.NullVal(cty.DynamicPseudoType),
				"output":           cty.NullVal(cty.DynamicPseudoType),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
		},

		"create-output": {
			prior: cty.NullVal(ty),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.UnknownVal(cty.String),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.UnknownVal(cty.String),
			}),
			state: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
		},

		"update-input": {
			prior: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"output":           cty.UnknownVal(cty.List(cty.String)),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			state: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"output":           cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
		},

		"update-trigger": {
			prior: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
				"id":               cty.StringVal("not-quite-unique"),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.UnknownVal(cty.String),
				"triggers_replace": cty.StringVal("new-value"),
				"id":               cty.UnknownVal(cty.String),
			}),
			state: cty.ObjectVal(map[string]cty.Value{
				"input":            cty.StringVal("input"),
				"output":           cty.StringVal("input"),
				"triggers_replace": cty.StringVal("new-value"),
				"id":               cty.StringVal("not-quite-unique"),
			}),
		},

		"update-input-trigger": {
			prior: cty.ObjectVal(map[string]cty.Value{
				"input":  cty.StringVal("input"),
				"output": cty.StringVal("input"),
				"triggers_replace": cty.MapVal(map[string]cty.Value{
					"key": cty.StringVal("value"),
				}),
				"id": cty.StringVal("not-quite-unique"),
			}),
			planned: cty.ObjectVal(map[string]cty.Value{
				"input":  cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"output": cty.UnknownVal(cty.List(cty.String)),
				"triggers_replace": cty.MapVal(map[string]cty.Value{
					"key": cty.StringVal("new value"),
				}),
				"id": cty.UnknownVal(cty.String),
			}),
			state: cty.ObjectVal(map[string]cty.Value{
				"input":  cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"output": cty.ListVal([]cty.Value{cty.StringVal("new-input")}),
				"triggers_replace": cty.MapVal(map[string]cty.Value{
					"key": cty.StringVal("new value"),
				}),
				"id": cty.StringVal("not-quite-unique"),
			}),
		},
	} {
		t.Run("apply-"+name, func(t *testing.T) {
			req := providers.ApplyResourceChangeRequest{
				TypeName:     "terraform_data",
				PriorState:   tc.prior,
				PlannedState: tc.planned,
			}

			resp := applyDataStoreResourceChange(req)
			if resp.Diagnostics.HasErrors() {
				t.Fatal(resp.Diagnostics.ErrWithWarnings())
			}

			if !resp.NewState.RawEquals(tc.state) {
				t.Errorf("expected:\n%#v\ngot:\n%#v\n", tc.state, resp.NewState)
			}
		})
	}
}
