// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/legacy/tofu"
)

func TestResourceDataGet(t *testing.T) {
	cases := []struct {
		Schema map[string]*Schema
		State  *tofu.InstanceState
		Diff   *tofu.InstanceDiff
		Key    string
		Value  interface{}
	}{
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "foo",
						New:         "bar",
						NewComputed: true,
					},
				},
			},

			Key:   "availability_zone",
			Value: "",
		},

		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Key: "availability_zone",

			Value: "foo",
		},

		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:      "",
						New:      "foo!",
						NewExtra: "foo",
					},
				},
			},

			Key:   "availability_zone",
			Value: "foo",
		},

		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Required: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Key: "ports.#",

			Value: 0,
		},

		{
			Schema: map[string]*Schema{
				"ingress": &Schema{
					Type:     TypeList,
					Required: true,
					Elem: &Resource{
						Schema: map[string]*Schema{
							"from": &Schema{
								Type:     TypeInt,
								Required: true,
							},
						},
					},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ingress.#": &tofu.ResourceAttrDiff{
						Old: "",
						New: "1",
					},
					"ingress.0.from": &tofu.ResourceAttrDiff{
						Old: "",
						New: "8080",
					},
				},
			},

			Key: "ingress.0",

			Value: map[string]interface{}{
				"from": 8080,
			},
		},

		{
			Schema: map[string]*Schema{
				"ingress": &Schema{
					Type:     TypeList,
					Required: true,
					Elem: &Resource{
						Schema: map[string]*Schema{
							"from": &Schema{
								Type:     TypeInt,
								Required: true,
							},
						},
					},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ingress.#": &tofu.ResourceAttrDiff{
						Old: "",
						New: "1",
					},
					"ingress.0.from": &tofu.ResourceAttrDiff{
						Old: "",
						New: "8080",
					},
				},
			},

			Key: "ingress",

			Value: []interface{}{
				map[string]interface{}{
					"from": 8080,
				},
			},
		},

		// Full object
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Key: "",

			Value: map[string]interface{}{
				"availability_zone": "foo",
			},
		},

		// List of maps
		{
			Schema: map[string]*Schema{
				"config_vars": &Schema{
					Type:     TypeList,
					Optional: true,
					Computed: true,
					Elem: &Schema{
						Type: TypeMap,
					},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"config_vars.#": &tofu.ResourceAttrDiff{
						Old: "0",
						New: "2",
					},
					"config_vars.0.foo": &tofu.ResourceAttrDiff{
						Old: "",
						New: "bar",
					},
					"config_vars.1.bar": &tofu.ResourceAttrDiff{
						Old: "",
						New: "baz",
					},
				},
			},

			Key: "config_vars",

			Value: []interface{}{
				map[string]interface{}{
					"foo": "bar",
				},
				map[string]interface{}{
					"bar": "baz",
				},
			},
		},

		// Empty Set
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
					Set: func(a interface{}) int {
						return a.(int)
					},
				},
			},

			State: nil,

			Diff: nil,

			Key: "ports",

			Value: []interface{}{},
		},

		// Float zero
		{
			Schema: map[string]*Schema{
				"ratio": &Schema{
					Type:     TypeFloat,
					Optional: true,
					Computed: true,
				},
			},

			State: nil,

			Diff: nil,

			Key: "ratio",

			Value: 0.0,
		},
	}

	for i, tc := range cases {
		d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		v := d.Get(tc.Key)
		if s, ok := v.(*Set); ok {
			v = s.List()
		}

		if !reflect.DeepEqual(v, tc.Value) {
			t.Fatalf("Bad: %d\n\n%#v\n\nExpected: %#v", i, v, tc.Value)
		}
	}
}

func TestResourceDataGetChange(t *testing.T) {
	cases := []struct {
		Schema   map[string]*Schema
		State    *tofu.InstanceState
		Diff     *tofu.InstanceDiff
		Key      string
		OldValue interface{}
		NewValue interface{}
	}{
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Key: "availability_zone",

			OldValue: "",
			NewValue: "foo",
		},
	}

	for i, tc := range cases {
		d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		o, n := d.GetChange(tc.Key)
		if !reflect.DeepEqual(o, tc.OldValue) {
			t.Fatalf("Old Bad: %d\n\n%#v", i, o)
		}
		if !reflect.DeepEqual(n, tc.NewValue) {
			t.Fatalf("New Bad: %d\n\n%#v", i, n)
		}
	}
}

func TestResourceDataGetOk(t *testing.T) {
	cases := []struct {
		Schema map[string]*Schema
		State  *tofu.InstanceState
		Diff   *tofu.InstanceDiff
		Key    string
		Value  interface{}
		Ok     bool
	}{
		/*
		 * Primitives
		 */
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old: "",
						New: "",
					},
				},
			},

			Key:   "availability_zone",
			Value: "",
			Ok:    false,
		},

		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "",
						NewComputed: true,
					},
				},
			},

			Key:   "availability_zone",
			Value: "",
			Ok:    false,
		},

		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: "",
			Ok:    false,
		},

		/*
		 * Lists
		 */

		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: []interface{}{},
			Ok:    false,
		},

		/*
		 * Map
		 */

		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeMap,
					Optional: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: map[string]interface{}{},
			Ok:    false,
		},

		/*
		 * Set
		 */

		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
					Set:      func(a interface{}) int { return a.(int) },
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: []interface{}{},
			Ok:    false,
		},

		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
					Set:      func(a interface{}) int { return a.(int) },
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports.0",
			Value: 0,
			Ok:    false,
		},

		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
					Set:      func(a interface{}) int { return a.(int) },
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ports.#": &tofu.ResourceAttrDiff{
						Old: "0",
						New: "0",
					},
				},
			},

			Key:   "ports",
			Value: []interface{}{},
			Ok:    false,
		},

		// Further illustrates and clarifies the GetOk semantics from #933, and
		// highlights the limitation that zero-value config is currently
		// indistinguishable from unset config.
		{
			Schema: map[string]*Schema{
				"from_port": &Schema{
					Type:     TypeInt,
					Optional: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"from_port": &tofu.ResourceAttrDiff{
						Old: "",
						New: "0",
					},
				},
			},

			Key:   "from_port",
			Value: 0,
			Ok:    false,
		},
	}

	for i, tc := range cases {
		d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		v, ok := d.GetOk(tc.Key)
		if s, ok := v.(*Set); ok {
			v = s.List()
		}

		if !reflect.DeepEqual(v, tc.Value) {
			t.Fatalf("Bad: %d\n\n%#v", i, v)
		}
		if ok != tc.Ok {
			t.Fatalf("%d: expected ok: %t, got: %t", i, tc.Ok, ok)
		}
	}
}

func TestResourceDataGetOkExists(t *testing.T) {
	cases := []struct {
		Name   string
		Schema map[string]*Schema
		State  *tofu.InstanceState
		Diff   *tofu.InstanceDiff
		Key    string
		Value  interface{}
		Ok     bool
	}{
		/*
		 * Primitives
		 */
		{
			Name: "string-literal-empty",
			Schema: map[string]*Schema{
				"availability_zone": {
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": {
						Old: "",
						New: "",
					},
				},
			},

			Key:   "availability_zone",
			Value: "",
			Ok:    true,
		},

		{
			Name: "string-computed-empty",
			Schema: map[string]*Schema{
				"availability_zone": {
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": {
						Old:         "",
						New:         "",
						NewComputed: true,
					},
				},
			},

			Key:   "availability_zone",
			Value: "",
			Ok:    false,
		},

		{
			Name: "string-optional-computed-nil-diff",
			Schema: map[string]*Schema{
				"availability_zone": {
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: "",
			Ok:    false,
		},

		/*
		 * Lists
		 */

		{
			Name: "list-optional",
			Schema: map[string]*Schema{
				"ports": {
					Type:     TypeList,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: []interface{}{},
			Ok:    false,
		},

		/*
		 * Map
		 */

		{
			Name: "map-optional",
			Schema: map[string]*Schema{
				"ports": {
					Type:     TypeMap,
					Optional: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: map[string]interface{}{},
			Ok:    false,
		},

		/*
		 * Set
		 */

		{
			Name: "set-optional",
			Schema: map[string]*Schema{
				"ports": {
					Type:     TypeSet,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
					Set:      func(a interface{}) int { return a.(int) },
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: []interface{}{},
			Ok:    false,
		},

		{
			Name: "set-optional-key",
			Schema: map[string]*Schema{
				"ports": {
					Type:     TypeSet,
					Optional: true,
					Elem:     &Schema{Type: TypeInt},
					Set:      func(a interface{}) int { return a.(int) },
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports.0",
			Value: 0,
			Ok:    false,
		},

		{
			Name: "bool-literal-empty",
			Schema: map[string]*Schema{
				"availability_zone": {
					Type:     TypeBool,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,
			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": {
						Old: "",
						New: "",
					},
				},
			},

			Key:   "availability_zone",
			Value: false,
			Ok:    true,
		},

		{
			Name: "bool-literal-set",
			Schema: map[string]*Schema{
				"availability_zone": {
					Type:     TypeBool,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": {
						New: "true",
					},
				},
			},

			Key:   "availability_zone",
			Value: true,
			Ok:    true,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
			if err != nil {
				t.Fatalf("%s err: %s", tc.Name, err)
			}

			v, ok := d.GetOkExists(tc.Key)
			if s, ok := v.(*Set); ok {
				v = s.List()
			}

			if !reflect.DeepEqual(v, tc.Value) {
				t.Fatalf("Bad %s: \n%#v", tc.Name, v)
			}
			if ok != tc.Ok {
				t.Fatalf("%s: expected ok: %t, got: %t", tc.Name, tc.Ok, ok)
			}
		})
	}
}

func TestResourceDataTimeout(t *testing.T) {
	cases := []struct {
		Name     string
		Rd       *ResourceData
		Expected *ResourceTimeout
	}{
		{
			Name:     "Basic example default",
			Rd:       &ResourceData{timeouts: timeoutForValues(10, 3, 0, 15, 0)},
			Expected: expectedTimeoutForValues(10, 3, 0, 15, 0),
		},
		{
			Name:     "Resource and config match update, create",
			Rd:       &ResourceData{timeouts: timeoutForValues(10, 0, 3, 0, 0)},
			Expected: expectedTimeoutForValues(10, 0, 3, 0, 0),
		},
		{
			Name:     "Resource provides default",
			Rd:       &ResourceData{timeouts: timeoutForValues(10, 0, 0, 0, 7)},
			Expected: expectedTimeoutForValues(10, 7, 7, 7, 7),
		},
		{
			Name:     "Resource provides default and delete",
			Rd:       &ResourceData{timeouts: timeoutForValues(10, 0, 0, 15, 7)},
			Expected: expectedTimeoutForValues(10, 7, 7, 15, 7),
		},
		{
			Name:     "Resource provides default, config overwrites other values",
			Rd:       &ResourceData{timeouts: timeoutForValues(10, 3, 0, 0, 13)},
			Expected: expectedTimeoutForValues(10, 3, 13, 13, 13),
		},
		{
			Name:     "Resource has no config",
			Rd:       &ResourceData{},
			Expected: expectedTimeoutForValues(0, 0, 0, 0, 0),
		},
	}

	keys := timeoutKeys()
	for i, c := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, c.Name), func(t *testing.T) {

			for _, k := range keys {
				got := c.Rd.Timeout(k)
				var ex *time.Duration
				switch k {
				case TimeoutCreate:
					ex = c.Expected.Create
				case TimeoutRead:
					ex = c.Expected.Read
				case TimeoutUpdate:
					ex = c.Expected.Update
				case TimeoutDelete:
					ex = c.Expected.Delete
				case TimeoutDefault:
					ex = c.Expected.Default
				}

				if got > 0 && ex == nil {
					t.Fatalf("Unexpected value in (%s), case %d check 1:\n\texpected: %#v\n\tgot: %#v", k, i, ex, got)
				}
				if got == 0 && ex != nil {
					t.Fatalf("Unexpected value in (%s), case %d check 2:\n\texpected: %#v\n\tgot: %#v", k, i, *ex, got)
				}

				// confirm values
				if ex != nil {
					if got != *ex {
						t.Fatalf("Timeout %s case (%d) expected (%s), got (%s)", k, i, *ex, got)
					}
				}
			}

		})
	}
}

func TestResourceDataHasChange(t *testing.T) {
	cases := []struct {
		Schema map[string]*Schema
		State  *tofu.InstanceState
		Diff   *tofu.InstanceDiff
		Key    string
		Change bool
	}{
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Key: "availability_zone",

			Change: true,
		},

		{
			Schema: map[string]*Schema{
				"tags": &Schema{
					Type:     TypeMap,
					Optional: true,
					Computed: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"tags.Name": &tofu.ResourceAttrDiff{
						Old: "foo",
						New: "foo",
					},
				},
			},

			Key: "tags",

			Change: true,
		},
	}

	for i, tc := range cases {
		d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		actual := d.HasChange(tc.Key)
		if actual != tc.Change {
			t.Fatalf("Bad: %d %#v", i, actual)
		}
	}
}

func TestResourceDataSet(t *testing.T) {
	var testNilPtr *string

	cases := []struct {
		TestName string
		Schema   map[string]*Schema
		State    *tofu.InstanceState
		Diff     *tofu.InstanceDiff
		Key      string
		Value    interface{}
		Err      bool
		GetKey   string
		GetValue interface{}

		// GetPreProcess can be set to munge the return value before being
		// compared to GetValue
		GetPreProcess func(interface{}) interface{}
	}{
		{
			TestName: "Basic good",
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: "foo",

			GetKey:   "availability_zone",
			GetValue: "foo",
		},
		{
			TestName: "Basic int",
			Schema: map[string]*Schema{
				"port": &Schema{
					Type:     TypeInt,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "port",
			Value: 80,

			GetKey:   "port",
			GetValue: 80,
		},
		{
			TestName: "Basic bool, true",
			Schema: map[string]*Schema{
				"vpc": &Schema{
					Type:     TypeBool,
					Optional: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "vpc",
			Value: true,

			GetKey:   "vpc",
			GetValue: true,
		},
		{
			TestName: "Basic bool, false",
			Schema: map[string]*Schema{
				"vpc": &Schema{
					Type:     TypeBool,
					Optional: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "vpc",
			Value: false,

			GetKey:   "vpc",
			GetValue: false,
		},
		{
			TestName: "Invalid type",
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: 80,
			Err:   true,

			GetKey:   "availability_zone",
			GetValue: "",
		},
		{
			TestName: "List of primitives, set list",
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: []int{1, 2, 5},

			GetKey:   "ports",
			GetValue: []interface{}{1, 2, 5},
		},
		{
			TestName: "List of primitives, set list with error",
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ports",
			Value: []interface{}{1, "NOPE", 5},
			Err:   true,

			GetKey:   "ports",
			GetValue: []interface{}{},
		},
		{
			TestName: "Set a list of maps",
			Schema: map[string]*Schema{
				"config_vars": &Schema{
					Type:     TypeList,
					Optional: true,
					Computed: true,
					Elem: &Schema{
						Type: TypeMap,
					},
				},
			},

			State: nil,

			Diff: nil,

			Key: "config_vars",
			Value: []interface{}{
				map[string]interface{}{
					"foo": "bar",
				},
				map[string]interface{}{
					"bar": "baz",
				},
			},
			Err: false,

			GetKey: "config_vars",
			GetValue: []interface{}{
				map[string]interface{}{
					"foo": "bar",
				},
				map[string]interface{}{
					"bar": "baz",
				},
			},
		},
		{
			TestName: "Set with nested set",
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type: TypeSet,
					Elem: &Resource{
						Schema: map[string]*Schema{
							"port": &Schema{
								Type: TypeInt,
							},

							"set": &Schema{
								Type: TypeSet,
								Elem: &Schema{Type: TypeInt},
								Set: func(a interface{}) int {
									return a.(int)
								},
							},
						},
					},
					Set: func(a interface{}) int {
						return a.(map[string]interface{})["port"].(int)
					},
				},
			},

			State: nil,

			Key: "ports",
			Value: []interface{}{
				map[string]interface{}{
					"port": 80,
				},
			},

			GetKey: "ports",
			GetValue: []interface{}{
				map[string]interface{}{
					"port": 80,
					"set":  []interface{}{},
				},
			},

			GetPreProcess: func(v interface{}) interface{} {
				if v == nil {
					return v
				}
				s, ok := v.([]interface{})
				if !ok {
					return v
				}
				for _, v := range s {
					m, ok := v.(map[string]interface{})
					if !ok {
						continue
					}
					if m["set"] == nil {
						continue
					}
					if s, ok := m["set"].(*Set); ok {
						m["set"] = s.List()
					}
				}

				return v
			},
		},
		{
			TestName: "List of floats, set list",
			Schema: map[string]*Schema{
				"ratios": &Schema{
					Type:     TypeList,
					Computed: true,
					Elem:     &Schema{Type: TypeFloat},
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ratios",
			Value: []float64{1.0, 2.2, 5.5},

			GetKey:   "ratios",
			GetValue: []interface{}{1.0, 2.2, 5.5},
		},
		{
			TestName: "Set of floats, set list",
			Schema: map[string]*Schema{
				"ratios": &Schema{
					Type:     TypeSet,
					Computed: true,
					Elem:     &Schema{Type: TypeFloat},
					Set: func(a interface{}) int {
						// Because we want to be safe on a 32-bit and 64-bit system,
						// we can just set a "scale factor" here that's always larger than the number of
						// decimal places we expect to see., and then multiply by that to cast to int
						// otherwise we could get clashes in unique ids
						scaleFactor := 100000
						return int(a.(float64) * float64(scaleFactor))
					},
				},
			},

			State: nil,

			Diff: nil,

			Key:   "ratios",
			Value: []float64{1.0, 2.2, 5.5},

			GetKey:   "ratios",
			GetValue: []interface{}{1.0, 2.2, 5.5},
		},
		{
			TestName: "Basic pointer",
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: testPtrTo("foo"),

			GetKey:   "availability_zone",
			GetValue: "foo",
		},
		{
			TestName: "Basic nil value",
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: testPtrTo(nil),

			GetKey:   "availability_zone",
			GetValue: "",
		},
		{
			TestName: "Basic nil pointer",
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: nil,

			Key:   "availability_zone",
			Value: testNilPtr,

			GetKey:   "availability_zone",
			GetValue: "",
		},
		{
			TestName: "Set in a list",
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type: TypeList,
					Elem: &Resource{
						Schema: map[string]*Schema{
							"set": &Schema{
								Type: TypeSet,
								Elem: &Schema{Type: TypeInt},
								Set: func(a interface{}) int {
									return a.(int)
								},
							},
						},
					},
				},
			},

			State: nil,

			Key: "ports",
			Value: []interface{}{
				map[string]interface{}{
					"set": []interface{}{
						1,
					},
				},
			},

			GetKey: "ports",
			GetValue: []interface{}{
				map[string]interface{}{
					"set": []interface{}{
						1,
					},
				},
			},
			GetPreProcess: func(v interface{}) interface{} {
				if v == nil {
					return v
				}
				s, ok := v.([]interface{})
				if !ok {
					return v
				}
				for _, v := range s {
					m, ok := v.(map[string]interface{})
					if !ok {
						continue
					}
					if m["set"] == nil {
						continue
					}
					if s, ok := m["set"].(*Set); ok {
						m["set"] = s.List()
					}
				}

				return v
			},
		},
	}

	t.Setenv(PanicOnErr, "")

	for _, tc := range cases {
		t.Run(tc.TestName, func(t *testing.T) {
			d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
			if err != nil {
				t.Fatalf("err: %s", err)
			}

			err = d.Set(tc.Key, tc.Value)
			if err != nil != tc.Err {
				t.Fatalf("unexpected err: %s", err)
			}

			v := d.Get(tc.GetKey)
			if s, ok := v.(*Set); ok {
				v = s.List()
			}

			if tc.GetPreProcess != nil {
				v = tc.GetPreProcess(v)
			}

			if !reflect.DeepEqual(v, tc.GetValue) {
				t.Fatalf("Got unexpected value\nactual: %#v\nexpected:%#v", v, tc.GetValue)
			}
		})
	}
}

func TestResourceDataState_dynamicAttributes(t *testing.T) {
	cases := []struct {
		Schema    map[string]*Schema
		State     *tofu.InstanceState
		Diff      *tofu.InstanceDiff
		Set       map[string]interface{}
		UnsafeSet map[string]string
		Result    *tofu.InstanceState
	}{
		{
			Schema: map[string]*Schema{
				"__has_dynamic_attributes": {
					Type:     TypeString,
					Optional: true,
				},

				"schema_field": {
					Type:     TypeString,
					Required: true,
				},
			},

			State: nil,

			Diff: nil,

			Set: map[string]interface{}{
				"schema_field": "present",
			},

			UnsafeSet: map[string]string{
				"test1": "value",
				"test2": "value",
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"schema_field": "present",
					"test1":        "value",
					"test2":        "value",
				},
			},
		},
	}

	for i, tc := range cases {
		d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		for k, v := range tc.Set {
			d.Set(k, v)
		}

		for k, v := range tc.UnsafeSet {
			d.UnsafeSetFieldRaw(k, v)
		}

		// Set an ID so that the state returned is not nil
		idSet := false
		if d.Id() == "" {
			idSet = true
			d.SetId("foo")
		}

		actual := d.State()

		// If we set an ID, then undo what we did so the comparison works
		if actual != nil && idSet {
			actual.ID = ""
			delete(actual.Attributes, "id")
		}

		if !reflect.DeepEqual(actual, tc.Result) {
			t.Fatalf("Bad: %d\n\n%#v\n\nExpected:\n\n%#v", i, actual, tc.Result)
		}
	}
}

func TestResourceDataState_schema(t *testing.T) {
	cases := []struct {
		Schema  map[string]*Schema
		State   *tofu.InstanceState
		Diff    *tofu.InstanceDiff
		Set     map[string]interface{}
		Result  *tofu.InstanceState
		Partial []string
	}{
		// #0 Basic primitive in diff
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"availability_zone": "foo",
				},
			},
		},

		// #1 Basic primitive set override
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Set: map[string]interface{}{
				"availability_zone": "bar",
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"availability_zone": "bar",
				},
			},
		},

		// #2
		{
			Schema: map[string]*Schema{
				"vpc": &Schema{
					Type:     TypeBool,
					Optional: true,
				},
			},

			State: nil,

			Diff: nil,

			Set: map[string]interface{}{
				"vpc": true,
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"vpc": "true",
				},
			},
		},

		// #3 Basic primitive with StateFunc set
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:      TypeString,
					Optional:  true,
					Computed:  true,
					StateFunc: func(interface{}) string { return "" },
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:      "",
						New:      "foo",
						NewExtra: "foo!",
					},
				},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"availability_zone": "foo",
				},
			},
		},

		// #10
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
					Set: func(a interface{}) int {
						return a.(int)
					},
				},
			},

			State: nil,

			Diff: nil,

			Set: map[string]interface{}{
				"ports": []interface{}{100, 80},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"ports.#":   "2",
					"ports.80":  "80",
					"ports.100": "100",
				},
			},
		},

		/*
		 * PARTIAL STATES
		 */

		// #12 Basic primitive
		{
			Schema: map[string]*Schema{
				"availability_zone": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
					ForceNew: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"availability_zone": &tofu.ResourceAttrDiff{
						Old:         "",
						New:         "foo",
						RequiresNew: true,
					},
				},
			},

			Partial: []string{},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{},
			},
		},

		// #14
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Optional: true,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ports.#": &tofu.ResourceAttrDiff{
						Old:         "",
						NewComputed: true,
					},
				},
			},

			Partial: []string{},

			Set: map[string]interface{}{
				"ports": []interface{}{},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{},
			},
		},

		// #18
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
					Set: func(a interface{}) int {
						return a.(int)
					},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ports.#": &tofu.ResourceAttrDiff{
						Old:         "",
						NewComputed: true,
					},
				},
			},

			Partial: []string{},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{},
			},
		},

		// #19 Maps
		{
			Schema: map[string]*Schema{
				"tags": &Schema{
					Type:     TypeMap,
					Optional: true,
					Computed: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"tags.Name": &tofu.ResourceAttrDiff{
						Old: "",
						New: "foo",
					},
				},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"tags.%":    "1",
					"tags.Name": "foo",
				},
			},
		},

		// #20 empty computed map
		{
			Schema: map[string]*Schema{
				"tags": &Schema{
					Type:     TypeMap,
					Optional: true,
					Computed: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"tags.Name": &tofu.ResourceAttrDiff{
						Old: "",
						New: "foo",
					},
				},
			},

			Set: map[string]interface{}{
				"tags": map[string]string{},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"tags.%": "0",
				},
			},
		},

		// #21
		{
			Schema: map[string]*Schema{
				"foo": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"foo": &tofu.ResourceAttrDiff{
						NewComputed: true,
					},
				},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{},
			},
		},

		// #22
		{
			Schema: map[string]*Schema{
				"foo": &Schema{
					Type:     TypeString,
					Optional: true,
					Computed: true,
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"foo": &tofu.ResourceAttrDiff{
						NewComputed: true,
					},
				},
			},

			Set: map[string]interface{}{
				"foo": "bar",
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"foo": "bar",
				},
			},
		},

		// #23 Set of maps
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Computed: true,
					Elem: &Resource{
						Schema: map[string]*Schema{
							"index": &Schema{Type: TypeInt},
							"uuids": &Schema{Type: TypeMap},
						},
					},
					Set: func(a interface{}) int {
						m := a.(map[string]interface{})
						return m["index"].(int)
					},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ports.10.uuids.#": &tofu.ResourceAttrDiff{
						NewComputed: true,
					},
				},
			},

			Set: map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"index": 10,
						"uuids": map[string]interface{}{
							"80": "value",
						},
					},
				},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"ports.#":           "1",
					"ports.10.index":    "10",
					"ports.10.uuids.%":  "1",
					"ports.10.uuids.80": "value",
				},
			},
		},

		// #25
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeSet,
					Optional: true,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
					Set: func(a interface{}) int {
						return a.(int)
					},
				},
			},

			State: nil,

			Diff: nil,

			Set: map[string]interface{}{
				"ports": []interface{}{},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"ports.#": "0",
				},
			},
		},

		// #26
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Optional: true,
					Computed: true,
					Elem:     &Schema{Type: TypeInt},
				},
			},

			State: nil,

			Diff: nil,

			Set: map[string]interface{}{
				"ports": []interface{}{},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"ports.#": "0",
				},
			},
		},

		// #27 Set lists
		{
			Schema: map[string]*Schema{
				"ports": &Schema{
					Type:     TypeList,
					Optional: true,
					Computed: true,
					Elem: &Resource{
						Schema: map[string]*Schema{
							"index": &Schema{Type: TypeInt},
							"uuids": &Schema{Type: TypeMap},
						},
					},
				},
			},

			State: nil,

			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"ports.#": &tofu.ResourceAttrDiff{
						NewComputed: true,
					},
				},
			},

			Set: map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"index": 10,
						"uuids": map[string]interface{}{
							"80": "value",
						},
					},
				},
			},

			Result: &tofu.InstanceState{
				Attributes: map[string]string{
					"ports.#":          "1",
					"ports.0.index":    "10",
					"ports.0.uuids.%":  "1",
					"ports.0.uuids.80": "value",
				},
			},
		},
	}

	for i, tc := range cases {
		d, err := schemaMap(tc.Schema).Data(tc.State, tc.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		for k, v := range tc.Set {
			if err := d.Set(k, v); err != nil {
				t.Fatalf("%d err: %s", i, err)
			}
		}

		// Set an ID so that the state returned is not nil
		idSet := false
		if d.Id() == "" {
			idSet = true
			d.SetId("foo")
		}

		// If we have partial, then enable partial state mode.
		if tc.Partial != nil {
			d.Partial(true)
			for _, k := range tc.Partial {
				d.SetPartial(k)
			}
		}

		actual := d.State()

		// If we set an ID, then undo what we did so the comparison works
		if actual != nil && idSet {
			actual.ID = ""
			delete(actual.Attributes, "id")
		}

		if !reflect.DeepEqual(actual, tc.Result) {
			t.Fatalf("Bad: %d\n\n%#v\n\nExpected:\n\n%#v", i, actual, tc.Result)
		}
	}
}

func TestResourceData_nonStringValuesInMap(t *testing.T) {
	cases := []struct {
		Schema       map[string]*Schema
		Diff         *tofu.InstanceDiff
		MapFieldName string
		ItemName     string
		ExpectedType string
	}{
		{
			Schema: map[string]*Schema{
				"boolMap": &Schema{
					Type:     TypeMap,
					Elem:     TypeBool,
					Optional: true,
				},
			},
			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"boolMap.%": &tofu.ResourceAttrDiff{
						Old: "",
						New: "1",
					},
					"boolMap.boolField": &tofu.ResourceAttrDiff{
						Old: "",
						New: "true",
					},
				},
			},
			MapFieldName: "boolMap",
			ItemName:     "boolField",
			ExpectedType: "bool",
		},
		{
			Schema: map[string]*Schema{
				"intMap": &Schema{
					Type:     TypeMap,
					Elem:     TypeInt,
					Optional: true,
				},
			},
			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"intMap.%": &tofu.ResourceAttrDiff{
						Old: "",
						New: "1",
					},
					"intMap.intField": &tofu.ResourceAttrDiff{
						Old: "",
						New: "8",
					},
				},
			},
			MapFieldName: "intMap",
			ItemName:     "intField",
			ExpectedType: "int",
		},
		{
			Schema: map[string]*Schema{
				"floatMap": &Schema{
					Type:     TypeMap,
					Elem:     TypeFloat,
					Optional: true,
				},
			},
			Diff: &tofu.InstanceDiff{
				Attributes: map[string]*tofu.ResourceAttrDiff{
					"floatMap.%": &tofu.ResourceAttrDiff{
						Old: "",
						New: "1",
					},
					"floatMap.floatField": &tofu.ResourceAttrDiff{
						Old: "",
						New: "8.22",
					},
				},
			},
			MapFieldName: "floatMap",
			ItemName:     "floatField",
			ExpectedType: "float64",
		},
	}

	for _, c := range cases {
		d, err := schemaMap(c.Schema).Data(nil, c.Diff)
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		m, ok := d.Get(c.MapFieldName).(map[string]interface{})
		if !ok {
			t.Fatalf("expected %q to be castable to a map", c.MapFieldName)
		}
		field, ok := m[c.ItemName]
		if !ok {
			t.Fatalf("expected %q in the map", c.ItemName)
		}

		typeName := reflect.TypeOf(field).Name()
		if typeName != c.ExpectedType {
			t.Fatalf("expected %q to be %q, it is %q.",
				c.ItemName, c.ExpectedType, typeName)
		}
	}
}

func TestResourceDataSetConnInfo(t *testing.T) {
	d := &ResourceData{}
	d.SetId("foo")
	d.SetConnInfo(map[string]string{
		"foo": "bar",
	})

	expected := map[string]string{
		"foo": "bar",
	}

	actual := d.State()
	if !reflect.DeepEqual(actual.Ephemeral.ConnInfo, expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestResourceDataSetMeta_Timeouts(t *testing.T) {
	d := &ResourceData{}
	d.SetId("foo")

	rt := ResourceTimeout{
		Create: DefaultTimeout(7 * time.Minute),
	}

	d.timeouts = &rt

	expected := expectedForValues(7, 0, 0, 0, 0)

	actual := d.State()
	if !reflect.DeepEqual(actual.Meta[TimeoutKey], expected) {
		t.Fatalf("Bad Meta_timeout match:\n\texpected: %#v\n\tgot: %#v", expected, actual.Meta[TimeoutKey])
	}
}

func TestResourceDataSetType(t *testing.T) {
	d := &ResourceData{}
	d.SetId("foo")
	d.SetType("bar")

	actual := d.State()
	if v := actual.Ephemeral.Type; v != "bar" {
		t.Fatalf("bad: %#v", actual)
	}
}

func testPtrTo(raw interface{}) interface{} {
	return &raw
}
