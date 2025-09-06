// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/legacy/hcl2shim"
	"github.com/opentofu/opentofu/internal/legacy/tofu"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func TestResourceDiff_Timeout_diff(t *testing.T) {
	r := &Resource{
		Schema: map[string]*Schema{
			"foo": &Schema{
				Type:     TypeInt,
				Optional: true,
			},
		},
		Timeouts: &ResourceTimeout{
			Create: DefaultTimeout(40 * time.Minute),
			Update: DefaultTimeout(80 * time.Minute),
			Delete: DefaultTimeout(40 * time.Minute),
		},
	}

	r.Create = func(d *ResourceData, m interface{}) error {
		d.SetId("foo")
		return nil
	}

	conf := tofu.NewResourceConfigRaw(
		map[string]interface{}{
			"foo": 42,
			TimeoutsConfigKey: map[string]interface{}{
				"create": "2h",
			},
		},
	)
	var s *tofu.InstanceState

	actual, err := r.Diff(s, conf, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := &tofu.InstanceDiff{
		Attributes: map[string]*tofu.ResourceAttrDiff{
			"foo": &tofu.ResourceAttrDiff{
				New: "42",
			},
		},
	}

	diffTimeout := &ResourceTimeout{
		Create: DefaultTimeout(120 * time.Minute),
		Update: DefaultTimeout(80 * time.Minute),
		Delete: DefaultTimeout(40 * time.Minute),
	}

	if err := diffTimeout.DiffEncode(expected); err != nil {
		t.Fatalf("Error encoding timeout to diff: %s", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Not equal Meta in Timeout Diff:\n\texpected: %#v\n\tactual: %#v", expected.Meta, actual.Meta)
	}
}

func TestResourceInternalValidate(t *testing.T) {
	cases := []struct {
		In       *Resource
		Writable bool
		Err      bool
	}{
		0: {
			nil,
			true,
			true,
		},

		// No optional and no required
		1: {
			&Resource{
				Schema: map[string]*Schema{
					"foo": &Schema{
						Type:     TypeInt,
						Optional: true,
						Required: true,
					},
				},
			},
			true,
			true,
		},

		// Update undefined for non-ForceNew field
		2: {
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"boo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
			},
			true,
			true,
		},

		// Update defined for ForceNew field
		3: {
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
						ForceNew: true,
					},
				},
			},
			true,
			true,
		},

		// non-writable doesn't need Update, Create or Delete
		4: {
			&Resource{
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
			},
			false,
			false,
		},

		// non-writable *must not* have Create
		5: {
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
			},
			false,
			true,
		},

		// writable must have Read
		6: {
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Delete: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
			},
			true,
			true,
		},

		// writable must have Delete
		7: {
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Read:   func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
			},
			true,
			true,
		},

		8: { // Reserved name at root should be disallowed
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Read:   func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Delete: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"count": {
						Type:     TypeInt,
						Optional: true,
					},
				},
			},
			true,
			true,
		},

		9: { // Reserved name at nested levels should be allowed
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Read:   func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Delete: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"parent_list": &Schema{
						Type:     TypeString,
						Optional: true,
						Elem: &Resource{
							Schema: map[string]*Schema{
								"provisioner": {
									Type:     TypeString,
									Optional: true,
								},
							},
						},
					},
				},
			},
			true,
			false,
		},

		10: { // Provider reserved name should be allowed in resource
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Read:   func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Delete: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"alias": &Schema{
						Type:     TypeString,
						Optional: true,
					},
				},
			},
			true,
			false,
		},

		11: { // ID should be allowed in data source
			&Resource{
				Read: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"id": &Schema{
						Type:     TypeString,
						Optional: true,
					},
				},
			},
			false,
			false,
		},

		12: { // Deprecated ID should be allowed in resource
			&Resource{
				Create: func(d *ResourceData, meta interface{}) error { return nil },
				Read:   func(d *ResourceData, meta interface{}) error { return nil },
				Update: func(d *ResourceData, meta interface{}) error { return nil },
				Delete: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"id": &Schema{
						Type:       TypeString,
						Optional:   true,
						Deprecated: "Use x_id instead",
					},
				},
			},
			true,
			false,
		},

		13: { // non-writable must not define CustomizeDiff
			&Resource{
				Read: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
				CustomizeDiff: func(*ResourceDiff, interface{}) error { return nil },
			},
			false,
			true,
		},
		14: { // Deprecated resource
			&Resource{
				Read: func(d *ResourceData, meta interface{}) error { return nil },
				Schema: map[string]*Schema{
					"goo": &Schema{
						Type:     TypeInt,
						Optional: true,
					},
				},
				DeprecationMessage: "This resource has been deprecated.",
			},
			true,
			true,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			sm := schemaMap{}
			if tc.In != nil {
				sm = schemaMap(tc.In.Schema)
			}

			err := tc.In.InternalValidate(sm, tc.Writable)
			if err != nil && !tc.Err {
				t.Fatalf("%d: expected validation to pass: %s", i, err)
			}
			if err == nil && tc.Err {
				t.Fatalf("%d: expected validation to fail", i)
			}
		})
	}
}

func TestResource_UpgradeState(t *testing.T) {
	// While this really only calls itself and therefore doesn't test any of
	// the Resource code directly, it still serves as an example of registering
	// a StateUpgrader.
	r := &Resource{
		SchemaVersion: 2,
		Schema: map[string]*Schema{
			"newfoo": &Schema{
				Type:     TypeInt,
				Optional: true,
			},
		},
	}

	r.StateUpgraders = []StateUpgrader{
		{
			Version: 1,
			Type: cty.Object(map[string]cty.Type{
				"id":     cty.String,
				"oldfoo": cty.Number,
			}),
			Upgrade: func(m map[string]interface{}, meta interface{}) (map[string]interface{}, error) {

				oldfoo, ok := m["oldfoo"].(float64)
				if !ok {
					t.Fatalf("expected 1.2, got %#v", m["oldfoo"])
				}
				m["newfoo"] = int(oldfoo * 10)
				delete(m, "oldfoo")

				return m, nil
			},
		},
	}

	oldStateAttrs := map[string]string{
		"id":     "bar",
		"oldfoo": "1.2",
	}

	// convert the legacy flatmap state to the json equivalent
	ty := r.StateUpgraders[0].Type
	val, err := hcl2shim.HCL2ValueFromFlatmap(oldStateAttrs, ty)
	if err != nil {
		t.Fatal(err)
	}
	js, err := ctyjson.Marshal(val, ty)
	if err != nil {
		t.Fatal(err)
	}

	// unmarshal the state using the json default types
	var m map[string]interface{}
	if err := json.Unmarshal(js, &m); err != nil {
		t.Fatal(err)
	}

	actual, err := r.StateUpgraders[0].Upgrade(m, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := map[string]interface{}{
		"id":     "bar",
		"newfoo": 12,
	}

	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected: %#v\ngot: %#v\n", expected, actual)
	}
}

func TestResource_ValidateUpgradeState(t *testing.T) {
	r := &Resource{
		SchemaVersion: 3,
		Schema: map[string]*Schema{
			"newfoo": &Schema{
				Type:     TypeInt,
				Optional: true,
			},
		},
	}

	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatal(err)
	}

	r.StateUpgraders = append(r.StateUpgraders, StateUpgrader{
		Version: 2,
		Type: cty.Object(map[string]cty.Type{
			"id": cty.String,
		}),
		Upgrade: func(m map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
			return m, nil
		},
	})
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatal(err)
	}

	// check for missing type
	r.StateUpgraders[0].Type = cty.Type{}
	if err := r.InternalValidate(nil, true); err == nil {
		t.Fatal("StateUpgrader must have type")
	}
	r.StateUpgraders[0].Type = cty.Object(map[string]cty.Type{
		"id": cty.String,
	})

	// check for missing Upgrade func
	r.StateUpgraders[0].Upgrade = nil
	if err := r.InternalValidate(nil, true); err == nil {
		t.Fatal("StateUpgrader must have an Upgrade func")
	}
	r.StateUpgraders[0].Upgrade = func(m map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
		return m, nil
	}

	// check for skipped version
	r.StateUpgraders[0].Version = 0
	r.StateUpgraders = append(r.StateUpgraders, StateUpgrader{
		Version: 2,
		Type: cty.Object(map[string]cty.Type{
			"id": cty.String,
		}),
		Upgrade: func(m map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
			return m, nil
		},
	})
	if err := r.InternalValidate(nil, true); err == nil {
		t.Fatal("StateUpgraders cannot skip versions")
	}

	// add the missing version, but fail because it's still out of order
	r.StateUpgraders = append(r.StateUpgraders, StateUpgrader{
		Version: 1,
		Type: cty.Object(map[string]cty.Type{
			"id": cty.String,
		}),
		Upgrade: func(m map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
			return m, nil
		},
	})
	if err := r.InternalValidate(nil, true); err == nil {
		t.Fatal("upgraders must be defined in order")
	}

	r.StateUpgraders[1], r.StateUpgraders[2] = r.StateUpgraders[2], r.StateUpgraders[1]
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatal(err)
	}

	// can't add an upgrader for a schema >= the current version
	r.StateUpgraders = append(r.StateUpgraders, StateUpgrader{
		Version: 3,
		Type: cty.Object(map[string]cty.Type{
			"id": cty.String,
		}),
		Upgrade: func(m map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
			return m, nil
		},
	})
	if err := r.InternalValidate(nil, true); err == nil {
		t.Fatal("StateUpgraders cannot have a version >= current SchemaVersion")
	}
}
