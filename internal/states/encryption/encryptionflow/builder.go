package encryptionflow

import (
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"sync"
)

func NewBuilder(configKey encryptionconfig.Key) Builder {
	return &builder{
		configKey:                 configKey,
		encryptionConfigs:         make(encryptionconfig.ConfigMap),
		decryptionFallbackConfigs: make(encryptionconfig.ConfigMap),
		logger:                    logging.HCLogger(),
	}
}

// Builder is the configuration builder for an encryption flow. Use this interface to allow assembling a valid
// encryption configuration.
type Builder interface {

	// EncryptionConfiguration provides this Flow with configuration for
	// encryption and decryption.
	//
	// Note: Some errors in the configuration can only be detected once it is used.
	EncryptionConfiguration(config encryptionconfig.Config) error

	// DecryptionFallbackConfiguration provides this Flow with a fallback configuration
	// for decryption.
	//
	// Configurations are deep-merged with SourceEnv taking precedence
	// over SourceHCL.
	//
	// The configuration from SourceEnvDefault is only considered if
	// no specific configuration is present.
	//
	// Note: Some errors in the configuration can only be detected once it is used.
	DecryptionFallbackConfiguration(config encryptionconfig.Config) error

	// Build merges and validates all configurations of this Flow.
	//
	// Call this only after all configurations have been supplied via EncryptionConfiguration()
	// and DecryptionFallbackConfiguration().
	//
	// This validation is far more complete than what EncryptionConfiguration() and
	// DecryptionFallbackConfiguration() can detect, because the configurations are now
	// completely known.
	//
	// Note: Some errors in the encryption or decryption fallback configurations can only be
	// detected once they are used, especially when external key management systems are involved.
	Build() (Flow, error)
}

type builder struct {
	configKey                 encryptionconfig.Key
	encryptionConfigs         encryptionconfig.ConfigMap
	decryptionFallbackConfigs encryptionconfig.ConfigMap
	configMutex               sync.RWMutex
	logger                    hclog.Logger
}

func (m *builder) EncryptionConfiguration(config encryptionconfig.Config) error {
	if !config.Source.IsValid() {
		panic(fmt.Errorf("EncryptionConfiguration() called with invalid source value: %s. This is a bug.", config.Source))
	}
	m.logger.Trace("encryption:EncryptionConfiguration", "key", m.configKey, "source", config.Source, "config", config)
	// we just store the configuration, so we can later merge and validate it

	m.configMutex.Lock()
	defer m.configMutex.Unlock()
	m.encryptionConfigs[config.Meta] = config
	return nil
}

func (m *builder) DecryptionFallbackConfiguration(config encryptionconfig.Config) error {
	if !config.Source.IsValid() {
		panic(fmt.Errorf("DecryptionFallbackConfiguration() called with invalid source value: %s. This is a bug.", config.Source))
	}
	m.logger.Trace("encryption:DecryptionFallbackConfiguration", "key", m.configKey, "source", config.Source, "config", config)
	// we just store the configuration, so we can later merge and validate it

	m.configMutex.Lock()
	defer m.configMutex.Unlock()
	m.decryptionFallbackConfigs[config.Meta] = config
	return nil
}

func (m *builder) Build() (Flow, error) {
	m.configMutex.RLock()
	defer m.configMutex.RUnlock()

	mergedEncryptionConfig, err := m.encryptionConfigs.Merge(m.configKey)
	if mergedEncryptionConfig != nil {
		// This section is before the error handling because we always want the trace log.
		// NOTE: Do not copy this to a production flow as it will log sensitive credentials!
		m.logger.Trace("encryption:merge using encryption config", "key", m.configKey, "config", *mergedEncryptionConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to merge encryption configuration (%w)", err)
	}

	mergedDecryptionFallbackConfig, err := m.decryptionFallbackConfigs.Merge(m.configKey)
	if mergedDecryptionFallbackConfig != nil {
		// This section is before the error handling because we always want the trace log.
		// NOTE: Do not copy this to a production flow as it will log sensitive credentials!
		m.logger.Trace("encryption:merge using fallback config", "key", m.configKey, "config", *mergedDecryptionFallbackConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to merge fallback configuration (%w)", err)
	}

	return &flow{
		m.configKey,
		m.logger,
		mergedEncryptionConfig,
		mergedDecryptionFallbackConfig,
	}, nil
}
