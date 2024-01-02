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
// If this function returns nil, Instance(), RemoteStateInstance(), StatefileInstance(), and PlanfileInstance()
// will not fail at inopportune times.
//
// Note: You should generally avoid setting the state encryption environment variables in tests, as this may make
// tests depend on each other. Just obtain a suitable Instance(), then call EncryptionConfiguration() and/or
// DecryptionFallbackConfiguration() on it to explicitly set up configuration that would normally have come from
// the environment.
func ParseEnvironmentVariables() error {
	if _, err := encryptionconfig.EncryptionConfigurationsFromEnv(); err != nil {
		return err
	}

	if _, err := encryptionconfig.FallbackConfigurationsFromEnv(); err != nil {
		return err
	}

	return nil
}

func applyEncryptionConfigIfExists(flow encryptionflow.Flow, source encryptionflow.ConfigurationSource, configKey string) error {
	configs, err := encryptionconfig.EncryptionConfigurationsFromEnv()
	if err != nil {
		return err
	}
	if configs == nil {
		logging.HCLogger().Trace("nothing to apply, environment variable for encryption is not set",
			"source", source, "configKey", configKey)
		return nil
	}

	config, ok := configs[configKey]
	if !ok {
		logging.HCLogger().Trace("nothing to apply from environment variable for encryption",
			"source", source, "configKey", configKey)
		return nil
	}

	err = flow.EncryptionConfiguration(source, config)
	if err != nil {
		logging.HCLogger().Error("encryption configuration from environment failed to apply. "+
			"This is a bug. It should not be validated, only stored at this point in time because it could still "+
			"be incomplete. Validation of the configuration can only occur once it is known to be complete.",
			"source", source, "configKey", configKey)
		logging.HCLogger().Error(err.Error())
		return err
	}

	logging.HCLogger().Trace("successfully applied config from environment variable for encryption",
		"source", source, "configKey", configKey)
	return nil
}

func applyDecryptionFallbackConfigIfExists(flow encryptionflow.Flow, source encryptionflow.ConfigurationSource, configKey string) error {
	configs, err := encryptionconfig.FallbackConfigurationsFromEnv()
	if err != nil {
		return err
	}
	if configs == nil {
		logging.HCLogger().Trace("nothing to apply, environment variable for decryption fallback is not set",
			"source", source, "configKey", configKey)
		return nil
	}

	config, ok := configs[configKey]
	if !ok {
		logging.HCLogger().Trace("nothing to apply from environment variable for decryption fallback",
			"source", source, "configKey", configKey)
		return nil
	}

	err = flow.DecryptionFallbackConfiguration(source, config)
	if err != nil {
		logging.HCLogger().Error("decryption fallback configuration from environment failed to apply. "+
			"This is a bug. It should not be validated, only stored at this point in time because it could still "+
			"be incomplete. Validation of the configuration can only occur once it is known to be complete.",
			"source", source, "configKey", configKey)
		logging.HCLogger().Error(err.Error())
		return err
	}

	logging.HCLogger().Trace("successfully applied config from environment variable for encryption",
		"source", source, "configKey", configKey)
	return nil
}
