// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/zclconf/go-cty/cty"
)

func TestMergeMethodConfigs(t *testing.T) {
	makeMethodConfig := func(typeName, name, key, value string) MethodConfig {
		return MethodConfig{
			Type: typeName,
			Name: name,
			Body: hcl2shim.SynthBody("method", map[string]cty.Value{
				key: cty.StringVal(value),
			}),
		}
	}

	schema := &hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "key"}}}

	tests := []struct {
		name         string
		configSchema *hcl.BodySchema
		input        []MethodConfig
		override     []MethodConfig
		expected     []MethodConfig
	}{
		{
			name:         "empty",
			configSchema: nil,
			input:        []MethodConfig{},
			override:     []MethodConfig{},
			expected:     []MethodConfig{},
		},
		{
			name:         "override one method config body",
			configSchema: schema,
			input: []MethodConfig{
				makeMethodConfig("type", "name", "key", "value"),
			},
			override: []MethodConfig{
				makeMethodConfig("type", "name", "key", "override"),
			},
			expected: []MethodConfig{
				makeMethodConfig("type", "name", "key", "override"),
			},
		},
		{
			name:         "initial config is empty",
			configSchema: schema,
			input:        []MethodConfig{},
			override: []MethodConfig{
				makeMethodConfig("type", "name", "key", "override"),
			},
			expected: []MethodConfig{
				makeMethodConfig("type", "name", "key", "override"),
			},
		},
		{
			name:         "override multiple method configs",
			configSchema: schema,
			input: []MethodConfig{
				makeMethodConfig("type", "name", "key", "value"),
				makeMethodConfig("type", "name2", "key", "value"),
				makeMethodConfig("type", "name3", "key", "value"),
			},
			override: []MethodConfig{
				makeMethodConfig("type", "name", "key", "override1"),
				makeMethodConfig("type", "name2", "key", "override2"),
			},
			expected: []MethodConfig{
				makeMethodConfig("type", "name", "key", "override1"),
				makeMethodConfig("type", "name2", "key", "override2"),
				makeMethodConfig("type", "name3", "key", "value"),
			},
		},
		{
			name:         "override config is empty",
			configSchema: schema,
			input: []MethodConfig{
				makeMethodConfig("type", "name", "key", "value"),
			},
			override: []MethodConfig{},
			expected: []MethodConfig{
				makeMethodConfig("type", "name", "key", "value"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := mergeMethodConfigs(test.input, test.override)

			// for each of the expected methods, check if it exists in the output
			for _, expectedMethod := range test.expected {
				found := false
				for _, method := range output {
					if method.Type == expectedMethod.Type && method.Name == expectedMethod.Name {
						found = true
						expectedContent, _ := expectedMethod.Body.Content(test.configSchema)
						actualContent, diags := method.Body.Content(test.configSchema)
						if diags.HasErrors() {
							t.Fatalf("unexpected diagnostics: %v", diags)
						}
						// Only compare the attributes here, so that we don't look at things like the MissingItemRange on the hcl.Body
						if !reflect.DeepEqual(expectedContent.Attributes, actualContent.Attributes) {
							t.Errorf("expected %v, got %v", spew.Sdump(expectedContent.Attributes), spew.Sdump(actualContent.Attributes))
						}
					}
				}
				if !found {
					t.Errorf("expected method %v not found in output", spew.Sdump(expectedMethod))
				}
			}
		})
	}
}

func TestMergeKeyProviderConfigs(t *testing.T) {
	makeKeyProviderConfig := func(typeName, name, key, value string) KeyProviderConfig {
		return KeyProviderConfig{
			Type: typeName,
			Name: name,
			Body: hcl2shim.SynthBody("key_provider", map[string]cty.Value{
				key: cty.StringVal(value),
			}),
		}
	}

	schema := &hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "key"}}}

	tests := []struct {
		name         string
		configSchema *hcl.BodySchema
		input        []KeyProviderConfig
		override     []KeyProviderConfig
		expected     []KeyProviderConfig
	}{
		{
			name:         "empty",
			configSchema: nil,
			input:        []KeyProviderConfig{},
			override:     []KeyProviderConfig{},
			expected:     []KeyProviderConfig{},
		},
		{
			name:         "override one key provider config body",
			configSchema: schema,
			input: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "value"),
			},
			override: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "override"),
			},
			expected: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "override"),
			},
		},
		{
			name:         "initial config is empty",
			configSchema: schema,
			input:        []KeyProviderConfig{},
			override: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "override"),
			},
			expected: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "override"),
			},
		},
		{
			name:         "override multiple key provider configs",
			configSchema: schema,
			input: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "value"),
				makeKeyProviderConfig("type", "name2", "key", "value"),
			},
			override: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "override1"),
				makeKeyProviderConfig("type", "name2", "key", "override2"),
			},
			expected: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "override1"),
				makeKeyProviderConfig("type", "name2", "key", "override2"),
			},
		},
		{
			name:         "override config is empty",
			configSchema: schema,
			input: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "value"),
			},
			override: []KeyProviderConfig{},
			expected: []KeyProviderConfig{
				makeKeyProviderConfig("type", "name", "key", "value"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := mergeKeyProviderConfigs(test.input, test.override)

			// for each of the expected key providers, check if it exists in the output
			for _, expectedKeyProvider := range test.expected {
				found := false
				for _, keyProvider := range output {
					if keyProvider.Type == expectedKeyProvider.Type && keyProvider.Name == expectedKeyProvider.Name {
						found = true
						expectedContent, _ := expectedKeyProvider.Body.Content(test.configSchema)
						actualContent, diags := keyProvider.Body.Content(test.configSchema)
						if diags.HasErrors() {
							t.Fatalf("unexpected diagnostics: %v", diags)
						}
						// Only compare the attributes here, so that we don't look at things like the MissingItemRange on the hcl.Body
						if !reflect.DeepEqual(expectedContent.Attributes, actualContent.Attributes) {
							t.Errorf("expected %v, got %v", spew.Sdump(expectedContent.Attributes), spew.Sdump(actualContent.Attributes))
						}
					}
				}
				if !found {
					t.Errorf("expected key provider %v not found in output", spew.Sdump(expectedKeyProvider))
				}
			}
		})
	}
}

func TestMergeTargetConfigs(t *testing.T) {
	makeTargetConfig := func(enforced bool, method hcl.Expression, fallback *TargetConfig) *TargetConfig {
		return &TargetConfig{
			Method:   method,
			Fallback: fallback,
		}
	}

	makeEnforceableTargetConfig := func(enforced bool, method hcl.Expression, fallback *TargetConfig) *EnforceableTargetConfig {
		return &EnforceableTargetConfig{
			Enforced: enforced,
			Method:   method,
			Fallback: fallback,
		}
	}

	expressionOne := hcltest.MockExprLiteral(cty.UnknownVal(cty.Set(cty.String)))
	expressionTwo := hcltest.MockExprLiteral(cty.UnknownVal(cty.Set(cty.Bool)))

	tests := []struct {
		name     string
		input    *EnforceableTargetConfig
		override *EnforceableTargetConfig
		expected *EnforceableTargetConfig
	}{
		{
			name:     "both nil",
			input:    nil,
			override: nil,
			expected: nil,
		},
		{
			name:     "input is nil",
			input:    nil,
			override: makeEnforceableTargetConfig(true, expressionOne, nil),
			expected: makeEnforceableTargetConfig(true, expressionOne, nil),
		},
		{
			name:     "override is nil",
			input:    makeEnforceableTargetConfig(true, expressionOne, nil),
			override: nil,
			expected: makeEnforceableTargetConfig(true, expressionOne, nil),
		},
		{
			name:     "override target config method",
			input:    makeEnforceableTargetConfig(true, expressionOne, nil),
			override: makeEnforceableTargetConfig(true, expressionTwo, nil),
			expected: makeEnforceableTargetConfig(true, expressionTwo, nil),
		},
		{
			name:     "override target config fallback",
			input:    makeEnforceableTargetConfig(true, expressionOne, makeTargetConfig(true, expressionOne, nil)),
			override: makeEnforceableTargetConfig(true, expressionOne, makeTargetConfig(true, expressionTwo, nil)),
			expected: makeEnforceableTargetConfig(true, expressionOne, makeTargetConfig(true, expressionTwo, nil)),
		},
		{
			name:     "override target config fallback",
			input:    makeEnforceableTargetConfig(true, expressionOne, nil),
			override: makeEnforceableTargetConfig(true, expressionOne, makeTargetConfig(true, expressionTwo, nil)),
			expected: makeEnforceableTargetConfig(true, expressionOne, makeTargetConfig(true, expressionTwo, nil)),
		},
		{
			name:     "override target config enforced - should be true if any are true",
			input:    makeEnforceableTargetConfig(true, expressionOne, nil),
			override: makeEnforceableTargetConfig(false, expressionOne, nil),
			expected: makeEnforceableTargetConfig(true, expressionOne, nil),
		},
		{
			name:     "override target config enforced - should be true if any are true",
			input:    makeEnforceableTargetConfig(false, expressionOne, nil),
			override: makeEnforceableTargetConfig(true, expressionOne, nil),
			expected: makeEnforceableTargetConfig(true, expressionOne, nil),
		},
		{
			name:     "override target config enforced - should be false if both are false",
			input:    makeEnforceableTargetConfig(false, expressionOne, nil),
			override: makeEnforceableTargetConfig(false, expressionOne, nil),
			expected: makeEnforceableTargetConfig(false, expressionOne, nil),
		},
		{
			name:     "override enforced, method and fallback",
			input:    makeEnforceableTargetConfig(false, expressionOne, makeTargetConfig(true, expressionOne, nil)),
			override: makeEnforceableTargetConfig(true, expressionTwo, makeTargetConfig(true, expressionTwo, nil)),
			expected: makeEnforceableTargetConfig(true, expressionTwo, makeTargetConfig(true, expressionTwo, nil)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := mergeEnforceableTargetConfigs(test.input, test.override)

			if !reflect.DeepEqual(output, test.expected) {
				t.Errorf("expected %v, got %v", spew.Sdump(test.expected), spew.Sdump(output))
			}
		})
	}
}
