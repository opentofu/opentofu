// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/states"
)

func TestOperationResourceInstanceCurrentMeta(t *testing.T) {
	instAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test",
		Name: "example",
	}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
	providerInstAddr := addrs.AbsProviderConfigCorrect{
		Module: addrs.RootModuleInstance,
		Config: addrs.ProviderConfigCorrect{
			Provider: addrs.NewBuiltInProvider("test"),
			Alias:    "boop",
		},
	}.Instance(addrs.NoKey)

	// This set of tests relies indirectly on the behavior of
	// [exec.BuildResourceInstanceObjectMeta] and makes assumptions about its
	// expected results, but we do this only as a way to infer whether the
	// operation seems to have handled its arguments in the expected way and
	// not to exhaustively test all behavior of the underlying function.
	//
	// If the behavior of that function was intentionally changed upstream
	// then it's likely appropriate to update the expected test results to
	// match as long as our checks still demonstrate that the operation is
	// passing arguments to that function as expected.

	tests := map[string]struct {
		Addr       addrs.AbsResourceInstance
		Configs    []*testResourceConfig
		PriorState *states.ResourceInstanceObjectFull

		Check     func(t *testing.T, got *exec.ResourceInstanceObjectMeta)
		WantError string
	}{
		"config only": {
			Addr: instAddr,
			Configs: []*testResourceConfig{
				{
					Addr: instAddr.CurrentObject(),
					Meta: []eval.ModuleConfiguredResourceInstanceObjectMeta{
						{
							ModuleInstance: addrs.RootModuleInstance,
							ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
								Provider: &eval.ResourceInstanceObjectProvider{
									Provider:     providerInstAddr.Config.Config.Provider,
									ResourceMode: addrs.ManagedResourceMode,
									ResourceType: "test",
									Instance:     new(exprs.Known(providerInstAddr)),
								},
								ReplaceOrder: new(exprs.Known(resources.ReplaceAnyOrder)),
							},
						},
					},
				},
			},
			PriorState: nil,
			Check: func(t *testing.T, got *exec.ResourceInstanceObjectMeta) {
				if got == nil {
					t.Fatal("result is nil; want non-nil")
				}
				if got, want := got.Addr.String(), `test.example`; got != want {
					t.Errorf("wrong object address\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := got.Provider.String(), `terraform.io/builtin/test`; got != want {
					t.Errorf("wrong provider\ngot:  %s\nwant: %s", got, want)
				}
				if got, ok := got.ProviderInstance.ValueOk(); ok {
					if got, want := got.String(), `provider["terraform.io/builtin/test"].boop`; got != want {
						t.Errorf("wrong provider instance\ngot:  %s\nwant: %s", got, want)
					}
				} else {
					t.Errorf("provider instance is unknown; want known")
				}
				if got, ok := got.ReplaceOrder.ValueOk(); ok {
					if want := resources.ReplaceAnyOrder; got != want {
						t.Errorf("wrong replace order\ngot:  %s\nwant: %s", got, want)
					}
				} else {
					t.Errorf("replace order is unknown; want known")
				}
			},
		},
		"prior state only": {
			Addr:    instAddr,
			Configs: nil,
			PriorState: &states.ResourceInstanceObjectFull{
				Status:               states.ObjectReady,
				Value:                cty.EmptyObjectVal,
				ResourceType:         instAddr.Resource.Resource.Type,
				ProviderInstanceAddr: providerInstAddr,
			},
			Check: func(t *testing.T, got *exec.ResourceInstanceObjectMeta) {
				if got == nil {
					t.Fatal("result is nil; want non-nil")
				}
				if got, want := got.Addr.String(), `test.example`; got != want {
					t.Errorf("wrong object address\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := got.Provider.String(), `terraform.io/builtin/test`; got != want {
					t.Errorf("wrong provider\ngot:  %s\nwant: %s", got, want)
				}
				if got, ok := got.ProviderInstance.ValueOk(); ok {
					if got, want := got.String(), `provider["terraform.io/builtin/test"].boop`; got != want {
						t.Errorf("wrong provider instance\ngot:  %s\nwant: %s", got, want)
					}
				} else {
					t.Errorf("provider instance is unknown; want known")
				}
				if got, ok := got.ReplaceOrder.ValueOk(); ok {
					if want := resources.ReplaceAnyOrder; got != want {
						t.Errorf("wrong replace order\ngot:  %s\nwant: %s", got, want)
					}
				} else {
					t.Errorf("replace order is unknown; want known")
				}
			},
		},
		"config and prior state": {
			Addr: instAddr,
			Configs: []*testResourceConfig{
				{
					Addr: instAddr.CurrentObject(),
					Meta: []eval.ModuleConfiguredResourceInstanceObjectMeta{
						{
							ModuleInstance: addrs.RootModuleInstance,
							ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
								Provider: &eval.ResourceInstanceObjectProvider{
									Provider:     providerInstAddr.Config.Config.Provider,
									ResourceMode: addrs.ManagedResourceMode,
									ResourceType: "test",
								},
							},
						},
					},
				},
			},
			PriorState: &states.ResourceInstanceObjectFull{
				Status:               states.ObjectReady,
				Value:                cty.EmptyObjectVal,
				ResourceType:         instAddr.Resource.Resource.Type,
				ProviderInstanceAddr: providerInstAddr,
				CreateBeforeDestroy:  true,
			},
			Check: func(t *testing.T, got *exec.ResourceInstanceObjectMeta) {
				if got == nil {
					t.Fatal("result is nil; want non-nil")
				}
				if got, want := got.Addr.String(), `test.example`; got != want {
					t.Errorf("wrong object address\ngot:  %s\nwant: %s", got, want)
				}
				if got, want := got.Provider.String(), `terraform.io/builtin/test`; got != want {
					t.Errorf("wrong provider\ngot:  %s\nwant: %s", got, want)
				}
				if got, ok := got.ProviderInstance.ValueOk(); ok {
					if got, want := got.String(), `provider["terraform.io/builtin/test"].boop`; got != want {
						t.Errorf("wrong provider instance\ngot:  %s\nwant: %s", got, want)
					}
				} else {
					t.Errorf("provider instance is unknown; want known")
				}
				if got, ok := got.ReplaceOrder.ValueOk(); ok {
					if want := resources.ReplaceCreateFirst; got != want {
						t.Errorf("wrong replace order\ngot:  %s\nwant: %s", got, want)
					}
				} else {
					t.Errorf("replace order is unknown; want known")
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ops := operationsForTesting(
				nil, // We're bypassing the state-access operations for this test
				resourceConfigsForTesting(t, test.Configs, nil),
				nil, // This operation does not modify the state at all
				nil, // This operation does not need any plugins
			)

			// Normally the prior state would've been fetched by an earlier
			// call to ResourceInstancePrior in a real execution graph, but
			// we'll just provide it directly for the sake of this test because
			// we're focused only on ResourceInstanceCurrentMeta here.
			priorObj := &exec.ResourceInstanceObject{
				Addr:  test.Addr.CurrentObject(),
				State: test.PriorState,
			}

			got, gotDiags := ops.ResourceInstanceCurrentMeta(t.Context(), test.Addr, priorObj)

			if test.Check == nil && test.WantError == "" {
				t.Fatal("test has neither a check function nor an expected error substring")
			}

			if test.WantError != "" {
				if !gotDiags.HasErrors() {
					t.Errorf("unexpected success\nwant error substring: %s", test.WantError)
				} else if got, wantSub := gotDiags.Err().Error(), test.WantError; !strings.Contains(got, wantSub) {
					t.Errorf("wrong error\ngot:\n%s\nwant substring: %s", got, wantSub)
				}
			} else {
				if gotDiags.HasErrors() {
					t.Errorf("unexpected errors:\n%s", gotDiags.Err().Error())
				}
			}

			if test.Check != nil {
				test.Check(t, got)
			}
		})
	}
}
