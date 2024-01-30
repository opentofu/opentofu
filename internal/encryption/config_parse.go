package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

var configSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       ConfigKeyProvider,
			LabelNames: []string{"type", "name"},
		},
		{
			Type:       ConfigKeyMethod,
			LabelNames: []string{"type", "name"},
		},

		{Type: ConfigKeyBackend},
		{Type: ConfigKeyStateFile},
		{Type: ConfigKeyPlanFile},
		{Type: ConfigKeyRemote},
	},
}

func DecodeConfig(body hcl.Body, rng hcl.Range) (*Config, hcl.Diagnostics) {
	content, diags := body.Content(configSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	config := Config{
		KeyProviders: make(map[string]hcl.Body),
		Methods:      make(map[string]hcl.Body),
		Targets:      make(map[string]*TargetConfig),

		DeclRange: rng,
	}

	for _, block := range content.Blocks {
		ident := block.Type

		switch ident {
		case ConfigKeyProvider:
			addr := KeyProviderAddr(block.Labels[0], block.Labels[1])
			if _, ok := config.KeyProviders[addr]; ok {
				// TODO diags = append(diags, DUPE)
				continue
			}
			config.KeyProviders[addr] = block.Body

		case ConfigKeyMethod:
			addr := MethodAddr(block.Labels[0], block.Labels[1])
			if _, ok := config.Methods[addr]; ok {
				// TODO diags = append(diags, DUPE)
				continue
			}
			config.Methods[addr] = block.Body

		case ConfigKeyBackend, ConfigKeyStateFile, ConfigKeyPlanFile:
			if _, ok := config.Targets[ident]; ok {
				// TODO diags = append(diags, DUPE)
				continue
			}

			cfg, cfgDiags := decodeTargetConfig(block.Body)
			diags = append(diags, cfgDiags...)
			config.Targets[ident] = cfg

		case ConfigKeyRemote:
			if config.RemoteTargets != nil {
				// TODO diags = append(diags, DUPE)
				continue
			}

			cfg, cfgDiags := decodeRemoteTargetsConfig(block.Body)
			diags = append(diags, cfgDiags...)
			config.RemoteTargets = cfg
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}
	return &config, diags
}

const (
	targetConfigKeyMethod   = "method"
	targetConfigKeyEnforced = "enforced"
	targetConfigKeyFallback = "fallback"
)

var targetConfigSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: targetConfigKeyFallback},
	},
	Attributes: []hcl.AttributeSchema{
		{Name: targetConfigKeyMethod},
		{Name: targetConfigKeyEnforced},
	},
}

func decodeTargetConfig(body hcl.Body) (*TargetConfig, hcl.Diagnostics) {
	content, diags := body.Content(targetConfigSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	cfg := TargetConfig{}

	if attr, ok := content.Attributes[targetConfigKeyMethod]; ok {
		valDiags := gohcl.DecodeExpression(attr.Expr, nil, &cfg.Method)
		diags = append(diags, valDiags...)
	}

	if attr, ok := content.Attributes[targetConfigKeyEnforced]; ok {
		valDiags := gohcl.DecodeExpression(attr.Expr, nil, &cfg.Enforced)
		diags = append(diags, valDiags...)
	}

	if len(content.Blocks) > 1 {
		// TODO diags = append(diags, DUPE)
	}

	if len(content.Blocks) == 1 {
		fallback, fallbackDiags := decodeTargetConfig(content.Blocks[0].Body)
		diags = append(diags, fallbackDiags...)
		cfg.Fallback = fallback
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return &cfg, diags
}

const (
	remoteConfigKeyDefault = "default"
	remoteConfigKeyTarget  = "remote_data_source"
)

var remoteConfigSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type: remoteConfigKeyDefault,
		},
		{
			Type:       remoteConfigKeyTarget,
			LabelNames: []string{"ident"},
		},
	},
}

func decodeRemoteTargetsConfig(body hcl.Body) (*RemoteTargetsConfig, hcl.Diagnostics) {
	content, diags := body.Content(remoteConfigSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	cfg := RemoteTargetsConfig{}

	for _, block := range content.Blocks {
		switch block.Type {
		case remoteConfigKeyDefault:
			if cfg.Default != nil {
				// TODO diags = append(diags, DUPE)
				continue
			}
			target, targetDiags := decodeTargetConfig(block.Body)
			diags = append(diags, targetDiags...)
			cfg.Default = target
		case remoteConfigKeyTarget:
			ident := block.Labels[0]
			if _, ok := cfg.Targets[ident]; ok {
				// TODO diags = append(diags, DUPE)
				continue
			}
			target, targetDiags := decodeTargetConfig(block.Body)
			diags = append(diags, targetDiags...)
			cfg.Targets[ident] = target
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return &cfg, diags
}
