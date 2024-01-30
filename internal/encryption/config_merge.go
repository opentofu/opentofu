package encryption

import "github.com/hashicorp/hcl/v2"

func (cfg *Config) ApplyOverrides(override *Config) {
	for name := range override.KeyProviders {
		cfg.KeyProviders[name] = mergeBody(
			cfg.KeyProviders[name],
			override.KeyProviders[name],
		)
	}

	for name := range override.Methods {
		cfg.Methods[name] = mergeBody(
			cfg.Methods[name],
			override.Methods[name],
		)
	}

	for name, overrideTarget := range override.Targets {
		if base, ok := cfg.Targets[name]; ok {
			base.ApplyOverrides(overrideTarget)
		} else {
			cfg.Targets[name] = overrideTarget
		}
	}

	if override.RemoteTargets != nil {
		if cfg.RemoteTargets != nil {
			cfg.RemoteTargets.ApplyOverrides(override.RemoteTargets)
		} else {
			cfg.RemoteTargets = override.RemoteTargets
		}
	}
}

func (cfg *TargetConfig) ApplyOverrides(override *TargetConfig) {
	cfg.Enforced = cfg.Enforced || override.Enforced
	if len(override.Method) != 0 {
		cfg.Method = override.Method
	}
	if override.Fallback != nil {
		if cfg.Fallback != nil {
			cfg.Fallback.ApplyOverrides(override.Fallback)
		} else {
			cfg.Fallback = override.Fallback
		}
	}
}

func (cfg *RemoteTargetsConfig) ApplyOverrides(override *RemoteTargetsConfig) {
	if override.Default != nil {
		if cfg.Default != nil {
			cfg.Default.ApplyOverrides(override.Default)
		} else {
			cfg.Default = override.Default
		}
	}

	for name, overrideTarget := range cfg.Targets {
		if base, ok := cfg.Targets[name]; ok {
			base.ApplyOverrides(overrideTarget)
		} else {
			cfg.Targets[name] = overrideTarget
		}
	}
}

func mergeBody(base hcl.Body, override hcl.Body) hcl.Body {
	if base == nil {
		return override
	}

	panic("TODO - see internal/configs/module_merge.go:MergeBodies()")
}
