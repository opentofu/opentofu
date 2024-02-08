package encryption

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/zclconf/go-cty/cty"
)

func TestMergeMethodConfigs(t *testing.T) {
	makeMethodConfig := func(typeName, name, key, value string) MethodConfig {
		return MethodConfig{
			Type: typeName,
			Name: name,
			Body: configs.SynthBody("method", map[string]cty.Value{
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
			output := MergeMethodConfigs(test.input, test.override)

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
			Body: configs.SynthBody("key_provider", map[string]cty.Value{
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
			output := MergeKeyProviderConfigs(test.input, test.override)

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
