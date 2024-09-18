// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
)

// MergeConfigs merges two Configs together, with the override taking precedence.
func MergeConfigs(cfg *EncryptionConfig, override *EncryptionConfig) *EncryptionConfig {
	if cfg == nil {
		return override
	}
	if override == nil {
		return cfg
	}
	return &EncryptionConfig{
		KeyProviderConfigs: mergeKeyProviderConfigs(cfg.KeyProviderConfigs, override.KeyProviderConfigs),
		MethodConfigs:      mergeMethodConfigs(cfg.MethodConfigs, override.MethodConfigs),

		State:  mergeEnforceableTargetConfigs(cfg.State, override.State),
		Plan:   mergeEnforceableTargetConfigs(cfg.Plan, override.Plan),
		Remote: mergeRemoteConfigs(cfg.Remote, override.Remote),
	}
}

func mergeMethodConfigs(configs []MethodConfig, overrides []MethodConfig) []MethodConfig {
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

func mergeKeyProviderConfigs(configs []KeyProviderConfig, overrides []KeyProviderConfig) []KeyProviderConfig {
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

func mergeTargetConfigs(cfg *TargetConfig, override *TargetConfig) *TargetConfig {
	if cfg == nil {
		return override
	}
	if override == nil {
		return cfg
	}

	merged := &TargetConfig{}

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

func mergeEnforceableTargetConfigs(cfg *EnforceableTargetConfig, override *EnforceableTargetConfig) *EnforceableTargetConfig {
	if cfg == nil {
		return override
	}
	if override == nil {
		return cfg
	}

	mergeTarget := mergeTargetConfigs(cfg.AsTargetConfig(), override.AsTargetConfig())
	return &EnforceableTargetConfig{
		Enforced: cfg.Enforced || override.Enforced,
		Method:   mergeTarget.Method,
		Fallback: mergeTarget.Fallback,
	}
}

func mergeRemoteConfigs(cfg *RemoteConfig, override *RemoteConfig) *RemoteConfig {
	if cfg == nil {
		return override
	}
	if override == nil {
		return cfg
	}

	merged := &RemoteConfig{
		Default: mergeTargetConfigs(cfg.Default, override.Default),
		Targets: make([]NamedTargetConfig, len(cfg.Targets)),
	}

	copy(merged.Targets, cfg.Targets)
	for _, overrideTarget := range override.Targets {
		found := false
		for i, t := range merged.Targets {
			found = t.Name == overrideTarget.Name
			if found {
				// gohcl does not support struct embedding
				mergeTarget := mergeTargetConfigs(t.AsTargetConfig(), overrideTarget.AsTargetConfig())
				merged.Targets[i] = NamedTargetConfig{
					Name:     t.Name,
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

	return hcl2shim.MergeBodies(base, override)
}
