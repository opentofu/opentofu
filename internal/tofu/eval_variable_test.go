// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestPrepareFinalInputVariableValue(t *testing.T) {
	// This is just a concise way to define a bunch of *configs.Variable
	// objects to use in our tests below. We're only going to decode this
	// config, not fully evaluate it.
	cfgSrc := `
		variable "nullable_required" {
		}
		variable "nullable_optional_default_string" {
			default = "hello"
		}
		variable "nullable_optional_default_null" {
			default = null
		}
		variable "constrained_string_nullable_required" {
			type = string
		}
		variable "constrained_string_nullable_optional_default_string" {
			type    = string
			default = "hello"
		}
		variable "constrained_string_nullable_optional_default_bool" {
			type    = string
			default = true
		}
		variable "constrained_string_nullable_optional_default_null" {
			type    = string
			default = null
		}
		variable "required" {
			nullable = false
		}
		variable "optional_default_string" {
			nullable = false
			default  = "hello"
		}
		variable "constrained_string_required" {
			nullable = false
			type     = string
		}
		variable "constrained_string_optional_default_string" {
			nullable = false
			type     = string
			default  = "hello"
		}
		variable "constrained_string_optional_default_bool" {
			nullable = false
			type     = string
			default  = true
		}
		variable "constrained_string_sensitive_required" {
			sensitive = true
			nullable  = false
			type      = string
		}
		variable "complex_type_with_nested_default_optional" {
			type = set(object({
				name      = string
				schedules = set(object({
					name               = string
					cold_storage_after = optional(number, 10)
				}))
  			}))
		}
		variable "complex_type_with_nested_complex_types" {
			type = object({
				name                       = string
				nested_object              = object({
					name  = string
					value = optional(string, "foo")
				})
				nested_object_with_default = optional(object({
					name  = string
					value = optional(string, "bar")
				}), {
					name = "nested_object_with_default"
				})
			})
		}
		// https://github.com/hashicorp/terraform/issues/32152
		// This variable was originally added to test that optional attribute
		// metadata is stripped from empty default collections. Essentially, you
		// should be able to mix and match custom and default values for the
		// optional_list attribute.
        variable "complex_type_with_empty_default_and_nested_optional" {
			type = list(object({
				name          = string
				optional_list = optional(list(object({
					string          = string
					optional_string = optional(string)
				})), [])
			}))
        }
 		// https://github.com/hashicorp/terraform/issues/32160#issuecomment-1302783910
		// These variables were added to test the specific use case from this
		// GitHub comment.
		variable "empty_object_with_optional_nested_object_with_optional_bool" {
			type = object({
				thing = optional(object({
					flag = optional(bool, false)
				}))
			})
			default = {}
		}
		variable "populated_object_with_optional_nested_object_with_optional_bool" {
			type = object({
				thing = optional(object({
					flag = optional(bool, false)
				}))
			})
			default = {
				thing = {}
			}
		}
		variable "empty_object_with_default_nested_object_with_optional_bool" {
			type = object({
				thing = optional(object({
					flag = optional(bool, false)
				}), {})
			})
			default = {}
		}
		// https://github.com/hashicorp/terraform/issues/32160
		// This variable was originally added to test that optional objects do
		// get created containing only their defaults. Instead they should be
		// left empty. We do not expect nested_object to be created just because
		// optional_string has a default value.
		variable "object_with_nested_object_with_required_and_optional_attributes" {
			type = object({
				nested_object = optional(object({
					string          = string
					optional_string = optional(string, "optional")
				}))
			})
		}
		// https://github.com/hashicorp/terraform/issues/32157
		// Similar to above, we want to see that merging combinations of the
		// nested_object into a single collection doesn't crash because of
		// inconsistent elements.
		variable "list_with_nested_object_with_required_and_optional_attributes" {
			type = list(object({
				nested_object = optional(object({
					string          = string
					optional_string = optional(string, "optional")
				}))
			}))
		}
		// https://github.com/hashicorp/terraform/issues/32109
		// This variable was originally introduced to test the behaviour of 
		// the dynamic type constraint. You should be able to use the 'any' 
		// constraint and introduce empty, null, and populated values into the
		// list.
		variable "list_with_nested_list_of_any" {
			type = list(object({
				a = string
				b = optional(list(any))
			}))
			default = [
				{
					a = "a"
				},
				{
					a = "b"
					b = [1]
				}
			]
		}
		// https://github.com/hashicorp/terraform/issues/32396
		// This variable was originally introduced to test the behaviour of the
        // dynamic type constraint. You should be able to set primitive types in
        // the list consistently.
        variable "list_with_nested_collections_dynamic_with_default" {
			type = list(
				object({
					name = optional(string, "default")
					taints = optional(list(map(any)), [])
				})
			)
		}
        // https://github.com/hashicorp/terraform/issues/32752
		// This variable was introduced to make sure the evaluation doesn't 
        // crash even when the types are wrong.
        variable "invalid_nested_type" {
            type = map(
                object({
					rules = map(
						object({
							destination_addresses = optional(list(string), [])
						})
					)
                })
            )
			default = {}
        }
	`
	cfg := testModuleInline(t, map[string]string{
		"main.tf": cfgSrc,
	})
	variableConfigs := cfg.Module.Variables

	// Because we loaded our pseudo-module from a temporary file, the
	// declaration source ranges will have unpredictable filenames. We'll
	// fix that here just to make things easier below.
	for _, vc := range variableConfigs {
		vc.DeclRange.Filename = "main.tf"
	}

	tests := []struct {
		varName string
		given   cty.Value
		want    cty.Value
		wantErr string
	}{
		// nullable_required
		{
			"nullable_required",
			cty.NilVal,
			cty.UnknownVal(cty.DynamicPseudoType),
			`Required variable not set: The variable "nullable_required" is required, but is not set.`,
		},
		{
			"nullable_required",
			cty.NullVal(cty.DynamicPseudoType),
			cty.NullVal(cty.DynamicPseudoType),
			``, // "required" for a nullable variable means only that it must be set, even if it's set to null
		},
		{
			"nullable_required",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"nullable_required",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// nullable_optional_default_string
		{
			"nullable_optional_default_string",
			cty.NilVal,
			cty.StringVal("hello"), // the declared default value
			``,
		},
		{
			"nullable_optional_default_string",
			cty.NullVal(cty.DynamicPseudoType),
			cty.NullVal(cty.DynamicPseudoType), // nullable variables can be really set to null, masking the default
			``,
		},
		{
			"nullable_optional_default_string",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"nullable_optional_default_string",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// nullable_optional_default_null
		{
			"nullable_optional_default_null",
			cty.NilVal,
			cty.NullVal(cty.DynamicPseudoType), // the declared default value
			``,
		},
		{
			"nullable_optional_default_null",
			cty.NullVal(cty.String),
			cty.NullVal(cty.String), // nullable variables can be really set to null, masking the default
			``,
		},
		{
			"nullable_optional_default_null",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"nullable_optional_default_null",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_nullable_required
		{
			"constrained_string_nullable_required",
			cty.NilVal,
			cty.UnknownVal(cty.String),
			`Required variable not set: The variable "constrained_string_nullable_required" is required, but is not set.`,
		},
		{
			"constrained_string_nullable_required",
			cty.NullVal(cty.DynamicPseudoType),
			cty.NullVal(cty.String), // the null value still gets converted to match the type constraint
			``,                      // "required" for a nullable variable means only that it must be set, even if it's set to null
		},
		{
			"constrained_string_nullable_required",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_nullable_required",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_nullable_optional_default_string
		{
			"constrained_string_nullable_optional_default_string",
			cty.NilVal,
			cty.StringVal("hello"), // the declared default value
			``,
		},
		{
			"constrained_string_nullable_optional_default_string",
			cty.NullVal(cty.DynamicPseudoType),
			cty.NullVal(cty.String), // nullable variables can be really set to null, masking the default
			``,
		},
		{
			"constrained_string_nullable_optional_default_string",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_nullable_optional_default_string",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_nullable_optional_default_bool
		{
			"constrained_string_nullable_optional_default_bool",
			cty.NilVal,
			cty.StringVal("true"), // the declared default value, automatically converted to match type constraint
			``,
		},
		{
			"constrained_string_nullable_optional_default_bool",
			cty.NullVal(cty.DynamicPseudoType),
			cty.NullVal(cty.String), // nullable variables can be really set to null, masking the default
			``,
		},
		{
			"constrained_string_nullable_optional_default_bool",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_nullable_optional_default_bool",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_nullable_optional_default_null
		{
			"constrained_string_nullable_optional_default_null",
			cty.NilVal,
			cty.NullVal(cty.String),
			``,
		},
		{
			"constrained_string_nullable_optional_default_null",
			cty.NullVal(cty.DynamicPseudoType),
			cty.NullVal(cty.String),
			``,
		},
		{
			"constrained_string_nullable_optional_default_null",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_nullable_optional_default_null",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// required
		{
			"required",
			cty.NilVal,
			cty.UnknownVal(cty.DynamicPseudoType),
			`Required variable not set: The variable "required" is required, but is not set.`,
		},
		{
			"required",
			cty.NullVal(cty.DynamicPseudoType),
			cty.UnknownVal(cty.DynamicPseudoType),
			`Required variable not set: Unsuitable value for var.required set from outside of the configuration: required variable may not be set to null.`,
		},
		{
			"required",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"required",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// optional_default_string
		{
			"optional_default_string",
			cty.NilVal,
			cty.StringVal("hello"), // the declared default value
			``,
		},
		{
			"optional_default_string",
			cty.NullVal(cty.DynamicPseudoType),
			cty.StringVal("hello"), // the declared default value
			``,
		},
		{
			"optional_default_string",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"optional_default_string",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_required
		{
			"constrained_string_required",
			cty.NilVal,
			cty.UnknownVal(cty.String),
			`Required variable not set: The variable "constrained_string_required" is required, but is not set.`,
		},
		{
			"constrained_string_required",
			cty.NullVal(cty.DynamicPseudoType),
			cty.UnknownVal(cty.String),
			`Required variable not set: Unsuitable value for var.constrained_string_required set from outside of the configuration: required variable may not be set to null.`,
		},
		{
			"constrained_string_required",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_required",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_optional_default_string
		{
			"constrained_string_optional_default_string",
			cty.NilVal,
			cty.StringVal("hello"), // the declared default value
			``,
		},
		{
			"constrained_string_optional_default_string",
			cty.NullVal(cty.DynamicPseudoType),
			cty.StringVal("hello"), // the declared default value
			``,
		},
		{
			"constrained_string_optional_default_string",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_optional_default_string",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},

		// constrained_string_optional_default_bool
		{
			"constrained_string_optional_default_bool",
			cty.NilVal,
			cty.StringVal("true"), // the declared default value, automatically converted to match type constraint
			``,
		},
		{
			"constrained_string_optional_default_bool",
			cty.NullVal(cty.DynamicPseudoType),
			cty.StringVal("true"), // the declared default value, automatically converted to match type constraint
			``,
		},
		{
			"constrained_string_optional_default_bool",
			cty.StringVal("ahoy"),
			cty.StringVal("ahoy"),
			``,
		},
		{
			"constrained_string_optional_default_bool",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},
		{
			"list_with_nested_collections_dynamic_with_default",
			cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("default"),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("complex"),
					"taints": cty.ListVal([]cty.Value{
						cty.MapVal(map[string]cty.Value{
							"key":   cty.StringVal("my_key"),
							"value": cty.StringVal("my_value"),
						}),
					}),
				}),
			}),
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("default"),
					"taints": cty.ListValEmpty(cty.Map(cty.String)),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("complex"),
					"taints": cty.ListVal([]cty.Value{
						cty.MapVal(map[string]cty.Value{
							"key":   cty.StringVal("my_key"),
							"value": cty.StringVal("my_value"),
						}),
					}),
				}),
			}),
			``,
		},

		// complex types

		{
			"complex_type_with_nested_default_optional",
			cty.SetVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("test1"),
					"schedules": cty.SetVal([]cty.Value{
						cty.MapVal(map[string]cty.Value{
							"name": cty.StringVal("daily"),
						}),
					}),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("test2"),
					"schedules": cty.SetVal([]cty.Value{
						cty.MapVal(map[string]cty.Value{
							"name": cty.StringVal("daily"),
						}),
						cty.MapVal(map[string]cty.Value{
							"name":               cty.StringVal("weekly"),
							"cold_storage_after": cty.StringVal("0"),
						}),
					}),
				}),
			}),
			cty.SetVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("test1"),
					"schedules": cty.SetVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"name":               cty.StringVal("daily"),
							"cold_storage_after": cty.NumberIntVal(10),
						}),
					}),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("test2"),
					"schedules": cty.SetVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"name":               cty.StringVal("daily"),
							"cold_storage_after": cty.NumberIntVal(10),
						}),
						cty.ObjectVal(map[string]cty.Value{
							"name":               cty.StringVal("weekly"),
							"cold_storage_after": cty.NumberIntVal(0),
						}),
					}),
				}),
			}),
			``,
		},
		{
			"complex_type_with_nested_complex_types",
			cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("object"),
				"nested_object": cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("nested_object"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("object"),
				"nested_object": cty.ObjectVal(map[string]cty.Value{
					"name":  cty.StringVal("nested_object"),
					"value": cty.StringVal("foo"),
				}),
				"nested_object_with_default": cty.ObjectVal(map[string]cty.Value{
					"name":  cty.StringVal("nested_object_with_default"),
					"value": cty.StringVal("bar"),
				}),
			}),
			``,
		},
		{
			"complex_type_with_empty_default_and_nested_optional",
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("abc"),
					"optional_list": cty.ListVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"string":          cty.StringVal("child"),
							"optional_string": cty.NullVal(cty.String),
						}),
					}),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("def"),
					"optional_list": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
						"string":          cty.String,
						"optional_string": cty.String,
					}))),
				}),
			}),
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("abc"),
					"optional_list": cty.ListVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"string":          cty.StringVal("child"),
							"optional_string": cty.NullVal(cty.String),
						}),
					}),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("def"),
					"optional_list": cty.ListValEmpty(cty.Object(map[string]cty.Type{
						"string":          cty.String,
						"optional_string": cty.String,
					})),
				}),
			}),
			``,
		},
		{
			"object_with_nested_object_with_required_and_optional_attributes",
			cty.EmptyObjectVal,
			cty.ObjectVal(map[string]cty.Value{
				"nested_object": cty.NullVal(cty.Object(map[string]cty.Type{
					"string":          cty.String,
					"optional_string": cty.String,
				})),
			}),
			``,
		},
		{
			"empty_object_with_optional_nested_object_with_optional_bool",
			cty.NilVal,
			cty.ObjectVal(map[string]cty.Value{
				"thing": cty.NullVal(cty.Object(map[string]cty.Type{
					"flag": cty.Bool,
				})),
			}),
			``,
		},
		{
			"populated_object_with_optional_nested_object_with_optional_bool",
			cty.NilVal,
			cty.ObjectVal(map[string]cty.Value{
				"thing": cty.ObjectVal(map[string]cty.Value{
					"flag": cty.False,
				}),
			}),
			``,
		},
		{
			"empty_object_with_default_nested_object_with_optional_bool",
			cty.NilVal,
			cty.ObjectVal(map[string]cty.Value{
				"thing": cty.ObjectVal(map[string]cty.Value{
					"flag": cty.False,
				}),
			}),
			``,
		},
		{
			"list_with_nested_object_with_required_and_optional_attributes",
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"nested_object": cty.ObjectVal(map[string]cty.Value{
						"string":          cty.StringVal("string"),
						"optional_string": cty.NullVal(cty.String),
					}),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"nested_object": cty.NullVal(cty.Object(map[string]cty.Type{
						"string":          cty.String,
						"optional_string": cty.String,
					})),
				}),
			}),
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"nested_object": cty.ObjectVal(map[string]cty.Value{
						"string":          cty.StringVal("string"),
						"optional_string": cty.StringVal("optional"),
					}),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"nested_object": cty.NullVal(cty.Object(map[string]cty.Type{
						"string":          cty.String,
						"optional_string": cty.String,
					})),
				}),
			}),
			``,
		},
		{
			"list_with_nested_list_of_any",
			cty.NilVal,
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("a"),
					"b": cty.NullVal(cty.List(cty.Number)),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("b"),
					"b": cty.ListVal([]cty.Value{
						cty.NumberIntVal(1),
					}),
				}),
			}),
			``,
		},
		{
			"list_with_nested_collections_dynamic_with_default",
			cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("default"),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("complex"),
					"taints": cty.ListVal([]cty.Value{
						cty.MapVal(map[string]cty.Value{
							"key":   cty.StringVal("my_key"),
							"value": cty.StringVal("my_value"),
						}),
					}),
				}),
			}),
			cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"name":   cty.StringVal("default"),
					"taints": cty.ListValEmpty(cty.Map(cty.String)),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("complex"),
					"taints": cty.ListVal([]cty.Value{
						cty.MapVal(map[string]cty.Value{
							"key":   cty.StringVal("my_key"),
							"value": cty.StringVal("my_value"),
						}),
					}),
				}),
			}),
			``,
		},
		{
			"invalid_nested_type",
			cty.MapVal(map[string]cty.Value{
				"mysql": cty.ObjectVal(map[string]cty.Value{
					"rules": cty.ObjectVal(map[string]cty.Value{
						"destination_addresses": cty.ListVal([]cty.Value{cty.StringVal("192.168.0.1")}),
					}),
				}),
			}),
			cty.UnknownVal(cty.Map(cty.Object(map[string]cty.Type{
				"rules": cty.Map(cty.Object(map[string]cty.Type{
					"destination_addresses": cty.List(cty.String),
				})),
			}))),
			`Invalid value for input variable: Unsuitable value for var.invalid_nested_type set from outside of the configuration: incorrect map element type: attribute "rules": element "destination_addresses": object required.`,
		},

		// sensitive
		{
			"constrained_string_sensitive_required",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String),
			``,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %#v", test.varName, test.given), func(t *testing.T) {
			varAddr := addrs.InputVariable{Name: test.varName}.Absolute(addrs.RootModuleInstance)
			varCfg := variableConfigs[test.varName]
			if varCfg == nil {
				t.Fatalf("invalid variable name %q", test.varName)
			}

			t.Logf(
				"test case\nvariable:    %s\nconstraint:  %#v\ndefault:     %#v\nnullable:    %#v\ngiven value: %#v",
				varAddr,
				varCfg.Type,
				varCfg.Default,
				varCfg.Nullable,
				test.given,
			)

			rawVal := &InputValue{
				Value:      test.given,
				SourceType: ValueFromCaller,
			}

			got, diags := prepareFinalInputVariableValue(
				varAddr, rawVal, varCfg,
			)

			if test.wantErr != "" {
				if !diags.HasErrors() {
					t.Errorf("unexpected success\nwant error: %s", test.wantErr)
				} else if got, want := diags.Err().Error(), test.wantErr; got != want {
					t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
				}
			} else {
				if diags.HasErrors() {
					t.Errorf("unexpected error\ngot: %s", diags.Err().Error())
				}
			}

			// NOTE: should still have returned some reasonable value even if there was an error
			if !test.want.RawEquals(got) {
				t.Fatalf("wrong result\ngot:  %#v\nwant: %#v", got, test.want)
			}
		})
	}

	t.Run("SourceType error message variants", func(t *testing.T) {
		tests := []struct {
			SourceType  ValueSourceType
			SourceRange tfdiags.SourceRange
			WantTypeErr string
			WantNullErr string
		}{
			{
				ValueFromUnknown,
				tfdiags.SourceRange{},
				`Invalid value for input variable: Unsuitable value for var.constrained_string_required set from outside of the configuration: string required, but have object.`,
				`Required variable not set: Unsuitable value for var.constrained_string_required set from outside of the configuration: required variable may not be set to null.`,
			},
			{
				ValueFromConfig,
				tfdiags.SourceRange{
					Filename: "example.tf",
					Start:    tfdiags.SourcePos(hcl.InitialPos),
					End:      tfdiags.SourcePos(hcl.InitialPos),
				},
				`Invalid value for input variable: The given value is not suitable for var.constrained_string_required declared at main.tf:32,3-41: string required, but have object.`,
				`Required variable not set: The given value is not suitable for var.constrained_string_required defined at main.tf:32,3-41: required variable may not be set to null.`,
			},
			{
				ValueFromAutoFile,
				tfdiags.SourceRange{
					Filename: "example.auto.tfvars",
					Start:    tfdiags.SourcePos(hcl.InitialPos),
					End:      tfdiags.SourcePos(hcl.InitialPos),
				},
				`Invalid value for input variable: The given value is not suitable for var.constrained_string_required declared at main.tf:32,3-41: string required, but have object.`,
				`Required variable not set: The given value is not suitable for var.constrained_string_required defined at main.tf:32,3-41: required variable may not be set to null.`,
			},
			{
				ValueFromNamedFile,
				tfdiags.SourceRange{
					Filename: "example.tfvars",
					Start:    tfdiags.SourcePos(hcl.InitialPos),
					End:      tfdiags.SourcePos(hcl.InitialPos),
				},
				`Invalid value for input variable: The given value is not suitable for var.constrained_string_required declared at main.tf:32,3-41: string required, but have object.`,
				`Required variable not set: The given value is not suitable for var.constrained_string_required defined at main.tf:32,3-41: required variable may not be set to null.`,
			},
			{
				ValueFromCLIArg,
				tfdiags.SourceRange{},
				`Invalid value for input variable: Unsuitable value for var.constrained_string_required set using -var="constrained_string_required=...": string required, but have object.`,
				`Required variable not set: Unsuitable value for var.constrained_string_required set using -var="constrained_string_required=...": required variable may not be set to null.`,
			},
			{
				ValueFromEnvVar,
				tfdiags.SourceRange{},
				`Invalid value for input variable: Unsuitable value for var.constrained_string_required set using the TF_VAR_constrained_string_required environment variable: string required, but have object.`,
				`Required variable not set: Unsuitable value for var.constrained_string_required set using the TF_VAR_constrained_string_required environment variable: required variable may not be set to null.`,
			},
			{
				ValueFromInput,
				tfdiags.SourceRange{},
				`Invalid value for input variable: Unsuitable value for var.constrained_string_required set using an interactive prompt: string required, but have object.`,
				`Required variable not set: Unsuitable value for var.constrained_string_required set using an interactive prompt: required variable may not be set to null.`,
			},
			{
				// NOTE: This isn't actually a realistic case for this particular
				// function, because if we have a value coming from a plan then
				// we must be in the apply step, and we shouldn't be able to
				// get past the plan step if we have invalid variable values,
				// and during planning we'll always have other source types.
				ValueFromPlan,
				tfdiags.SourceRange{},
				`Invalid value for input variable: Unsuitable value for var.constrained_string_required set from outside of the configuration: string required, but have object.`,
				`Required variable not set: Unsuitable value for var.constrained_string_required set from outside of the configuration: required variable may not be set to null.`,
			},
			{
				ValueFromCaller,
				tfdiags.SourceRange{},
				`Invalid value for input variable: Unsuitable value for var.constrained_string_required set from outside of the configuration: string required, but have object.`,
				`Required variable not set: Unsuitable value for var.constrained_string_required set from outside of the configuration: required variable may not be set to null.`,
			},
		}

		for _, test := range tests {
			t.Run(fmt.Sprintf("%s %s", test.SourceType, test.SourceRange.StartString()), func(t *testing.T) {
				varAddr := addrs.InputVariable{Name: "constrained_string_required"}.Absolute(addrs.RootModuleInstance)
				varCfg := variableConfigs[varAddr.Variable.Name]
				t.Run("type error", func(t *testing.T) {
					rawVal := &InputValue{
						Value:       cty.EmptyObjectVal,
						SourceType:  test.SourceType,
						SourceRange: test.SourceRange,
					}

					_, diags := prepareFinalInputVariableValue(
						varAddr, rawVal, varCfg,
					)
					if !diags.HasErrors() {
						t.Fatalf("unexpected success; want error")
					}

					if got, want := diags.Err().Error(), test.WantTypeErr; got != want {
						t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
					}
				})
				t.Run("null error", func(t *testing.T) {
					rawVal := &InputValue{
						Value:       cty.NullVal(cty.DynamicPseudoType),
						SourceType:  test.SourceType,
						SourceRange: test.SourceRange,
					}

					_, diags := prepareFinalInputVariableValue(
						varAddr, rawVal, varCfg,
					)
					if !diags.HasErrors() {
						t.Fatalf("unexpected success; want error")
					}

					if got, want := diags.Err().Error(), test.WantNullErr; got != want {
						t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
					}
				})
			})
		}
	})

	t.Run("SensitiveVariable error message variants, with source variants", func(t *testing.T) {
		tests := []struct {
			SourceType  ValueSourceType
			SourceRange tfdiags.SourceRange
			WantTypeErr string
			HideSubject bool
		}{
			{
				ValueFromUnknown,
				tfdiags.SourceRange{},
				"Invalid value for input variable: Unsuitable value for var.constrained_string_sensitive_required set from outside of the configuration: string required, but have object.",
				false,
			},
			{
				ValueFromConfig,
				tfdiags.SourceRange{
					Filename: "example.tfvars",
					Start:    tfdiags.SourcePos(hcl.InitialPos),
					End:      tfdiags.SourcePos(hcl.InitialPos),
				},
				`Invalid value for input variable: The given value is not suitable for var.constrained_string_sensitive_required, which is sensitive: string required, but have object. Invalid value defined at example.tfvars:1,1-1.`,
				true,
			},
		}

		for _, test := range tests {
			t.Run(fmt.Sprintf("%s %s", test.SourceType, test.SourceRange.StartString()), func(t *testing.T) {
				varAddr := addrs.InputVariable{Name: "constrained_string_sensitive_required"}.Absolute(addrs.RootModuleInstance)
				varCfg := variableConfigs[varAddr.Variable.Name]
				t.Run("type error", func(t *testing.T) {
					rawVal := &InputValue{
						Value:       cty.EmptyObjectVal,
						SourceType:  test.SourceType,
						SourceRange: test.SourceRange,
					}

					_, diags := prepareFinalInputVariableValue(
						varAddr, rawVal, varCfg,
					)
					if !diags.HasErrors() {
						t.Fatalf("unexpected success; want error")
					}

					if got, want := diags.Err().Error(), test.WantTypeErr; got != want {
						t.Errorf("wrong error\ngot:  %s\nwant: %s", got, want)
					}

					if test.HideSubject {
						if got, want := diags[0].Source().Subject.StartString(), test.SourceRange.StartString(); got == want {
							t.Errorf("Subject start should have been hidden, but was %s", got)
						}
					}
				})
			})
		}
	})
}

// These tests cover the JSON syntax configuration edge case handling,
// the background of which is described in detail in comments in the
// evalVariableValidations function. Future versions of OpenTofu may
// be able to remove this behaviour altogether.
func TestEvalVariableValidations_jsonErrorMessageEdgeCase(t *testing.T) {
	cfgSrc := `{
  "variable": {
    "valid": {
      "type": "string",
      "validation": {
        "condition": "${var.valid != \"bar\"}",
        "error_message": "Valid template string ${var.valid}"
      }
    },
    "invalid": {
      "type": "string",
      "validation": {
        "condition": "${var.invalid != \"bar\"}",
        "error_message": "Invalid template string ${"
      }
    }
  }
}
`
	cfg := testModuleInline(t, map[string]string{
		"main.tf.json": cfgSrc,
	})
	variableConfigs := cfg.Module.Variables

	// Because we loaded our pseudo-module from a temporary file, the
	// declaration source ranges will have unpredictable filenames. We'll
	// fix that here just to make things easier below.
	for _, vc := range variableConfigs {
		vc.DeclRange.Filename = "main.tf.json"
		for _, v := range vc.Validations {
			v.DeclRange.Filename = "main.tf.json"
		}
	}

	tests := []struct {
		varName  string
		given    cty.Value
		wantErr  []string
		wantWarn []string
		status   checks.Status
	}{
		// Valid variable validation declaration, assigned value which passes
		// the condition generates no diagnostics.
		{
			varName: "valid",
			given:   cty.StringVal("foo"),
			status:  checks.StatusPass,
		},
		// Assigning a value which fails the condition generates an error
		// message with the expression successfully evaluated.
		{
			varName: "valid",
			given:   marks.Deprecated(cty.StringVal("bar"), marks.DeprecationCause{}), // deprecation mark must not cause panic
			wantErr: []string{
				"Invalid value for variable",
				"Valid template string bar",
			},
			status: checks.StatusFail,
		},
		// Invalid variable validation declaration due to an unparsable
		// template string. Assigning a value which passes the condition
		// results in a warning about the error message.
		{
			varName: "invalid",
			given:   cty.StringVal("foo"),
			wantWarn: []string{
				"Validation error message expression is invalid",
				"Missing expression; Expected the start of an expression, but found the end of the file.",
			},
			status: checks.StatusPass,
		},
		// Assigning a value which fails the condition generates an error
		// message including the configured string interpreted as a literal
		// value, and the same warning diagnostic as above.
		{
			varName: "invalid",
			given:   cty.StringVal("bar"),
			wantErr: []string{
				"Invalid value for variable",
				"Invalid template string ${",
			},
			wantWarn: []string{
				"Validation error message expression is invalid",
				"Missing expression; Expected the start of an expression, but found the end of the file.",
			},
			status: checks.StatusFail,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %#v", test.varName, test.given), func(t *testing.T) {
			varAddr := addrs.InputVariable{Name: test.varName}.Absolute(addrs.RootModuleInstance)
			varCfg := variableConfigs[test.varName]
			if varCfg == nil {
				t.Fatalf("invalid variable name %q", test.varName)
			}

			// Build a mock context to allow the function under test to
			// retrieve the variable value and evaluate the expressions
			ctx := &MockEvalContext{}

			// We need a minimal scope to allow basic functions to be passed to
			// the HCL scope
			ctx.EvaluationScopeScope = &lang.Scope{
				Data: &evaluationStateData{Evaluator: &Evaluator{
					Config:             cfg,
					VariableValuesLock: &sync.Mutex{},
					VariableValues: map[string]map[string]cty.Value{"": {
						test.varName: cty.UnknownVal(cty.String),
					}},
				}},
			}
			ctx.GetVariableValueFunc = func(addr addrs.AbsInputVariableInstance) cty.Value {
				if got, want := addr.String(), varAddr.String(); got != want {
					t.Errorf("incorrect argument to GetVariableValue: got %s, want %s", got, want)
				}
				return test.given
			}
			ctx.ChecksState = checks.NewState(cfg)
			ctx.ChecksState.ReportCheckableObjects(varAddr.ConfigCheckable(), addrs.MakeSet[addrs.Checkable](varAddr))

			gotDiags := evalVariableValidations(
				varAddr, varCfg, nil, ctx,
			)

			if ctx.ChecksState.ObjectCheckStatus(varAddr) != test.status {
				t.Errorf("expected check result %s but instead %s", test.status, ctx.ChecksState.ObjectCheckStatus(varAddr))
			}

			if len(test.wantErr) == 0 && len(test.wantWarn) == 0 {
				if len(gotDiags) > 0 {
					t.Errorf("no diags expected, got %s", gotDiags.Err().Error())
				}
			} else {
			wantErrs:
				for _, want := range test.wantErr {
					for _, diag := range gotDiags {
						if diag.Severity() != tfdiags.Error {
							continue
						}
						desc := diag.Description()
						if strings.Contains(desc.Summary, want) || strings.Contains(desc.Detail, want) {
							continue wantErrs
						}
					}
					t.Errorf("no error diagnostics found containing %q\ngot: %s", want, gotDiags.Err().Error())
				}

			wantWarns:
				for _, want := range test.wantWarn {
					for _, diag := range gotDiags {
						if diag.Severity() != tfdiags.Warning {
							continue
						}
						desc := diag.Description()
						if strings.Contains(desc.Summary, want) || strings.Contains(desc.Detail, want) {
							continue wantWarns
						}
					}
					t.Errorf("no warning diagnostics found containing %q\ngot: %s", want, gotDiags.Err().Error())
				}
			}
		})
	}
}

func TestEvalVariableValidations_sensitiveValues(t *testing.T) {
	cfgSrc := `
variable "foo" {
  type      = string
  sensitive = true
  default   = "boop"

  validation {
    condition     = length(var.foo) == 4
	error_message = "Foo must be 4 characters, not ${length(var.foo)}"
  }
}

variable "bar" {
  type      = string
  sensitive = true
  default   = "boop"

  validation {
    condition     = length(var.bar) == 4
	error_message = "Bar must be 4 characters, not ${nonsensitive(length(var.bar))}."
  }
}
`
	cfg := testModuleInline(t, map[string]string{
		"main.tf": cfgSrc,
	})
	variableConfigs := cfg.Module.Variables

	// Because we loaded our pseudo-module from a temporary file, the
	// declaration source ranges will have unpredictable filenames. We'll
	// fix that here just to make things easier below.
	for _, vc := range variableConfigs {
		vc.DeclRange.Filename = "main.tf"
		for _, v := range vc.Validations {
			v.DeclRange.Filename = "main.tf"
		}
	}

	tests := []struct {
		varName string
		given   cty.Value
		wantErr []string
		status  checks.Status
	}{
		// Validations pass on a sensitive variable with an error message which
		// would generate a sensitive value
		{
			varName: "foo",
			given:   cty.StringVal("boop"),
			status:  checks.StatusPass,
		},
		// Assigning a value which fails the condition generates a sensitive
		// error message, which is elided and generates another error
		{
			varName: "foo",
			given:   cty.StringVal("bap"),
			wantErr: []string{
				"Invalid value for variable",
				"The error message included a sensitive value, so it will not be displayed.",
				"Error message refers to sensitive values",
			},
			status: checks.StatusFail,
		},
		// Validations pass on a sensitive variable with a correctly defined
		// error message
		{
			varName: "bar",
			given:   cty.StringVal("boop"),
			status:  checks.StatusPass,
		},
		// Assigning a value which fails the condition generates a nonsensitive
		// error message, which is displayed
		{
			varName: "bar",
			given:   cty.StringVal("bap"),
			wantErr: []string{
				"Invalid value for variable",
				"Bar must be 4 characters, not 3.",
			},
			status: checks.StatusFail,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %#v", test.varName, test.given), func(t *testing.T) {
			varAddr := addrs.InputVariable{Name: test.varName}.Absolute(addrs.RootModuleInstance)
			varCfg := variableConfigs[test.varName]
			if varCfg == nil {
				t.Fatalf("invalid variable name %q", test.varName)
			}

			// Build a mock context to allow the function under test to
			// retrieve the variable value and evaluate the expressions
			ctx := &MockEvalContext{}

			// We need a minimal scope to allow basic functions to be passed to
			// the HCL scope
			ctx.EvaluationScopeScope = &lang.Scope{
				Data: &evaluationStateData{Evaluator: &Evaluator{
					Config:             cfg,
					VariableValuesLock: &sync.Mutex{},
					VariableValues: map[string]map[string]cty.Value{"": {
						test.varName: cty.UnknownVal(cty.String),
					}},
				}},
			}
			ctx.GetVariableValueFunc = func(addr addrs.AbsInputVariableInstance) cty.Value {
				if got, want := addr.String(), varAddr.String(); got != want {
					t.Errorf("incorrect argument to GetVariableValue: got %s, want %s", got, want)
				}
				// NOTE: This intentionally doesn't mark the result as sensitive,
				// because BuiltinEvalContext.GetVariableValue doesn't either.
				// It's the responsibility of downstream code to detect and handle
				// configured sensitivity.
				return test.given
			}
			ctx.ChecksState = checks.NewState(cfg)
			ctx.ChecksState.ReportCheckableObjects(varAddr.ConfigCheckable(), addrs.MakeSet[addrs.Checkable](varAddr))

			gotDiags := evalVariableValidations(
				varAddr, varCfg, nil, ctx,
			)

			if ctx.ChecksState.ObjectCheckStatus(varAddr) != test.status {
				t.Errorf("expected check result %s but instead %s", test.status, ctx.ChecksState.ObjectCheckStatus(varAddr))
			}

			if len(test.wantErr) == 0 {
				if len(gotDiags) > 0 {
					t.Errorf("no diags expected, got %s", gotDiags.Err().Error())
				}
			} else {
			wantErrs:
				for _, want := range test.wantErr {
					for _, diag := range gotDiags {
						if diag.Severity() != tfdiags.Error {
							continue
						}
						desc := diag.Description()
						if strings.Contains(desc.Summary, want) || strings.Contains(desc.Detail, want) {
							continue wantErrs
						}
					}
					t.Errorf("no error diagnostics found containing %q\ngot: %s", want, gotDiags.Err().Error())
				}
			}
		})
	}
}

func TestEvalVariableValidations_sensitiveValueDiagnostics(t *testing.T) {
	// This test verifies that values for sensitive variables get captured
	// into diagnostic messages with the sensitive mark intact, so that
	// the values won't be disclosed in the UI.
	// Earlier versions handled this incorrectly:
	//    https://github.com/opentofu/opentofu/issues/2219

	cfgSrc := `
		variable "foo" {
			type      = string
			sensitive = true

			validation {
				condition     = length(var.foo) == 8 # intentionally fails
				error_message = "Foo must have 8 characters."
			}
		}
	`
	cfg := testModuleInline(t, map[string]string{
		"main.tf": cfgSrc,
	})
	varAddr := addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	ctx := &MockEvalContext{}
	ctx.EvaluationScopeScope = &lang.Scope{
		Data: &evaluationStateData{Evaluator: &Evaluator{
			Config:             cfg,
			VariableValuesLock: &sync.Mutex{},
			VariableValues: map[string]map[string]cty.Value{"": {
				varAddr.Variable.Name: cty.UnknownVal(cty.String),
			}},
		}},
	}
	ctx.GetVariableValueFunc = func(addr addrs.AbsInputVariableInstance) cty.Value {
		if got, want := addr.String(), varAddr.String(); got != want {
			t.Errorf("incorrect argument to GetVariableValue: got %s, want %s", got, want)
		}
		// NOTE: This intentionally doesn't mark the result as sensitive,
		// because BuiltinEvalContext.GetVariableValueFunc doesn't either.
		// It's the responsibility of downstream code to detect and handle
		// configured sensitivity.
		return cty.StringVal("boop")
	}
	ctx.ChecksState = checks.NewState(cfg)
	ctx.ChecksState.ReportCheckableObjects(varAddr.ConfigCheckable(), addrs.MakeSet[addrs.Checkable](varAddr))

	gotDiags := evalVariableValidations(
		varAddr, cfg.Module.Variables["foo"], nil, ctx,
	)
	if !gotDiags.HasErrors() {
		t.Fatalf("unexpected success; want validation error")
	}

	// The generated diagnostic(s) should all capture the evaluation context
	// that was used to evaluate the condition, where the variable's value
	// should be marked as sensitive so it won't get displayed in clear
	// in the UI output.
	// (The HasErrors check above guarantees that there's at least one
	// diagnostic here for us to iterate over.)
	for _, diag := range gotDiags {
		if diag.Severity() != tfdiags.Error {
			continue
		}
		fromExpr := diag.FromExpr()
		if fromExpr == nil {
			t.Fatalf("diagnostic does not have source expression information at all")
		}
		allVarVals := fromExpr.EvalContext.Variables["var"]
		if allVarVals == cty.NilVal || !allVarVals.Type().IsObjectType() {
			t.Fatalf("diagnostic did not capture an object value for the top-level symbol 'var'")
		}
		gotVal := allVarVals.GetAttr("foo")
		if gotVal == cty.NilVal {
			t.Fatalf("diagnostic did not capture a value for var.foo")
		}
		if !gotVal.HasMark(marks.Sensitive) {
			t.Errorf("var.foo value is not marked as sensitive in diagnostic")
		}
	}
}

// Testing the way variable deprecation diagnostics are generated
func TestEvalVariableValidations_deprecationDiagnostics(t *testing.T) {
	cfg := testModule(t, "validate-deprecated-var")

	ctx := &MockEvalContext{}
	ctx.EvaluationScopeScope = &lang.Scope{
		Data: &evaluationStateData{Evaluator: &Evaluator{
			Config:             cfg,
			VariableValuesLock: &sync.Mutex{},
		}},
	}

	tests := map[string]struct {
		varAddr    addrs.AbsInputVariableInstance
		varCfg     *configs.Variable
		expr       hcl.Expression
		fromRemote bool

		expectedDiags tfdiags.Diagnostics
	}{
		"local-mod-called-from-root": {
			varAddr: addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance.Child("foo-call", nil)),
			varCfg:  cfg.Children["foo-call"].Module.Variables["foo"],
			expr:    cfg.Module.ModuleCalls["foo-call"].Source,
			expectedDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  `Variable marked as deprecated by the module author`,
				Detail: fmt.Sprintf(
					"Variable \"foo\" is marked as deprecated with the following message:\n%s",
					cfg.Children["foo-call"].Module.Variables["foo"].Deprecated,
				),
				Subject: cfg.Module.ModuleCalls["foo-call"].Source.Range().Ptr(),
			}),
			fromRemote: false,
		},
		"local-mod-called-from-root-with-no-var": {
			varAddr:       addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance.Child("foo-call-no-var", nil)),
			varCfg:        cfg.Children["foo-call-no-var"].Module.Variables["foo"],
			expr:          nil,
			expectedDiags: tfdiags.Diagnostics{},
			fromRemote:    false,
		},
		"local-mod-called-from-root-with-null-var": {
			varAddr:       addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance.Child("foo-call-null", nil)),
			varCfg:        cfg.Children["foo-call-null"].Module.Variables["foo"],
			expr:          nil,
			expectedDiags: tfdiags.Diagnostics{},
			fromRemote:    false,
		},
		"local-mod-called-from-direct-child": {
			varAddr: addrs.InputVariable{Name: "bar"}.Absolute(addrs.RootModuleInstance.Child("foo-call", nil).Child("bar-call", nil)),
			varCfg:  cfg.Children["foo-call"].Children["bar-call"].Module.Variables["bar"],
			expr:    cfg.Children["foo-call"].Module.ModuleCalls["bar-call"].Source,
			expectedDiags: tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  `Variable marked as deprecated by the module author`,
				Detail: fmt.Sprintf(
					"Variable \"bar\" is marked as deprecated with the following message:\n%s",
					cfg.Children["foo-call"].Children["bar-call"].Module.Variables["bar"].Deprecated,
				),
				Subject: cfg.Children["foo-call"].Module.ModuleCalls["bar-call"].Source.Range().Ptr(),
			}),
			fromRemote: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			varAddr := tt.varAddr

			ctx.GetVariableValueFunc = func(addr addrs.AbsInputVariableInstance) cty.Value {
				if got, want := addr.String(), varAddr.String(); got != want {
					t.Errorf("incorrect argument to GetVariableValue: got %s, want %s", got, want)
				}
				// NOTE: the value itself doesn't matter. The value itself is not part of the processing so it can be whatever value we want
				return cty.StringVal("bar baz")
			}

			expr := tt.expr
			gotDiags := evalVariableDeprecation(varAddr, tt.varCfg, expr, ctx, tt.fromRemote)

			if gotLen, expectedLen := len(gotDiags), len(tt.expectedDiags); gotLen != expectedLen {
				t.Fatalf("expected %d diagnostics; got %d", expectedLen, gotLen)
			}
			for i := 0; i < len(gotDiags); i++ {
				gotDiag := gotDiags[i]
				expectedDiag := tt.expectedDiags[i]
				if gotDiag.Severity() != expectedDiag.Severity() {
					t.Fatalf(`return diagnostic is having the wrong severity. expected "%c"; got: "%c"`, expectedDiag.Severity(), gotDiag.Severity())
				}
				if gotDiag.Description().Summary != expectedDiag.Description().Summary {
					t.Fatalf("invalid diag summary.\n\texpected:\n\t\t%s\n\tactual:\n\t\t%s", expectedDiag.Description().Summary, gotDiag.Description().Summary)
				}
				if gotDiag.Description().Detail != expectedDiag.Description().Detail {
					t.Fatalf("invalid diag detail.\n\texpected:\n\t\t%s\n\tactual:\n\t\t%s", expectedDiag.Description().Detail, gotDiag.Description().Detail)
				}
				if !gotDiag.Source().Equal(expectedDiag.Source()) {
					t.Fatalf("invalid diag source.\n\texpected:\n\t\t%+v\n\tactual:\n\t\t%+v", expectedDiag.Source(), gotDiag.Source())
				}
			}
		})
	}
}
