package encryptionconfig

import (
	"errors"
	"fmt"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

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
	}
}

func ParseConfig(cfg cty.Value) (Config, error) {
	result := Config{}

	keyProvider := cfg.GetAttr("key_provider")
	if !keyProvider.IsNull() {
		result.KeyProvider = KeyProviderConfig{}

		name := keyProvider.GetAttr("name")
		if !name.IsNull() {
			if name.Type() != cty.String {
				return Config{}, errors.New("key provider name must be a string")
			}
			result.KeyProvider.Name = KeyProviderName(name.AsString())
			if err := result.KeyProvider.NameValid(); err != nil {
				return Config{}, err
			}
		}

		configParams := keyProvider.GetAttr("config")
		if !configParams.IsNull() {
			converted, err := toConfigParamsMap(configParams, "key_provider.config")
			if err != nil {
				return Config{}, err
			}
			result.KeyProvider.Config = converted
		}
	}

	method := cfg.GetAttr("method")
	if !method.IsNull() {
		result.Method = EncryptionMethodConfig{}

		name := method.GetAttr("name")
		if !name.IsNull() {
			if name.Type() != cty.String {
				return Config{}, errors.New("key provider name must be a string")
			}
			result.Method.Name = EncryptionMethodName(name.AsString())
			if err := result.Method.NameValid(); err != nil {
				return Config{}, err
			}
		}

		configParams := method.GetAttr("config")
		if !configParams.IsNull() {
			converted, err := toConfigParamsMap(configParams, "method.config")
			if err != nil {
				return Config{}, err
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
