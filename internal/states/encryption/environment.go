package encryption

import (
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/flow"
)

var environmentParsedSuccessfully = false

var (
	environmentEncryptionConfigs         encryptionconfig.ConfigEnvJsonStructure
	environmentDecryptionFallbackConfigs encryptionconfig.ConfigEnvJsonStructure
)

// ParseEnvironmentVariables checks the state encryption environment variables for structure and syntax errors.
//
// Call ParseEnvironmentVariables early during OpenTofu's initialization to ensure that the configuration from
// environment variables can be correctly parsed (and to cache the result).
//
// It is not an error if either of the environment variables has an empty value or is unset.
//
// If this function returns nil, Instance(), RemoteStateInstance(), StatefileInstance(), and PlanfileInstance()
// will not fail at inopportune times.
//
// If you call them without calling this function first, they will panic to remind you of your programming error.
//
// Note: You should generally avoid setting the state encryption environment variables in tests, as this may make
// tests depend on each other. Run this function without the environment variables set, obtain a suitable
// Instance(), then call EncryptionConfiguration() and/or DecryptionFallbackConfiguration() on it to
// explicitly set up configuration that would normally have come from the environment.
func ParseEnvironmentVariables() error {
	var err error

	if environmentEncryptionConfigs, err = encryptionconfig.EncryptionConfigurationsFromEnv(); err != nil {
		return err
	}

	if environmentDecryptionFallbackConfigs, err = encryptionconfig.FallbackConfigurationsFromEnv(); err != nil {
		return err
	}

	environmentParsedSuccessfully = true

	return nil
}

func applyEncryptionConfigIfExists(flow flow.Flow, source flow.ConfigurationSource, configKey string) error {
	config, ok := environmentEncryptionConfigs[configKey]
	if !ok {
		logging.HCLogger().Trace("nothing to apply from environment variable for encryption",
			"source", source, "configKey", configKey)
		return nil
	}

	err := flow.EncryptionConfiguration(source, config)
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

func applyDecryptionFallbackConfigIfExists(flow flow.Flow, source flow.ConfigurationSource, configKey string) error {
	config, ok := environmentDecryptionFallbackConfigs[configKey]
	if !ok {
		logging.HCLogger().Trace("nothing to apply from environment variable for decryption fallback",
			"source", source, "configKey", configKey)
		return nil
	}

	err := flow.DecryptionFallbackConfiguration(source, config)
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
