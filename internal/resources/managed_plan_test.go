// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestManagedResourceTypePlanChanges(t *testing.T) {
	emptySchema := &configschema.Block{
		// Intentionally empty
	}
	nameSchema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": {
				Type:     cty.String,
				Optional: true,
			},
		},
	}
	nameAndIDSchema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": {
				Type:     cty.String,
				Optional: true,
			},
			"id": {
				Type:     cty.String,
				Computed: true,
			},
		},
	}
	passwordSchema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"password": {
				Type:      cty.String,
				Optional:  true,
				Sensitive: true,
			},
		},
	}

	tests := map[string]struct {
		Schema                 *configschema.Block
		ProviderCanPlanDestroy bool
		PlanImpl               func(context.Context, providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse
		Request                *ManagedResourcePlanRequest

		WantResponse *ManagedResourcePlanResponse
		WantDiags    tfdiags.Diagnostics
	}{
		"empty unchanged": {
			Schema: emptySchema,
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
				DesiredValue: cty.EmptyObjectVal,
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
				DesiredValue: cty.EmptyObjectVal,
				Planned: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
			},
		},
		"name unchanged": {
			Schema: nameSchema,
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
			},
		},
		"diagnostics from provider": {
			Schema: emptySchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				var diags tfdiags.Diagnostics
				diags = diags.Append(tfdiags.SimpleWarning("It is dark. You might be eaten by a grue."))
				return providers.PlanResourceChangeResponse{
					PlannedState: req.ProposedNewState,
					Diagnostics:  diags,
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
				DesiredValue: cty.EmptyObjectVal,
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
				DesiredValue: cty.EmptyObjectVal,
				Planned: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
			},
			WantDiags: (tfdiags.Diagnostics)(nil).Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"It is dark. You might be eaten by a grue.",
				``,
			)),
		},
		"private data": {
			Schema: nameSchema,
			PlanImpl: func(ctx context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				return providers.PlanResourceChangeResponse{
					PlannedState:   req.ProposedNewState,
					PlannedPrivate: []byte("!!!"),
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
					Private: []byte("..."),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
					Private: []byte("..."), // preserved verbatim from the input
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
					Private: []byte("!!!"), // from the provider's planning response
				},
			},
		},
		"proposed new value": {
			Schema: nameAndIDSchema,
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"id":   cty.StringVal("imp-ab463455"),
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"id":   cty.NullVal(cty.String),
					"name": cty.StringVal("rumleskaft"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"id":   cty.StringVal("imp-ab463455"),
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"id":   cty.NullVal(cty.String),
					"name": cty.StringVal("rumleskaft"),
				}),
				Planned: ValueWithPrivate{
					// Because this test uses the fake provider's default
					// implementation of planning, the planned new value is
					// just what was proposed by our core logic. This is
					// therefore indirectly testing that we're calling
					// [objchange.ProposedNew] by expecting what that function
					// should produce for the given current and desired values.
					// This is not an exhaustive test of objchange.ProposedNew's
					// behavior though; it has its own tests.
					Value: cty.ObjectVal(map[string]cty.Value{
						// The computed attribute value from "current".
						"id": cty.StringVal("imp-ab463455"),
						// The optional attribute value from "desired".
						"name": cty.StringVal("rumleskaft"),
					}),
				},
			},
		},
		"marks preserved": {
			Schema: nameSchema,
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin").Mark("imp"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin").Mark("mischievous"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin").Mark("imp"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin").Mark("mischievous"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin").Mark("mischievous").Mark("imp"),
					}),
				},
			},
		},
		"sensitive marks added": {
			Schema: passwordSchema,
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"password": cty.StringVal("1234"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"password": cty.StringVal("12345"), // much more secure!
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"password": cty.StringVal("1234"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"password": cty.StringVal("12345"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"password": cty.StringVal("12345"),
						// FIXME: We've not actually implemented the schema-based marking yet,
						// but this result should actually be:
						// cty.StringVal("12345").Mark(marks.Sensitive),
					}),
				},
			},
		},
		"absent ProviderMeta given as null": {
			Schema: emptySchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				var diags tfdiags.Diagnostics
				// Real-world providers crash if given a nil ProviderMeta,
				// so our PlanChanges implementation must automatically
				// substitute a null value when the input is nil.
				if req.ProviderMeta == cty.NilVal || !req.ProviderMeta.IsNull() {
					diags = diags.Append(fmt.Errorf("provider meta is %#v; want null", req.ProviderMeta))
				}
				return providers.PlanResourceChangeResponse{
					PlannedState: req.ProposedNewState,
					Diagnostics:  diags,
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
				DesiredValue:      cty.EmptyObjectVal,
				ProviderMetaValue: cty.NilVal, // zero value of cty.Value
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
				DesiredValue: cty.EmptyObjectVal,
				Planned: ValueWithPrivate{
					Value: cty.EmptyObjectVal,
				},
			},
		},
		"requires replacement when updating": {
			Schema: nameSchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				return providers.PlanResourceChangeResponse{
					PlannedState: req.ProposedNewState,
					RequiresReplace: []cty.Path{
						cty.GetAttrPath("name"),
					},
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumleskaft"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumleskaft"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumleskaft"),
					}),
				},
				RequiresReplace: []cty.Path{
					cty.GetAttrPath("name"),
				},
			},
		},
		"requires replacement when creating": {
			Schema: nameSchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				return providers.PlanResourceChangeResponse{
					PlannedState: req.ProposedNewState,
					RequiresReplace: []cty.Path{
						cty.GetAttrPath("name"),
					},
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				// Some real-world providers return a nonsensical "requires
				// replace" even when they're describing the creation of
				// something new. PlanChanges is responsible for discarding
				// the paths in that case so that the rest of the system can
				// assume that RequiresReplace is set only when a "replace"
				// action would be appropriate.
				RequiresReplace: nil,
			},
		},
		"requires replacement when destroying": {
			Schema: nameSchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				return providers.PlanResourceChangeResponse{
					PlannedState: req.ProposedNewState,
					RequiresReplace: []cty.Path{
						cty.GetAttrPath("name"),
					},
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.NullVal(cty.Object(map[string]cty.Type{
					"name": cty.String,
				})),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.NullVal(cty.Object(map[string]cty.Type{
					"name": cty.String,
				})),
				Planned: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
				// Some real-world providers return a nonsensical "requires
				// replace" even when they're describing the creation of
				// something new. PlanChanges is responsible for discarding
				// the paths in that case so that the rest of the system can
				// assume that RequiresReplace is set only when a "replace"
				// action would be appropriate.
				RequiresReplace: nil,
			},
		},
		"invalid plan": {
			Schema: nameSchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				return providers.PlanResourceChangeResponse{
					PlannedState: cty.ObjectVal(map[string]cty.Value{
						// This is emulating one potential kind of misbehavior:
						// returning a case-normalized version of the input,
						// rather than preserving the input and then treating
						// it as case-sensitive in future rounds.
						"name": cty.StringVal("RUMPELSTILTSKIN"),

						// This test intentionally doesn't exhaustively cover
						// all possible ways a plan can be invalid, because
						// the underlying function in "package objchange"
						// already has its own unit tests covering all that.
						// We're essentially just testing that that function
						// gets called at all.
					}),
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
			},
			WantDiags: (tfdiags.Diagnostics)(nil).Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Provider produced invalid plan",
				`Provider "terraform.io/builtin/test" planned an invalid value for test_thing.test.name: planned value cty.StringVal("RUMPELSTILTSKIN") does not match config value cty.StringVal("rumpelstiltskin").

This is a bug in the provider, which should be reported in the provider's own issue tracker.`,
			)),
		},
		"invalid plan from legacy plugin SDK": {
			Schema: nameSchema,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				return providers.PlanResourceChangeResponse{
					PlannedState: cty.ObjectVal(map[string]cty.Value{
						// This is not an acceptable plan but PlanChanges must
						// allow it anyway because we're also going to report
						// that we're the legacy Terraform Plugin SDK, which
						// was originally made for Terraform v1.11 and earlier
						// and isn't capable of implementing the modern protocol
						// properly.
						"name": cty.StringVal("RUMPELSTILTSKIN"),
					}),
					// This is how the Terraform Plugin SDK excuses itself from
					// implementing the protocol correctly.
					LegacyTypeSystem: true,
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
				DesiredValue: cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("rumpelstiltskin"),
				}),
				Planned: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						// The invalid value is allowed to leak out in this
						// case, because the quirks that causes are less
						// disruptive than completely refusing to interact
						// with old provider implementations.
						"name": cty.StringVal("RUMPELSTILTSKIN"),
					}),
				},
			},
		},
		"provider vetoes destroy plan": {
			Schema:                 nameSchema,
			ProviderCanPlanDestroy: true,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				var diags tfdiags.Diagnostics
				diags = diags.Append(fmt.Errorf("destroy not supported"))
				return providers.PlanResourceChangeResponse{
					Diagnostics: diags,
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.NullVal(cty.Object(map[string]cty.Type{
					"name": cty.String,
				})),
			},
			WantDiags: (tfdiags.Diagnostics)(nil).Append(tfdiags.Sourceless(
				tfdiags.Error,
				"destroy not supported",
				``,
			)),
		},
		"provider doesn't support destroy planning": {
			Schema:                 nameSchema,
			ProviderCanPlanDestroy: false,
			PlanImpl: func(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
				// Originally providers were only asked to plan create or update
				// actions and the core system just made a synthetic plan for
				// destroy. Later there was a requirement to support systems
				// where destroying either isn't possible at all or involves a
				// warning to the operator, so support for destroy-planning was
				// added to the protocol but only when the provider announces
				// a capability to handle it because existing providers would
				// crash in that case. This test is covering the situation where
				// the provider _doesn't_ opt in, and so this function should
				// never be called.
				var diags tfdiags.Diagnostics
				diags = diags.Append(fmt.Errorf("provider should not be asked to plan destroy"))
				return providers.PlanResourceChangeResponse{
					Diagnostics: diags,
				}
			},
			Request: &ManagedResourcePlanRequest{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.NullVal(cty.Object(map[string]cty.Type{
					"name": cty.String,
				})),
			},
			WantResponse: &ManagedResourcePlanResponse{
				Current: ValueWithPrivate{
					Value: cty.ObjectVal(map[string]cty.Value{
						"name": cty.StringVal("rumpelstiltskin"),
					}),
				},
				DesiredValue: cty.NullVal(cty.Object(map[string]cty.Type{
					"name": cty.String,
				})),
				Planned: ValueWithPrivate{
					Value: cty.NullVal(cty.Object(map[string]cty.Type{
						"name": cty.String,
					})),
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			providerClient := &fakeProviderClient{
				schema: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_thing": {
							Block: test.Schema,
						},
					},
					ServerCapabilities: providers.ServerCapabilities{
						PlanDestroy: test.ProviderCanPlanDestroy,
					},
				},
				planResourceChange: test.PlanImpl,
			}
			resourceType := NewManagedResourceType(
				addrs.NewBuiltInProvider("test"),
				"test_thing",
				providerClient,
			)
			objAddr := addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_thing",
				Name: "test",
			}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey).CurrentObject()

			gotResp, gotDiags := resourceType.PlanChanges(t.Context(), test.Request, objAddr)
			if diff := cmp.Diff(test.WantResponse, gotResp, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong response\n" + diff)
			}
			// We'll use the "ForRPC" form of diagnostics here just because
			// it's a form that's friendly to diffing in this way and we're
			// interested only in the diagnostic content as end-users would
			// experience it, rather than the internal representation details.
			if diff := cmp.Diff(test.WantDiags.ForRPC(), gotDiags.ForRPC(), ctydebug.CmpOptions); diff != "" {
				t.Error("wrong diagnostics\n" + diff)
			}
		})
	}
}
