package encryptionconfig

import (
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
