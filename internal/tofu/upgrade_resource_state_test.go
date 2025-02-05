// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/zclconf/go-cty/cty"
)

func TestStripRemovedStateAttributes(t *testing.T) {
	cases := []struct {
		name     string
		state    map[string]interface{}
		expect   map[string]interface{}
		ty       cty.Type
		modified bool
	}{
		{
			"removed string",
			map[string]interface{}{
				"a": "ok",
				"b": "gone",
			},
			map[string]interface{}{
				"a": "ok",
			},
			cty.Object(map[string]cty.Type{
				"a": cty.String,
			}),
			true,
		},
		{
			"removed null",
			map[string]interface{}{
				"a": "ok",
				"b": nil,
			},
			map[string]interface{}{
				"a": "ok",
			},
			cty.Object(map[string]cty.Type{
				"a": cty.String,
			}),
			true,
		},
		{
			"removed nested string",
			map[string]interface{}{
				"a": "ok",
				"b": map[string]interface{}{
					"a": "ok",
					"b": "removed",
				},
			},
			map[string]interface{}{
				"a": "ok",
				"b": map[string]interface{}{
					"a": "ok",
				},
			},
			cty.Object(map[string]cty.Type{
				"a": cty.String,
				"b": cty.Object(map[string]cty.Type{
					"a": cty.String,
				}),
			}),
			true,
		},
		{
			"removed nested list",
			map[string]interface{}{
				"a": "ok",
				"b": map[string]interface{}{
					"a": "ok",
					"b": []interface{}{"removed"},
				},
			},
			map[string]interface{}{
				"a": "ok",
				"b": map[string]interface{}{
					"a": "ok",
				},
			},
			cty.Object(map[string]cty.Type{
				"a": cty.String,
				"b": cty.Object(map[string]cty.Type{
					"a": cty.String,
				}),
			}),
			true,
		},
		{
			"removed keys in set of objs",
			map[string]interface{}{
				"a": "ok",
				"b": map[string]interface{}{
					"a": "ok",
					"set": []interface{}{
						map[string]interface{}{
							"x": "ok",
							"y": "removed",
						},
						map[string]interface{}{
							"x": "ok",
							"y": "removed",
						},
					},
				},
			},
			map[string]interface{}{
				"a": "ok",
				"b": map[string]interface{}{
					"a": "ok",
					"set": []interface{}{
						map[string]interface{}{
							"x": "ok",
						},
						map[string]interface{}{
							"x": "ok",
						},
					},
				},
			},
			cty.Object(map[string]cty.Type{
				"a": cty.String,
				"b": cty.Object(map[string]cty.Type{
					"a": cty.String,
					"set": cty.Set(cty.Object(map[string]cty.Type{
						"x": cty.String,
					})),
				}),
			}),
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modified := removeRemovedAttrs(tc.state, tc.ty)
			if !reflect.DeepEqual(tc.state, tc.expect) {
				t.Fatalf("expected: %#v\n      got: %#v\n", tc.expect, tc.state)
			}
			if modified != tc.modified {
				t.Fatal("incorrect return value")
			}
		})
	}
}

// mustResourceInstanceAddr is a helper to create an absolute resource instance to test moveResourceStateTransform.
// current address foo2_instance.cur and prev address foo_instance.prev
func getMoveStateArgs() stateTransformArgs {
	providerSchemaResponse := &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"foo_instance": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"foo": {
					Type:     cty.String,
					Required: true,
				},
			}),
			"foo2_instance": constructProviderSchemaForTesting(map[string]*configschema.Attribute{
				"foo": {
					Type:     cty.String,
					Required: true,
				},
			}),
		},
	}
	return stateTransformArgs{
		currentAddr: mustResourceInstanceAddr("foo2_instance.cur"),
		prevAddr:    mustResourceInstanceAddr("foo_instance.prev"),
		provider: &MockProvider{
			ConfigureProviderCalled: true,
			MoveResourceStateResponse: &providers.MoveResourceStateResponse{
				TargetState: cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("bar"),
				}),
				TargetPrivate: []byte("private"),
			},
			GetProviderSchemaResponse: providerSchemaResponse,
		},
		objectSrc: &states.ResourceInstanceObjectSrc{
			SchemaVersion: 2,
			AttrsJSON:     []byte(`{"foo":"bar"}`),
			AttrsFlat:     map[string]string{"foo": "bar"},
			Private:       []byte("private"),
		},
		currentSchema:        &configschema.Block{},
		currentSchemaVersion: 3,
	}
}

func TestMoveResourceStateTransform(t *testing.T) {
	type test struct {
		name        string
		args        stateTransformArgs
		wantRequest *providers.MoveResourceStateRequest
		wantState   cty.Value
		wantPrivate []byte
	}

	tests := []test{
		// Make sure that the moveResourceStateTransform function does not change state if the provider does not return a new state
		{
			name: "Move no changes",
			args: getMoveStateArgs(),
			wantState: cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			wantPrivate: []byte("private"),
		},
		// Check that the request is correctly populated
		{
			name: "Move check request",
			args: getMoveStateArgs(),
			wantRequest: &providers.MoveResourceStateRequest{
				SourceProviderAddress: "foo",
				SourceTypeName:        "foo_instance",
				SourceSchemaVersion:   2,
				SourceStateJSON:       []byte(`{"foo":"bar"}`),
				SourceStateFlatmap:    map[string]string{"foo": "bar"},
				SourcePrivate:         []byte("private"),
				TargetTypeName:        "foo2_instance",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotPrivate, diags := moveResourceStateTransform(tt.args)
			if tt.wantRequest != nil {
				mockProvider, ok := tt.args.provider.(*MockProvider)
				if !ok {
					t.Fatalf("unexpected provider type: %T", tt.args.provider)
				}
				if !reflect.DeepEqual(mockProvider.MoveResourceStateRequest, *tt.wantRequest) {
					t.Fatalf("unexpected request: got %+v, want %+v", mockProvider.MoveResourceStateRequest, tt.wantRequest)
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected error: %s", diags.Err())
			}
			if !gotState.RawEquals(tt.wantState) {
				t.Fatalf("unexpected state: got %#v, want %#v", gotState, tt.wantState)
			}
			if !bytes.Equal(gotPrivate, tt.wantPrivate) {
				t.Fatalf("unexpected private: got %#v, want %#v", gotPrivate, tt.wantPrivate)
			}
		})
	}
}

// mustResourceInstanceAddr is a helper to create an absolute resource instance to test upgradeResourceStateTransform.
// current address and the previous address are foo_instance.cur
// schema has field1 and field2, but the json state has field1, field2, and field3
func getUpgradeStateArgs() stateTransformArgs {
	sch := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"field1": {
				Type: cty.String,
			},
			"field2": {
				Type: cty.Bool,
			},
		},
	}
	args := stateTransformArgs{
		currentAddr: mustResourceInstanceAddr("foo_instance.cur"),
		prevAddr:    mustResourceInstanceAddr("foo_instance.cur"),
		provider: &MockProvider{
			ConfigureProviderCalled: true,
			UpgradeResourceStateResponse: &providers.UpgradeResourceStateResponse{
				UpgradedState: cty.ObjectVal(map[string]cty.Value{
					"field1": cty.StringVal("bar"),
					"field2": cty.True,
				}),
			},
			GetProviderSchemaResponse: getProviderSchemaResponseFromProviderSchema(&ProviderSchema{
				ResourceTypes: map[string]*configschema.Block{
					"foo_instance": sch,
				},
			}),
		},
		objectSrc: &states.ResourceInstanceObjectSrc{
			SchemaVersion: 2,
			// Currently json state trimming to match the current schema is done, so field3 should be removed in passed request
			AttrsJSON: []byte(`{"field1":"bar","field2":true,"field3":"baz"}`), // field3 is not in the schema
			AttrsFlat: map[string]string{"foo": "bar"},
			Private:   []byte("private"),
		},
		currentSchema:        sch,
		currentSchemaVersion: 2,
	}
	return args
}

func TestUpgradeResourceStateTransform(t *testing.T) {
	type test struct {
		name        string
		args        stateTransformArgs
		wantRequest *providers.UpgradeResourceStateRequest
		wantState   cty.Value
		wantPrivate []byte
		wantErr     string
	}

	argsForDowngrade := getUpgradeStateArgs()
	argsForDowngrade.currentSchemaVersion = 1

	tests := []test{
		{
			name: "Upgrade basic",
			args: getUpgradeStateArgs(),
			// Checking schema trimming and private state update
			wantState: cty.ObjectVal(map[string]cty.Value{
				"field1": cty.StringVal("bar"),
				"field2": cty.True,
			}),
			wantPrivate: []byte("private"),
		},
		{
			name: "Upgrade check request",
			args: getUpgradeStateArgs(),
			// Checking that field3 is removed from the state before sending the request
			wantRequest: &providers.UpgradeResourceStateRequest{
				TypeName:     "foo_instance",
				Version:      2,
				RawStateJSON: []byte(`{"field1":"bar","field2":true}`),
			},
		},
		{
			name:    "Upgrade to lower version should fail",
			args:    argsForDowngrade,
			wantErr: "Resource instance managed by newer provider version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotPrivate, diags := upgradeResourceStateTransform(tt.args)
			if tt.wantErr != "" {
				if !diags.HasErrors() {
					t.Fatalf("expected error: %s", tt.wantErr)
				}
				if !strings.Contains(diags.Err().Error(), tt.wantErr) {
					t.Fatalf("unexpected error: got %s, want %s", diags.Err(), tt.wantErr)
				}
				return
			}
			if tt.wantRequest != nil {
				mockProvider, ok := tt.args.provider.(*MockProvider)
				if !ok {
					t.Fatalf("unexpected provider type: %T", tt.args.provider)
				}
				if !reflect.DeepEqual(mockProvider.UpgradeResourceStateRequest, *tt.wantRequest) {
					t.Fatalf("unexpected request: got %+v, want %+v", mockProvider.UpgradeResourceStateRequest, tt.wantRequest)
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected error: %s", diags.Err())
			}
			if !gotState.RawEquals(tt.wantState) {
				t.Fatalf("unexpected state: got %#v, want %#v", gotState, tt.wantState)
			}
			if !bytes.Equal(gotPrivate, tt.wantPrivate) {
				t.Fatalf("unexpected private: got %#v, want %#v", gotPrivate, tt.wantPrivate)
			}
		})
	}
}

func getDataResourceModeInstance() addrs.AbsResourceInstance {
	addr := mustResourceInstanceAddr("foo_instance.cur")
	addr.Resource.Resource.Mode = addrs.DataResourceMode
	return addr
}

func TestTransformResourceState(t *testing.T) {
	tests := []struct {
		name           string
		args           stateTransformArgs
		stateTransform providerStateTransform
		wantErr        string
		// Callback to validate the object source after transformation.
		objectSrcValidator func(*testing.T, *states.ResourceInstanceObjectSrc)
	}{
		// We should never have flatmap state in the returned ObjectSrc from transformResourceState (Ensured by objectSrc.CompleteUpgrade)
		{
			name: "flatmap state should be removed after transformation",
			args: stateTransformArgs{
				currentAddr: mustResourceInstanceAddr("test_instance.foo"),
				provider: &MockProvider{
					GetProviderSchemaResponse: testProviderSchema("test"),
				},
				objectSrc: &states.ResourceInstanceObjectSrc{
					AttrsFlat: map[string]string{"foo": "bar"},
					AttrsJSON: []byte(`{"foo":"bar"}`),
				},
				currentSchema: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String},
					},
				},
			},
			stateTransform: func(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics) {
				return cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("bar"),
				}), nil, nil
			},
			objectSrcValidator: func(t *testing.T, got *states.ResourceInstanceObjectSrc) {
				if got.AttrsFlat != nil {
					t.Error("AttrsFlat should be nil after transformation")
				}
			},
		},
		// State must pass TestConformance, i.e. be the same type as the current schema, otherwise expect error
		{
			name: "state must conform to schema",
			args: stateTransformArgs{
				currentAddr: mustResourceInstanceAddr("test_instance.foo"),
				provider: &MockProvider{
					GetProviderSchemaResponse: testProviderSchema("test"),
				},
				objectSrc: &states.ResourceInstanceObjectSrc{
					AttrsJSON: []byte(`{"foo":"bar"}`),
				},
				currentSchema: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"baz": {Type: cty.String}, // different attribute
					},
				},
			},
			stateTransform: func(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics) {
				return cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("bar"), // doesn't match schema
				}), nil, nil
			},
			wantErr: "Invalid resource state transformation",
		},
		// Non-Managed resources should not be upgraded and should return the same object source without errors
		{
			name: "non-managed resource should not be upgraded",
			args: stateTransformArgs{
				currentAddr: getDataResourceModeInstance(),
				objectSrc: &states.ResourceInstanceObjectSrc{
					AttrsJSON: []byte(`{"foo":"bar"}`),
				},
			},
			stateTransform: func(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics) {
				t.Error("stateTransform should not be called for data sources")
				return cty.NilVal, nil, nil
			},
			objectSrcValidator: func(t *testing.T, got *states.ResourceInstanceObjectSrc) {
				if !bytes.Equal(got.AttrsJSON, []byte(`{"foo":"bar"}`)) {
					t.Error("state should remain unchanged")
				}
			},
		},
		// The new private state (returned from the provider/state transform call) should be set in the object source
		{
			name: "private state should be updated",
			args: stateTransformArgs{
				currentAddr: mustResourceInstanceAddr("test_instance.foo"),
				provider: &MockProvider{
					GetProviderSchemaResponse: testProviderSchema("test"),
				},
				objectSrc: &states.ResourceInstanceObjectSrc{
					AttrsJSON: []byte(`{"foo":"bar"}`),
					Private:   []byte("old-private"),
				},
				currentSchema: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String},
					},
				},
			},
			stateTransform: func(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics) {
				return cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("bar"),
				}), []byte("new-private"), nil
			},
			objectSrcValidator: func(t *testing.T, got *states.ResourceInstanceObjectSrc) {
				if !bytes.Equal(got.Private, []byte("new-private")) {
					t.Error("private state not updated")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := transformResourceState(tt.args, tt.stateTransform)

			if tt.wantErr != "" {
				if !diags.HasErrors() {
					t.Fatal("expected error diagnostics")
				}
				if !strings.Contains(diags.Err().Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, diags.Err())
				}
				return
			}

			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.Err())
			}

			if tt.objectSrcValidator != nil {
				tt.objectSrcValidator(t, got)
			}
		})
	}
}
