// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/legacy/hcl2shim"
	"github.com/opentofu/opentofu/internal/legacy/tofu"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

var (
	typeComparer  = cmp.Comparer(cty.Type.Equals)
	valueComparer = cmp.Comparer(cty.Value.RawEquals)
	equateEmpty   = cmpopts.EquateEmpty()
)

func testApplyDiff(t *testing.T,
	resource *Resource,
	state, expected *tofu.InstanceState,
	diff *tofu.InstanceDiff) {

	testSchema := providers.Schema{
		Version: int64(resource.SchemaVersion),
		Block:   resourceSchemaToBlock(resource.Schema),
	}

	stateVal, err := StateValueFromInstanceState(state, testSchema.Block.ImpliedType())
	if err != nil {
		t.Fatal(err)
	}

	newState, err := ApplyDiff(stateVal, diff, testSchema.Block)
	if err != nil {
		t.Fatal(err)
	}

	// verify that "id" is correct
	id := newState.AsValueMap()["id"]

	switch {
	case diff.Destroy || diff.DestroyDeposed || diff.DestroyTainted:
		// there should be no id
		if !id.IsNull() {
			t.Fatalf("destroyed instance should have no id: %#v", id)
		}
	default:
		// the "id" field always exists and is computed, so it must have a
		// valid value or be unknown.
		if id.IsNull() {
			t.Fatal("new instance state cannot have a null id")
		}

		if id.IsKnown() && id.AsString() == "" {
			t.Fatal("new instance id cannot be an empty string")
		}
	}

	// Resource.Meta will be handled separately, so it's OK that we lose the
	// timeout values here.
	expectedState, err := StateValueFromInstanceState(expected, testSchema.Block.ImpliedType())
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(expectedState, newState, equateEmpty, typeComparer, valueComparer) {
		t.Fatalf("state diff (-expected +got):\n%s", cmp.Diff(expectedState, newState, equateEmpty, typeComparer, valueComparer))
	}
}

func TestShimResourcePlan_destroyCreate(t *testing.T) {
	r := &Resource{
		SchemaVersion: 2,
		Schema: map[string]*Schema{
			"foo": &Schema{
				Type:     TypeInt,
				Optional: true,
				ForceNew: true,
			},
		},
	}

	d := &tofu.InstanceDiff{
		Attributes: map[string]*tofu.ResourceAttrDiff{
			"foo": &tofu.ResourceAttrDiff{
				RequiresNew: true,
				Old:         "3",
				New:         "42",
			},
		},
	}

	state := &tofu.InstanceState{
		Attributes: map[string]string{"foo": "3"},
	}

	expected := &tofu.InstanceState{
		ID: hcl2shim.UnknownVariableValue,
		Attributes: map[string]string{
			"id":  hcl2shim.UnknownVariableValue,
			"foo": "42",
		},
		Meta: map[string]interface{}{
			"schema_version": "2",
		},
	}

	testApplyDiff(t, r, state, expected, d)
}

func resourceSchemaToBlock(s map[string]*Schema) *configschema.Block {
	return (&Resource{Schema: s}).CoreConfigSchema()
}

func TestRemoveConfigUnknowns(t *testing.T) {
	cfg := map[string]interface{}{
		"id": "74D93920-ED26-11E3-AC10-0800200C9A66",
		"route_rules": []interface{}{
			map[string]interface{}{
				"cidr_block":        "74D93920-ED26-11E3-AC10-0800200C9A66",
				"destination":       "0.0.0.0/0",
				"destination_type":  "CIDR_BLOCK",
				"network_entity_id": "1",
			},
			map[string]interface{}{
				"cidr_block":       "74D93920-ED26-11E3-AC10-0800200C9A66",
				"destination":      "0.0.0.0/0",
				"destination_type": "CIDR_BLOCK",
				"sub_block": []interface{}{
					map[string]interface{}{
						"computed": "74D93920-ED26-11E3-AC10-0800200C9A66",
					},
				},
			},
		},
	}

	expect := map[string]interface{}{
		"route_rules": []interface{}{
			map[string]interface{}{
				"destination":       "0.0.0.0/0",
				"destination_type":  "CIDR_BLOCK",
				"network_entity_id": "1",
			},
			map[string]interface{}{
				"destination":      "0.0.0.0/0",
				"destination_type": "CIDR_BLOCK",
				"sub_block": []interface{}{
					map[string]interface{}{},
				},
			},
		},
	}

	removeConfigUnknowns(cfg)

	if !reflect.DeepEqual(cfg, expect) {
		t.Fatalf("\nexpected: %#v\ngot:      %#v", expect, cfg)
	}
}
