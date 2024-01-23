package encryptionconfig

import "maps"

// injectDefaultNamesIfNotSet sets the default key provider name and method name if encryption is enabled, but the
// names are not explicitly set.
//
// Default values:
//
//   - Key provider: passphrase
//   - Method: full
//
// Note: if you specify an encryption configuration, but do not set any parameters in it, the encryption will fail due
// to the missing passphrase for the default key provider.
func injectDefaultNamesIfNotSet(config *Config) {
	if config == nil {
		return
	}

	if config.KeyProvider.Name == "" {
		config.KeyProvider.Name = KeyProviderPassphrase
	}
	if config.Method.Name == "" {
		config.Method.Name = MethodFull
	}
}

// mergeConfigs performs a merge of a number of (optional) configurations.
//
// Each argument is permitted to be nil, the result is nil if and only if all arguments are nil.
//
// Any of the configs arguments that are non-nil are consecutively deep-merged. Any configuration options
// later in the line will overwrite previous configurations if set.
//
// If you specify no otherConfigs, the defaultConfig is returned. However, the defaultConfig is not merged into the
// configs.
func mergeConfigs(defaultConfig *Config, configs ...*Config) *Config {
	var current *Config
	for _, deepMergeMe := range configs {
		current = deepMergeConfig(current, deepMergeMe)
	}
	if current != nil {
		return current
	}
	return defaultConfig
}

func deepMergeConfig(base *Config, addon *Config) *Config {
	if base == nil {
		return addon
	}
	if addon == nil {
		return base
	}

	// now we know both are non-nil
	result := Config{}
	result.KeyProvider = mergeKeyProvider(addon.KeyProvider, base.KeyProvider)
	result.Method = mergeMethod(addon.Method, base.Method)
	result.Enforced = valOrDefault(addon.Enforced, false, base.Enforced)
	return &result
}

func mergeKeyProvider(addon KeyProviderConfig, base KeyProviderConfig) KeyProviderConfig {
	return KeyProviderConfig{
		Name:   valOrDefault(addon.Name, "", base.Name),
		Config: mapMerge(base.Config, addon.Config),
	}
}

func mergeMethod(addon MethodConfig, base MethodConfig) MethodConfig {
	return MethodConfig{
		Name:   valOrDefault(addon.Name, "", base.Name),
		Config: mapMerge(base.Config, addon.Config),
	}
}

func valOrDefault[T comparable](val T, empty T, defaultVal T) T {
	if val == empty {
		return defaultVal
	} else {
		return val
	}
}

func mapMerge[K comparable, V any](base map[K]V, addon map[K]V) map[K]V {
	if len(addon) == 0 {
		return base
	}
	if len(base) == 0 {
		return addon
	}

	result := make(map[K]V)
	maps.Copy(result, base)
	maps.Copy(result, addon)
	return result
}
