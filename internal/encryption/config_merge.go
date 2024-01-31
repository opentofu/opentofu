package encryption

import "github.com/hashicorp/hcl/v2"

func MergeConfigs(cfg *Config, override *Config) *Config {
	merged := &Config{
		KeyProviders: make([]KeyProviderConfig, len(cfg.KeyProviders)),
		Methods:      make([]MethodConfig, len(cfg.Methods)),

		StateFile: MergeTargetConfigs(cfg.StateFile, override.StateFile),
		PlanFile:  MergeTargetConfigs(cfg.PlanFile, override.PlanFile),
		Backend:   MergeTargetConfigs(cfg.Backend, override.Backend),
		Remote:    MergeRemoteConfigs(cfg.Remote, override.Remote),
	}

	copy(merged.KeyProviders, cfg.KeyProviders)
	for _, okp := range override.KeyProviders {
		found := false
		for i, kp := range merged.KeyProviders {
			found = kp.Type == okp.Type && kp.Name == okp.Name
			if found {
				merged.KeyProviders[i] = KeyProviderConfig{
					Type: kp.Type,
					Name: kp.Name,
					Body: mergeBody(kp.Body, okp.Body),
				}
				break
			}
		}
		if !found {
			merged.KeyProviders = append(merged.KeyProviders, okp)
		}
	}

	copy(merged.Methods, cfg.Methods)
	for _, om := range override.Methods {
		found := false
		for i, m := range merged.Methods {
			found = m.Type == om.Type && m.Name == om.Name
			if found {
				merged.Methods[i] = MethodConfig{
					Type: m.Type,
					Name: m.Name,
					Body: mergeBody(m.Body, om.Body),
				}
				break
			}
		}
		if !found {
			merged.Methods = append(merged.Methods, om)
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

	if len(override.Method) != 0 {
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

// gohcl does not support struct embedding
func MergeRemoteTargetConfigs(cfg RemoteTargetConfig, override RemoteTargetConfig) RemoteTargetConfig {
	merged := RemoteTargetConfig{
		Name: cfg.Name,
	}

	merged.Enforced = cfg.Enforced || override.Enforced

	if len(override.Method) != 0 {
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
	for _, ot := range override.Targets {
		found := false
		for i, t := range merged.Targets {
			found = t.Name == ot.Name
			if found {
				merged.Targets[i] = MergeRemoteTargetConfigs(t, ot)
				break
			}
		}
		if !found {
			merged.Targets = append(merged.Targets, ot)
		}
	}

	return merged
}

func mergeBody(base hcl.Body, override hcl.Body) hcl.Body {
	if base == nil {
		return override
	}

	panic("TODO - see internal/configs/module_merge.go:MergeBodies()")
}
