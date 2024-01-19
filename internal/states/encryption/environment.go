package encryption

import (
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

// ParseEnvironmentVariables checks the state encryption environment variables for structure and syntax errors.
//
// Call ParseEnvironmentVariables early during OpenTofu's initialization to ensure that the configuration from
// environment variables can be correctly parsed.
//
// It is not an error if either of the environment variables has an empty value or is unset.
//
// If this function returns nil, GetSingleton(), GetRemoteStateSingleton(), GetStatefileSingleton(), and
// GetPlanfileSingleton() are much less likely to fail at inopportune times.
//
// Note: You should generally avoid setting the state encryption environment variables in tests, as this may make
// tests depend on each other. Just obtain a suitable Instance(), then call EncryptionConfiguration() and/or
// DecryptionFallbackConfiguration() on it to explicitly set up configuration that would normally have come from
// the environment.
func ParseEnvironmentVariables() error {
	if _, err := encryptionconfig.ConfigurationFromEnv(encryptionconfig.ConfigEnvName); err != nil {
		return err
	}

	if _, err := encryptionconfig.ConfigurationFromEnv(encryptionconfig.FallbackConfigEnvName); err != nil {
		return err
	}

	return nil
}

func applyEncryptionConfigIfExists(flow encryptionflow.Builder, configKey encryptionconfig.Key) error {
	configs, err := encryptionconfig.ConfigurationFromEnv(encryptionconfig.ConfigEnvName)
	if err != nil {
		return err
	}
	if configs == nil {
		logging.HCLogger().Trace("nothing to apply, environment variable for encryption is not set",
			"configKey", configKey)
		return nil
	}

	config, ok := configs[configKey]
	if !ok {
		logging.HCLogger().Trace("nothing to apply from environment variable for encryption",
			"configKey", configKey)
		return nil
	}

	err = flow.EncryptionConfiguration(config)
	if err != nil {
		logging.HCLogger().Error("encryption configuration from environment failed to apply. "+
			"This is a bug. It should not be validated, only stored at this point in time because it could still "+
			"be incomplete. Validation of the configuration can only occur once it is known to be complete.",
			"configKey", configKey)
		logging.HCLogger().Error(err.Error())
		return err
	}

	logging.HCLogger().Trace("successfully applied config from environment variable for encryption",
		"configKey", configKey)
	return nil
}

func applyDecryptionFallbackConfigIfExists(flow encryptionflow.Builder, configKey encryptionconfig.Key) error {
	configs, err := encryptionconfig.ConfigurationFromEnv(encryptionconfig.FallbackConfigEnvName)
	if err != nil {
		return err
	}
	if configs == nil {
		logging.HCLogger().Trace("nothing to apply, environment variable for decryption fallback is not set",
			"configKey", configKey)
		return nil
	}

	config, ok := configs[configKey]
	if !ok {
		logging.HCLogger().Trace("nothing to apply from environment variable for decryption fallback",
			"configKey", configKey)
		return nil
	}

	err = flow.DecryptionFallbackConfiguration(config)
	if err != nil {
		logging.HCLogger().Error("decryption fallback configuration from environment failed to apply. "+
			"This is a bug. It should not be validated, only stored at this point in time because it could still "+
			"be incomplete. Validation of the configuration can only occur once it is known to be complete.",
			"configKey", configKey)
		logging.HCLogger().Error(err.Error())
		return err
	}

	logging.HCLogger().Trace("successfully applied config from environment variable for encryption",
		"configKey", configKey)
	return nil
}
