// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"bytes"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"reflect"
	"testing"

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

func getMoveStateArgs() stateTransformArgs {
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
			GetProviderSchemaResponse: testProviderSchema("foo2"),
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
		{
			name: "Move no changes",
			args: getMoveStateArgs(),
			wantState: cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			wantPrivate: []byte("private"),
		},
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
				if !reflect.DeepEqual(tt.args.provider.(*MockProvider).MoveResourceStateRequest, *tt.wantRequest) {
					t.Fatalf("unexpected request: got %+v, want %+v", tt.args.provider.(*MockProvider).MoveResourceStateRequest, tt.wantRequest)
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
	}

	tests := []test{
		{
			name: "Upgrade basic",
			args: getUpgradeStateArgs(),
			wantState: cty.ObjectVal(map[string]cty.Value{
				"field1": cty.StringVal("bar"),
				"field2": cty.True,
			}),
			wantPrivate: []byte("private"),
		},
		{
			name: "Upgrade check request",
			args: getUpgradeStateArgs(),
			wantRequest: &providers.UpgradeResourceStateRequest{
				TypeName:        "foo_instance",
				Version:         2,
				RawStateJSON:    []byte(`{"field1":"bar","field2":true}`),
				RawStateFlatmap: map[string]string{"foo": "bar"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotPrivate, diags := upgradeResourceStateTransform(tt.args)
			if tt.wantRequest != nil {
				if !reflect.DeepEqual(tt.args.provider.(*MockProvider).UpgradeResourceStateRequest, *tt.wantRequest) {
					t.Fatalf("unexpected request: got %+v, want %+v", tt.args.provider.(*MockProvider).UpgradeResourceStateRequest, tt.wantRequest)
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

// 1) We should never have flatmap state in the returned state from transformResourceState
// 2) State must pass TestConformance, i.e. be the same type as the current schema, otherwise expect error
// 3) If the current version of the schema is higher than the previous version, expect an error "Resource instance managed by newer provider version"
// 4) Non-Managed resources should not be upgraded and should return the same object source without errors
func TestTransformResourceState(t *testing.T) {

}
