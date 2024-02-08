package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
)

// MergeConfigs merges two Configs together, with the override taking precedence.
func MergeConfigs(cfg *Config, override *Config) *Config {
	merged := &Config{
		KeyProviderConfigs: MergeKeyProviderConfigs(cfg.KeyProviderConfigs, override.KeyProviderConfigs),
		MethodConfigs:      MergeMethodConfigs(cfg.MethodConfigs, override.MethodConfigs),

		StateFile: MergeTargetConfigs(cfg.StateFile, override.StateFile),
		PlanFile:  MergeTargetConfigs(cfg.PlanFile, override.PlanFile),
		Backend:   MergeTargetConfigs(cfg.Backend, override.Backend),
		Remote:    MergeRemoteConfigs(cfg.Remote, override.Remote),
	}

	return merged
}

func MergeMethodConfigs(configs []MethodConfig, overrides []MethodConfig) []MethodConfig {
	// Initialize a copy of configs to preserve the original entries.
	merged := make([]MethodConfig, len(configs))
	copy(merged, configs)

	for _, override := range overrides {
		wasOverridden := false

		// Attempt to find a match based on type/name
		for i, method := range merged {
			if method.Type == override.Type && method.Name == override.Name {
				// Override the existing method.
				merged[i].Body = mergeBody(method.Body, override.Body)
				wasOverridden = true
				break
			}
		}

		// If no existing method was overridden, append the new override.
		if !wasOverridden {
			merged = append(merged, override)
		}
	}
	return merged
}

func MergeKeyProviderConfigs(configs []KeyProviderConfig, overrides []KeyProviderConfig) []KeyProviderConfig {
	// Initialize a copy of configs to preserve the original entries.
	merged := make([]KeyProviderConfig, len(configs))
	copy(merged, configs)

	for _, override := range overrides {
		wasOverridden := false

		// Attempt to find a match based on type/name
		for i, keyProvider := range merged {
			if keyProvider.Type == override.Type && keyProvider.Name == override.Name {
				// Override the existing key provider.
				merged[i].Body = mergeBody(keyProvider.Body, override.Body)
				wasOverridden = true
				break
			}
		}

		// If no existing key provider was overridden, append the new override.
		if !wasOverridden {
			merged = append(merged, override)
		}
	}
	return merged
}

func MergeTargetConfigs(cfg *TargetConfig, override *TargetConfig) *TargetConfig {
	if cfg == nil {
		return override
	}
	if override == nil {
		return cfg
	}

	merged := &TargetConfig{}

	merged.Enforced = cfg.Enforced || override.Enforced

	if override.Method != nil {
		merged.Method = override.Method
	} else {
		merged.Method = cfg.Method
	}

	if override.Fallback != nil {
		merged.Fallback = override.Fallback
	} else {
		merged.Fallback = cfg.Fallback
	}

	return merged
}

func MergeRemoteConfigs(cfg *RemoteConfig, override *RemoteConfig) *RemoteConfig {
	if cfg == nil {
		return override
	}
	if override == nil {
		return cfg
	}

	merged := &RemoteConfig{
		Default: MergeTargetConfigs(cfg.Default, override.Default),
		Targets: make([]RemoteTargetConfig, len(cfg.Targets)),
	}

	copy(merged.Targets, cfg.Targets)
	for _, overrideTarget := range override.Targets {
		found := false
		for i, t := range merged.Targets {
			found = t.Name == overrideTarget.Name
			if found {
				// gohcl does not support struct embedding
				mergeTarget := MergeTargetConfigs(t.AsTargetConfig(), overrideTarget.AsTargetConfig())
				merged.Targets[i] = RemoteTargetConfig{
					Name:     t.Name,
					Enforced: mergeTarget.Enforced,
					Method:   mergeTarget.Method,
					Fallback: mergeTarget.Fallback,
				}
				break
			}
		}
		if !found {
			merged.Targets = append(merged.Targets, overrideTarget)
		}
	}

	return merged
}

func mergeBody(base hcl.Body, override hcl.Body) hcl.Body {
	if base == nil {
		return override
	}

	if override == nil {
		return base
	}

	return configs.MergeBodies(base, override)
}
