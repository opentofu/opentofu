package encryptionconfig

// InjectDefaultNamesIfUnset sets some configuration defaults, but only if there is a configuration.
//
// If config is nil, state encryption / decryption is disabled and this function does nothing.
//
// If a config is present, but any of the names are blank, we choose sensible defaults.
//
// Note that a blank configuration with these defaults will fail because it will be missing the passphrase.
func InjectDefaultNamesIfUnset(config *Config) {
	if config != nil {
		if config.KeyProvider.Name == "" {
			config.KeyProvider.Name = KeyProviderPassphrase
		}
		if config.Method.Name == "" {
			config.Method.Name = EncryptionMethodFull
		}
	}
}

// MergeConfigs performs a merge of a number of (optional) configurations.
//
// Each argument is permitted to be nil, the result is nil if and only if all
// arguments are nil.
//
// Any of the deepMergeIncreasingPriority arguments that are non-nil are
// consecutively deep-merged.
//
// The defaultIfAllOthersNil plays a special role, it is NOT deep-merged,
// but if all other arguments are nil or missing, it will be returned.
func MergeConfigs(defaultIfAllOthersNil *Config, deepMergeIncreasingPriority ...*Config) *Config {
	var current *Config
	for _, deepMergeMe := range deepMergeIncreasingPriority {
		current = deepMergeConfig(deepMergeMe, current)
	}
	if current != nil {
		return current
	} else {
		return defaultIfAllOthersNil
	}
}

func deepMergeConfig(addon *Config, base *Config) *Config {
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
	result.Required = valOrDefault(addon.Required, false, base.Required)
	return &result
}

func mergeKeyProvider(addon KeyProviderConfig, base KeyProviderConfig) KeyProviderConfig {
	return KeyProviderConfig{
		Name:   valOrDefault(addon.Name, "", base.Name),
		Config: mapMerge(addon.Config, base.Config),
	}
}

func mergeMethod(addon EncryptionMethodConfig, base EncryptionMethodConfig) EncryptionMethodConfig {
	return EncryptionMethodConfig{
		Name:   valOrDefault(addon.Name, "", base.Name),
		Config: mapMerge(addon.Config, base.Config),
	}
}

func valOrDefault[T comparable](val T, empty T, defaultVal T) T {
	if val == empty {
		return defaultVal
	} else {
		return val
	}
}

func mapMerge[K comparable, V any](addon map[K]V, base map[K]V) map[K]V {
	if len(addon) == 0 {
		return base
	}
	if len(base) == 0 {
		return addon
	}

	result := make(map[K]V)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range addon {
		result[k] = v
	}
	return result
}
