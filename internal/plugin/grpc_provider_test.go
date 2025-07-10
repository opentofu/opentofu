// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/legacy/hcl2shim"
	mockproto "github.com/opentofu/opentofu/internal/plugin/mock_proto"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	proto "github.com/opentofu/opentofu/internal/tfplugin5"
)

var _ providers.Interface = (*GRPCProvider)(nil)

func mockProviderClient(t *testing.T) *mockproto.MockProviderClient {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)

	// we always need a GetSchema method
	client.EXPECT().GetSchema(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(providerProtoSchema(), nil)

	return client
}

func checkDiags(t *testing.T, d tfdiags.Diagnostics) {
	t.Helper()
	if d.HasErrors() {
		t.Fatal(d.Err())
	}
}

// checkDiagsHasError ensures error diagnostics are present or fails the test.
func checkDiagsHasError(t *testing.T, d tfdiags.Diagnostics) {
	t.Helper()

	if !d.HasErrors() {
		t.Fatal("expected error diagnostics")
	}
}

func providerProtoSchema() *proto.GetProviderSchema_Response {
	return &proto.GetProviderSchema_Response{
		Provider: &proto.Schema{
			Block: &proto.Schema_Block{
				Attributes: []*proto.Schema_Attribute{
					{
						Name:     "attr",
						Type:     []byte(`"string"`),
						Required: true,
					},
				},
			},
		},
		ResourceSchemas: map[string]*proto.Schema{
			"resource": &proto.Schema{
				Version: 1,
				Block: &proto.Schema_Block{
					Attributes: []*proto.Schema_Attribute{
						{
							Name:     "attr",
							Type:     []byte(`"string"`),
							Required: true,
						},
					},
				},
			},
		},
		DataSourceSchemas: map[string]*proto.Schema{
			"data": &proto.Schema{
				Version: 1,
				Block: &proto.Schema_Block{
					Attributes: []*proto.Schema_Attribute{
						{
							Name:     "attr",
							Type:     []byte(`"string"`),
							Required: true,
						},
					},
				},
			},
		},
		EphemeralResourceSchemas: map[string]*proto.Schema{
			"eph": {
				Version: 1,
				Block: &proto.Schema_Block{
					Attributes: []*proto.Schema_Attribute{
						{
							Name:     "attr",
							Type:     []byte(`"string"`),
							Required: true,
						},
					},
				},
			},
		},
		Functions: map[string]*proto.Function{
			"fn": &proto.Function{
				Parameters: []*proto.Function_Parameter{{
					Name:               "par_a",
					Type:               []byte(`"string"`),
					AllowNullValue:     false,
					AllowUnknownValues: false,
				}},
				VariadicParameter: &proto.Function_Parameter{
					Name:               "par_var",
					Type:               []byte(`"string"`),
					AllowNullValue:     true,
					AllowUnknownValues: false,
				},
				Return: &proto.Function_Return{
					Type: []byte(`"string"`),
				},
			},
		},
	}
}

func TestGRPCProvider_GetSchema(t *testing.T) {
	p := &GRPCProvider{
		client: mockProviderClient(t),
	}

	resp := p.GetProviderSchema(t.Context())
	checkDiags(t, resp.Diagnostics)

	{ // check ephemeral attribute of the schema blocks
		if !resp.Provider.Block.Ephemeral {
			t.Errorf("provider.Block.Ephemeral meant to be true")
		}
		checkResources := func(t *testing.T, r map[string]providers.Schema, want bool) {
			for typ, schema := range r {
				if schema.Block.Ephemeral != want {
					t.Errorf("expected resource %q to have ephemeral as %t", typ, want)
				}
			}
		}
		checkResources(t, resp.ResourceTypes, false)
		checkResources(t, resp.DataSources, false)
		checkResources(t, resp.EphemeralResources, true)
	}
}

// Ensure that gRPC errors are returned early.
// Reference: https://github.com/hashicorp/terraform/issues/31047
func TestGRPCProvider_GetSchema_GRPCError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)

	client.EXPECT().GetSchema(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.GetProviderSchema_Response{}, fmt.Errorf("test error"))

	p := &GRPCProvider{
		client: client,
	}

	resp := p.GetProviderSchema(t.Context())

	checkDiagsHasError(t, resp.Diagnostics)
}

func TestGRPCProvider_GetSchema_GlobalCacheEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)
	// The SchemaCache is global and is saved between test runs
	providers.SchemaCache = providers.NewMockSchemaCache()

	providerAddr := addrs.Provider{
		Namespace: "namespace",
		Type:      "type",
	}

	mockedProviderResponse := &proto.Schema{Version: 2, Block: &proto.Schema_Block{}}

	client.EXPECT().GetSchema(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Times(1).Return(&proto.GetProviderSchema_Response{
		Provider:           mockedProviderResponse,
		ServerCapabilities: &proto.ServerCapabilities{GetProviderSchemaOptional: true},
	}, nil)

	// Run GetProviderTwice, expect GetSchema to be called once
	// Re-initialize the provider before each run to avoid usage of the local cache
	p := &GRPCProvider{
		client: client,
		Addr:   providerAddr,
	}
	resp := p.GetProviderSchema(t.Context())

	checkDiags(t, resp.Diagnostics)
	if !cmp.Equal(resp.Provider.Version, mockedProviderResponse.Version) {
		t.Fatal(cmp.Diff(resp.Provider.Version, mockedProviderResponse.Version))
	}

	p = &GRPCProvider{
		client: client,
		Addr:   providerAddr,
	}
	resp = p.GetProviderSchema(t.Context())

	checkDiags(t, resp.Diagnostics)
	if !cmp.Equal(resp.Provider.Version, mockedProviderResponse.Version) {
		t.Fatal(cmp.Diff(resp.Provider.Version, mockedProviderResponse.Version))
	}
}

func TestGRPCProvider_GetSchema_GlobalCacheDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)
	// The SchemaCache is global and is saved between test runs
	providers.SchemaCache = providers.NewMockSchemaCache()

	providerAddr := addrs.Provider{
		Namespace: "namespace",
		Type:      "type",
	}

	mockedProviderResponse := &proto.Schema{Version: 2, Block: &proto.Schema_Block{}}

	client.EXPECT().GetSchema(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Times(2).Return(&proto.GetProviderSchema_Response{
		Provider:           mockedProviderResponse,
		ServerCapabilities: &proto.ServerCapabilities{GetProviderSchemaOptional: false},
	}, nil)

	// Run GetProviderTwice, expect GetSchema to be called once
	// Re-initialize the provider before each run to avoid usage of the local cache
	p := &GRPCProvider{
		client: client,
		Addr:   providerAddr,
	}
	resp := p.GetProviderSchema(t.Context())

	checkDiags(t, resp.Diagnostics)
	if !cmp.Equal(resp.Provider.Version, mockedProviderResponse.Version) {
		t.Fatal(cmp.Diff(resp.Provider.Version, mockedProviderResponse.Version))
	}

	p = &GRPCProvider{
		client: client,
		Addr:   providerAddr,
	}
	resp = p.GetProviderSchema(t.Context())

	checkDiags(t, resp.Diagnostics)
	if !cmp.Equal(resp.Provider.Version, mockedProviderResponse.Version) {
		t.Fatal(cmp.Diff(resp.Provider.Version, mockedProviderResponse.Version))
	}
}

// Ensure that provider error diagnostics are returned early.
// Reference: https://github.com/hashicorp/terraform/issues/31047
func TestGRPCProvider_GetSchema_ResponseErrorDiagnostic(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)

	client.EXPECT().GetSchema(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.GetProviderSchema_Response{
		Diagnostics: []*proto.Diagnostic{
			{
				Severity: proto.Diagnostic_ERROR,
				Summary:  "error summary",
				Detail:   "error detail",
			},
		},
		// Trigger potential panics
		Provider: &proto.Schema{},
	}, nil)

	p := &GRPCProvider{
		client: client,
	}

	resp := p.GetProviderSchema(t.Context())

	checkDiagsHasError(t, resp.Diagnostics)
}

func TestGRPCProvider_PrepareProviderConfig(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().PrepareProviderConfig(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.PrepareProviderConfig_Response{}, nil)

	cfg := hcl2shim.HCL2ValueFromConfigValue(map[string]interface{}{"attr": "value"})
	resp := p.ValidateProviderConfig(t.Context(), providers.ValidateProviderConfigRequest{Config: cfg})
	checkDiags(t, resp.Diagnostics)
}

func TestGRPCProvider_ValidateResourceConfig(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ValidateResourceTypeConfig(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ValidateResourceTypeConfig_Response{}, nil)

	cfg := hcl2shim.HCL2ValueFromConfigValue(map[string]interface{}{"attr": "value"})
	resp := p.ValidateResourceConfig(t.Context(), providers.ValidateResourceConfigRequest{
		TypeName: "resource",
		Config:   cfg,
	})
	checkDiags(t, resp.Diagnostics)
}

func TestGRPCProvider_ValidateDataSourceConfig(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ValidateDataSourceConfig(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ValidateDataSourceConfig_Response{}, nil)

	cfg := hcl2shim.HCL2ValueFromConfigValue(map[string]interface{}{"attr": "value"})
	resp := p.ValidateDataResourceConfig(t.Context(), providers.ValidateDataResourceConfigRequest{
		TypeName: "data",
		Config:   cfg,
	})
	checkDiags(t, resp.Diagnostics)
}

func TestGRPCProvider_ValidateEphemeralResourceConfig(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ValidateEphemeralResourceConfig(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ValidateEphemeralResourceConfig_Response{}, nil)

	cfg := hcl2shim.HCL2ValueFromConfigValue(map[string]interface{}{"attr": "value"})
	resp := p.ValidateEphemeralConfig(t.Context(), providers.ValidateEphemeralConfigRequest{
		TypeName: "eph",
		Config:   cfg,
	})
	checkDiags(t, resp.Diagnostics)
}

func TestGRPCProvider_UpgradeResourceState(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().UpgradeResourceState(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.UpgradeResourceState_Response{
		UpgradedState: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
	}, nil)

	resp := p.UpgradeResourceState(t.Context(), providers.UpgradeResourceStateRequest{
		TypeName:     "resource",
		Version:      0,
		RawStateJSON: []byte(`{"old_attr":"bar"}`),
	})
	checkDiags(t, resp.Diagnostics)

	expected := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expected, resp.UpgradedState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.UpgradedState, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_UpgradeResourceStateJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().UpgradeResourceState(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.UpgradeResourceState_Response{
		UpgradedState: &proto.DynamicValue{
			Json: []byte(`{"attr":"bar"}`),
		},
	}, nil)

	resp := p.UpgradeResourceState(t.Context(), providers.UpgradeResourceStateRequest{
		TypeName:     "resource",
		Version:      0,
		RawStateJSON: []byte(`{"old_attr":"bar"}`),
	})
	checkDiags(t, resp.Diagnostics)

	expected := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expected, resp.UpgradedState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.UpgradedState, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_MoveResourceState(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().MoveResourceState(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.MoveResourceState_Response{
		TargetState: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
		TargetPrivate: []byte(`{"meta": "data"}`),
	}, nil)

	resp := p.MoveResourceState(t.Context(), providers.MoveResourceStateRequest{
		SourceTypeName:      "resource_old",
		SourceSchemaVersion: 0,
		TargetTypeName:      "resource",
	})
	checkDiags(t, resp.Diagnostics)

	expectedState := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})
	expectedPrivate := []byte(`{"meta": "data"}`)

	if !cmp.Equal(expectedState, resp.TargetState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedState, resp.TargetState, typeComparer, valueComparer, equateEmpty))
	}
	if !bytes.Equal(expectedPrivate, resp.TargetPrivate) {
		t.Fatalf("expected %q, got %q", expectedPrivate, resp.TargetPrivate)
	}
}

func TestGRPCProvider_Configure(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().Configure(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.Configure_Response{}, nil)

	resp := p.ConfigureProvider(t.Context(), providers.ConfigureProviderRequest{
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
	})
	checkDiags(t, resp.Diagnostics)
}

func TestGRPCProvider_Stop(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().Stop(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.Stop_Response{}, nil)

	err := p.Stop(t.Context())
	if err != nil {
		t.Fatal(err)
	}
}

func TestGRPCProvider_ReadResource(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ReadResource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ReadResource_Response{
		NewState: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
	}, nil)

	resp := p.ReadResource(t.Context(), providers.ReadResourceRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
	})

	checkDiags(t, resp.Diagnostics)

	expected := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expected, resp.NewState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.NewState, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_ReadResourceJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ReadResource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ReadResource_Response{
		NewState: &proto.DynamicValue{
			Json: []byte(`{"attr":"bar"}`),
		},
	}, nil)

	resp := p.ReadResource(t.Context(), providers.ReadResourceRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
	})

	checkDiags(t, resp.Diagnostics)

	expected := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expected, resp.NewState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.NewState, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_ReadEmptyJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ReadResource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ReadResource_Response{
		NewState: &proto.DynamicValue{
			Json: []byte(``),
		},
	}, nil)

	obj := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("foo"),
	})
	resp := p.ReadResource(t.Context(), providers.ReadResourceRequest{
		TypeName:   "resource",
		PriorState: obj,
	})

	checkDiags(t, resp.Diagnostics)

	expected := cty.NullVal(obj.Type())

	if !cmp.Equal(expected, resp.NewState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.NewState, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_PlanResourceChange(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	expectedPrivate := []byte(`{"meta": "data"}`)

	client.EXPECT().PlanResourceChange(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.PlanResourceChange_Response{
		PlannedState: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
		RequiresReplace: []*proto.AttributePath{
			{
				Steps: []*proto.AttributePath_Step{
					{
						Selector: &proto.AttributePath_Step_AttributeName{
							AttributeName: "attr",
						},
					},
				},
			},
		},
		PlannedPrivate: expectedPrivate,
	}, nil)

	resp := p.PlanResourceChange(t.Context(), providers.PlanResourceChangeRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
		ProposedNewState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
	})

	checkDiags(t, resp.Diagnostics)

	expectedState := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expectedState, resp.PlannedState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedState, resp.PlannedState, typeComparer, valueComparer, equateEmpty))
	}

	expectedReplace := `[]cty.Path{cty.Path{cty.GetAttrStep{Name:"attr"}}}`
	replace := fmt.Sprintf("%#v", resp.RequiresReplace)
	if expectedReplace != replace {
		t.Fatalf("expected %q, got %q", expectedReplace, replace)
	}

	if !bytes.Equal(expectedPrivate, resp.PlannedPrivate) {
		t.Fatalf("expected %q, got %q", expectedPrivate, resp.PlannedPrivate)
	}
}

func TestGRPCProvider_PlanResourceChange_deferred(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().PlanResourceChange(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.PlanResourceChange_Response{
		PlannedState: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
		Deferred: &proto.Deferred{
			Reason: proto.Deferred_PROVIDER_CONFIG_UNKNOWN,
		},
	}, nil)

	resp := p.PlanResourceChange(t.Context(), providers.PlanResourceChangeRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
		ProposedNewState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
	})

	if len(resp.Diagnostics) != 1 {
		t.Fatal("wrong number of diagnostics; want one\n" + spew.Sdump(resp.Diagnostics))
	}
	desc := resp.Diagnostics[0].Description()
	if got, want := desc.Summary, `Provider configuration is incomplete`; got != want {
		t.Errorf("wrong error summary\ngot:  %s\nwant: %s", got, want)
	}
	if got, want := desc.Detail, `The provider was unable to work with this resource because the associated provider configuration makes use of values from other resources that will not be known until after apply.`; got != want {
		t.Errorf("wrong error detail\ngot:  %s\nwant: %s", got, want)
	}
	if !providers.IsDeferralDiagnostic(resp.Diagnostics[0]) {
		t.Errorf("diagnostic is not marked as being a \"deferral diagnostic\"")
	}
}

func TestGRPCProvider_PlanResourceChangeJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	expectedPrivate := []byte(`{"meta": "data"}`)

	client.EXPECT().PlanResourceChange(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.PlanResourceChange_Response{
		PlannedState: &proto.DynamicValue{
			Json: []byte(`{"attr":"bar"}`),
		},
		RequiresReplace: []*proto.AttributePath{
			{
				Steps: []*proto.AttributePath_Step{
					{
						Selector: &proto.AttributePath_Step_AttributeName{
							AttributeName: "attr",
						},
					},
				},
			},
		},
		PlannedPrivate: expectedPrivate,
	}, nil)

	resp := p.PlanResourceChange(t.Context(), providers.PlanResourceChangeRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
		ProposedNewState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
	})

	checkDiags(t, resp.Diagnostics)

	expectedState := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expectedState, resp.PlannedState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedState, resp.PlannedState, typeComparer, valueComparer, equateEmpty))
	}

	expectedReplace := `[]cty.Path{cty.Path{cty.GetAttrStep{Name:"attr"}}}`
	replace := fmt.Sprintf("%#v", resp.RequiresReplace)
	if expectedReplace != replace {
		t.Fatalf("expected %q, got %q", expectedReplace, replace)
	}

	if !bytes.Equal(expectedPrivate, resp.PlannedPrivate) {
		t.Fatalf("expected %q, got %q", expectedPrivate, resp.PlannedPrivate)
	}
}

func TestGRPCProvider_ApplyResourceChange(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	expectedPrivate := []byte(`{"meta": "data"}`)

	client.EXPECT().ApplyResourceChange(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ApplyResourceChange_Response{
		NewState: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
		Private: expectedPrivate,
	}, nil)

	resp := p.ApplyResourceChange(t.Context(), providers.ApplyResourceChangeRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
		PlannedState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		PlannedPrivate: expectedPrivate,
	})

	checkDiags(t, resp.Diagnostics)

	expectedState := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expectedState, resp.NewState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedState, resp.NewState, typeComparer, valueComparer, equateEmpty))
	}

	if !bytes.Equal(expectedPrivate, resp.Private) {
		t.Fatalf("expected %q, got %q", expectedPrivate, resp.Private)
	}
}
func TestGRPCProvider_ApplyResourceChangeJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	expectedPrivate := []byte(`{"meta": "data"}`)

	client.EXPECT().ApplyResourceChange(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ApplyResourceChange_Response{
		NewState: &proto.DynamicValue{
			Json: []byte(`{"attr":"bar"}`),
		},
		Private: expectedPrivate,
	}, nil)

	resp := p.ApplyResourceChange(t.Context(), providers.ApplyResourceChangeRequest{
		TypeName: "resource",
		PriorState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
		PlannedState: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		PlannedPrivate: expectedPrivate,
	})

	checkDiags(t, resp.Diagnostics)

	expectedState := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expectedState, resp.NewState, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedState, resp.NewState, typeComparer, valueComparer, equateEmpty))
	}

	if !bytes.Equal(expectedPrivate, resp.Private) {
		t.Fatalf("expected %q, got %q", expectedPrivate, resp.Private)
	}
}

func TestGRPCProvider_ImportResourceState(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	expectedPrivate := []byte(`{"meta": "data"}`)

	client.EXPECT().ImportResourceState(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ImportResourceState_Response{
		ImportedResources: []*proto.ImportResourceState_ImportedResource{
			{
				TypeName: "resource",
				State: &proto.DynamicValue{
					Msgpack: []byte("\x81\xa4attr\xa3bar"),
				},
				Private: expectedPrivate,
			},
		},
	}, nil)

	resp := p.ImportResourceState(t.Context(), providers.ImportResourceStateRequest{
		TypeName: "resource",
		ID:       "foo",
	})

	checkDiags(t, resp.Diagnostics)

	expectedResource := providers.ImportedResource{
		TypeName: "resource",
		State: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Private: expectedPrivate,
	}

	imported := resp.ImportedResources[0]
	if !cmp.Equal(expectedResource, imported, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedResource, imported, typeComparer, valueComparer, equateEmpty))
	}
}
func TestGRPCProvider_ImportResourceStateJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	expectedPrivate := []byte(`{"meta": "data"}`)

	client.EXPECT().ImportResourceState(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ImportResourceState_Response{
		ImportedResources: []*proto.ImportResourceState_ImportedResource{
			{
				TypeName: "resource",
				State: &proto.DynamicValue{
					Json: []byte(`{"attr":"bar"}`),
				},
				Private: expectedPrivate,
			},
		},
	}, nil)

	resp := p.ImportResourceState(t.Context(), providers.ImportResourceStateRequest{
		TypeName: "resource",
		ID:       "foo",
	})

	checkDiags(t, resp.Diagnostics)

	expectedResource := providers.ImportedResource{
		TypeName: "resource",
		State: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		}),
		Private: expectedPrivate,
	}

	imported := resp.ImportedResources[0]
	if !cmp.Equal(expectedResource, imported, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expectedResource, imported, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_ReadDataSource(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ReadDataSource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ReadDataSource_Response{
		State: &proto.DynamicValue{
			Msgpack: []byte("\x81\xa4attr\xa3bar"),
		},
	}, nil)

	resp := p.ReadDataSource(t.Context(), providers.ReadDataSourceRequest{
		TypeName: "data",
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
	})

	checkDiags(t, resp.Diagnostics)

	expected := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expected, resp.State, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.State, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_ReadDataSourceJSON(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().ReadDataSource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.ReadDataSource_Response{
		State: &proto.DynamicValue{
			Json: []byte(`{"attr":"bar"}`),
		},
	}, nil)

	resp := p.ReadDataSource(t.Context(), providers.ReadDataSourceRequest{
		TypeName: "data",
		Config: cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("foo"),
		}),
	})

	checkDiags(t, resp.Diagnostics)

	expected := cty.ObjectVal(map[string]cty.Value{
		"attr": cty.StringVal("bar"),
	})

	if !cmp.Equal(expected, resp.State, typeComparer, valueComparer, equateEmpty) {
		t.Fatal(cmp.Diff(expected, resp.State, typeComparer, valueComparer, equateEmpty))
	}
}

func TestGRPCProvider_OpenEphemeralResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := mockProviderClient(t)
		p := &GRPCProvider{
			client: client,
		}

		future := time.Now().Add(time.Minute)
		client.EXPECT().OpenEphemeralResource(
			gomock.Any(),
			gomock.Any(),
		).Return(&proto.OpenEphemeralResource_Response{
			Result: &proto.DynamicValue{
				Msgpack: []byte("\x81\xa4attr\xa3bar"),
			},
			Private: []byte("private data"),
			RenewAt: timestamppb.New(future),
			Deferred: &proto.Deferred{
				Reason: proto.Deferred_RESOURCE_CONFIG_UNKNOWN,
			},
		}, nil)

		resp := p.OpenEphemeralResource(t.Context(), providers.OpenEphemeralResourceRequest{
			TypeName: "eph",
			Config: cty.ObjectVal(map[string]cty.Value{
				"attr": cty.StringVal("foo"),
			}),
		})

		checkDiags(t, resp.Diagnostics)

		expected := cty.ObjectVal(map[string]cty.Value{
			"attr": cty.StringVal("bar"),
		})
		if diff := cmp.Diff(expected, resp.Result, typeComparer, valueComparer, equateEmpty); diff != "" {
			t.Fatalf("expected to have no diff between the expected result and result from the openEphemeral. got: %s", diff)
		}
		if resp.RenewAt == nil || !future.Equal(*resp.RenewAt) {
			t.Fatalf("unexpected renewAt. got: %s, want %s", resp.RenewAt, future)
		}
		if got, want := resp.Private, []byte("private data"); !slices.Equal(got, want) {
			t.Fatalf("unexpected private data. got: %q, want %q", got, want)
		}
		{
			if resp.Deferred == nil {
				t.Fatal("expected to have a deferred object but got none")
			}
			if got, want := resp.Deferred.DeferralReason, providers.DeferredBecauseResourceConfigUnknown; got != want {
				t.Fatalf("unexpected deferred reason. got: %d, want %d", got, want)
			}
		}
	})
	t.Run("requested type is not in schema", func(t *testing.T) {
		client := mockProviderClient(t)
		p := &GRPCProvider{
			client: client,
		}

		resp := p.OpenEphemeralResource(t.Context(), providers.OpenEphemeralResourceRequest{
			TypeName: "non_existing",
			Config: cty.ObjectVal(map[string]cty.Value{
				"attr": cty.StringVal("foo"),
			}),
		})
		checkDiagsHasError(t, resp.Diagnostics)
		if got, want := resp.Diagnostics.Err().Error(), `unknown ephemeral resource "non_existing"`; !strings.Contains(got, want) {
			t.Fatalf("diagnostis does not contain the expected content. got: %s; want: %s", got, want)
		}
	})
}

func TestGRPCProvider_RenewEphemeralResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)
	p := &GRPCProvider{
		client: client,
	}

	future := time.Now().Add(time.Minute)
	client.EXPECT().RenewEphemeralResource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.RenewEphemeralResource_Response{
		Private: []byte("private data new"),
		RenewAt: timestamppb.New(future),
	}, nil)

	resp := p.RenewEphemeralResource(t.Context(), providers.RenewEphemeralResourceRequest{
		TypeName: "eph",
	})

	checkDiags(t, resp.Diagnostics)

	if resp.RenewAt == nil || !future.Equal(*resp.RenewAt) {
		t.Fatalf("unexpected renewAt. got: %s, want %s", resp.RenewAt, future)
	}

	if got, want := resp.Private, []byte("private data new"); !slices.Equal(got, want) {
		t.Fatalf("unexpected private data. got: %q, want %q", got, want)
	}
}

func TestGRPCProvider_CloseEphemeralResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockproto.NewMockProviderClient(ctrl)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().CloseEphemeralResource(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.CloseEphemeralResource_Response{}, nil)

	resp := p.CloseEphemeralResource(t.Context(), providers.CloseEphemeralResourceRequest{
		TypeName: "eph",
	})

	checkDiags(t, resp.Diagnostics)
}

func TestGRPCProvider_CallFunction(t *testing.T) {
	client := mockProviderClient(t)
	p := &GRPCProvider{
		client: client,
	}

	client.EXPECT().CallFunction(
		gomock.Any(),
		gomock.Any(),
	).Return(&proto.CallFunction_Response{
		Result: &proto.DynamicValue{Json: []byte(`"foo"`)},
	}, nil)

	resp := p.CallFunction(t.Context(), providers.CallFunctionRequest{
		Name:      "fn",
		Arguments: []cty.Value{cty.StringVal("bar"), cty.NilVal},
	})

	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	if resp.Result != cty.StringVal("foo") {
		t.Fatalf("%v", resp.Result)
	}
}
