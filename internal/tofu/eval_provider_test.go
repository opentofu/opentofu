// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
)

func TestBuildProviderConfig(t *testing.T) {
	configBody := configs.SynthBody("", map[string]cty.Value{
		"set_in_config": cty.StringVal("config"),
	})
	providerAddr := addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: addrs.NewDefaultProvider("foo"),
	}

	evalCtx := &MockEvalContext{
		// The input values map is expected to contain only keys that aren't
		// already present in the config, since we skip prompting for
		// attributes that are already set.
		ProviderInputValues: map[string]cty.Value{
			"set_by_input": cty.StringVal("input"),
		},
	}
	gotBody := buildProviderConfig(t.Context(), evalCtx, providerAddr, &configs.Provider{
		Name:   "foo",
		Config: configBody,
	})

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"set_in_config": {Type: cty.String, Optional: true},
			"set_by_input":  {Type: cty.String, Optional: true},
		},
	}
	got, diags := hcldec.Decode(gotBody, schema.DecoderSpec(), nil)
	if diags.HasErrors() {
		t.Fatalf("body decode failed: %s", diags.Error())
	}

	// We expect the provider config with the added input value
	want := cty.ObjectVal(map[string]cty.Value{
		"set_in_config": cty.StringVal("config"),
		"set_by_input":  cty.StringVal("input"),
	})
	if !got.RawEquals(want) {
		t.Fatalf("incorrect merged config\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestResolveProviderInstance_TypeConversion(t *testing.T) {
	testCases := []struct {
		name        string
		inputValue  cty.Value
		expectedKey addrs.InstanceKey
	}{
		{
			name:        "bool_true_to_string",
			inputValue:  cty.BoolVal(true),
			expectedKey: addrs.StringKey("true"),
		},
		{
			name:        "bool_false_to_string",
			inputValue:  cty.BoolVal(false),
			expectedKey: addrs.StringKey("false"),
		},
		{
			name:        "integer_to_string",
			inputValue:  cty.NumberIntVal(1),
			expectedKey: addrs.StringKey("1"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expr := hcl.StaticExpr(tc.inputValue, hcl.Range{})
			scope := &lang.Scope{
				Data: &evaluationStateData{
					ModulePath:      addrs.RootModuleInstance,
					InstanceKeyData: instances.RepetitionData{},
				},
				BaseDir: ".",
			}
			// call of the function to test
			actualKey, diags := resolveProviderInstance(expr, scope, "test-source")

			if diags.HasErrors() {
				t.Fatalf("Unexpected error: %s", diags.Err())
			}

			if actualKey != tc.expectedKey {
				t.Fatalf("Incorrect instance key got:  %#v want: %#v", actualKey, tc.expectedKey)
			}
		})
	}
}
