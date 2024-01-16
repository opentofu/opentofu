package tf

import (
	"errors"
	"fmt"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/zclconf/go-cty/cty"
)

// TODO this is not completely correct - see the one failing unit test to see the problem
// (internal/command/e2etest/remote_state_test.go:13)

// the block, if defined like this, is not optional, so the operation fails with
//    Provider "provider[\"terraform.io/builtin/terraform\"]" produced an invalid
//    value for data.terraform_remote_state.test: missing required attribute
//    "state_decryption_fallback".

func StateEncryptionConfigSchema() *configschema.NestedBlock {
	return &configschema.NestedBlock{
		Block: configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"required": {
					Type: cty.Bool,
					Description: "Enforce state encryption while writing state.\n\n" +
						"If set, tofu will fail rather than write unencrypted state, " +
						"e.g. due to missing configuration.",
					DescriptionKind: configschema.StringMarkdown,
					Optional:        true,
				},
			},
			BlockTypes: map[string]*configschema.NestedBlock{
				"key_provider": KeyProviderConfigSchema(),
				"method":       MethodConfigSchema(),
			},
			Description:     "Configures client-side state encryption.",
			DescriptionKind: configschema.StringMarkdown,
		},
		// TODO setting this to NestingSingle does not allow omitting the entire block,
		// TODO even though the documentation suggests it should unless we also set MinItems and MaxItems to 1???
		Nesting: configschema.NestingSingle,
	}
}

func StateDecryptionFallbackConfigSchema() *configschema.NestedBlock {
	return &configschema.NestedBlock{
		Block: configschema.Block{
			BlockTypes: map[string]*configschema.NestedBlock{
				"key_provider": KeyProviderConfigSchema(),
				"method":       MethodConfigSchema(),
			},
			Description: "Configures client-side state decryption fallback.\n\n" +
				"This configuration is tried for decrypting state if it cannot" +
				"be decrypted using the primary encryption configuraiton. " +
				"Useful for situations like key rotation.",
			DescriptionKind: configschema.StringMarkdown,
		},
		// TODO setting this to NestingSingle does not allow omitting the entire block,
		// TODO even though the documentation suggests it should unless we also set MinItems and MaxItems to 1???
		Nesting: configschema.NestingSingle,
	}
}

func KeyProviderConfigSchema() *configschema.NestedBlock {
	return &configschema.NestedBlock{
		Block: configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"name": {
					Type:            cty.String,
					Description:     "The key provider to use, e.g. `passphrase` (the default) or `direct`.",
					DescriptionKind: configschema.StringMarkdown,
					Optional:        true,
				},
				"config": {
					Type: cty.DynamicPseudoType,
					Description: "The configuration of the key provider. " +
						"Although this is optional, most key providers require " +
						"some configuration.\n\n" +
						"You can overwrite or add values using environment variables.",
					DescriptionKind: configschema.StringMarkdown,
					Optional:        true,
				},
			},
		},
		Nesting: configschema.NestingSingle,
	}
}

func MethodConfigSchema() *configschema.NestedBlock {
	return &configschema.NestedBlock{
		Block: configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"name": {
					Type:            cty.String,
					Description:     "The encryption method to use, e.g. `full` (the default).",
					DescriptionKind: configschema.StringMarkdown,
					Optional:        true,
				},
				"config": {
					Type: cty.DynamicPseudoType,
					Description: "The configuration of the encryption method. " +
						"Although this is optional, some encryption methods may require " +
						"some configuration.\n\n" +
						"You can overwrite or add values using environment variables.",
					DescriptionKind: configschema.StringMarkdown,
					Optional:        true,
				},
			},
		},
		Nesting: configschema.NestingSingle,
	}
}

func ParseEncryptionConfig(cfg cty.Value) (encryptionconfig.Config, error) {
	result := encryptionconfig.Config{}

	// TODO this is not completely correct

	keyProvider := cfg.GetAttr("key_provider")
	if !keyProvider.IsNull() {
		result.KeyProvider = encryptionconfig.KeyProviderConfig{}

		name := keyProvider.GetAttr("name")
		if !name.IsNull() {
			if name.Type() != cty.String {
				return encryptionconfig.Config{}, errors.New("key provider name must be a string")
			}
			result.KeyProvider.Name = encryptionconfig.KeyProviderName(name.AsString())
			if err := result.KeyProvider.NameValid(); err != nil {
				return encryptionconfig.Config{}, err
			}
		}

		configParams := keyProvider.GetAttr("config")
		if !configParams.IsNull() {
			converted, err := toConfigParamsMap(configParams, "key_provider.config")
			if err != nil {
				return encryptionconfig.Config{}, err
			}
			result.KeyProvider.Config = converted
		}
	}

	method := cfg.GetAttr("method")
	if !method.IsNull() {
		result.Method = encryptionconfig.EncryptionMethodConfig{}

		name := method.GetAttr("name")
		if !name.IsNull() {
			if name.Type() != cty.String {
				return encryptionconfig.Config{}, errors.New("key provider name must be a string")
			}
			result.Method.Name = encryptionconfig.EncryptionMethodName(name.AsString())
			if err := result.Method.NameValid(); err != nil {
				return encryptionconfig.Config{}, err
			}
		}

		configParams := method.GetAttr("config")
		if !configParams.IsNull() {
			converted, err := toConfigParamsMap(configParams, "method.config")
			if err != nil {
				return encryptionconfig.Config{}, err
			}
			result.Method.Config = converted
		}
	}

	required := cfg.GetAttr("required")
	if !required.IsNull() && required.Equals(cty.True).True() {
		result.Required = true
	}

	return result, nil
}

func toConfigParamsMap(cfg cty.Value, field string) (map[string]string, error) {
	// gocty.FromCtyValue produces horrible error messages, so let's do it by hand
	result := make(map[string]string)
	if cfg.CanIterateElements() {
		for k, v := range cfg.AsValueMap() {
			if v.IsNull() || v.Type() != cty.String {
				return nil, fmt.Errorf("invalid value for key '%s' in %s, must be a string", k, field)
			} else {
				result[k] = v.AsString()
			}
		}
		return result, nil
	} else {
		return nil, fmt.Errorf("%s must be a map", field)
	}
}
