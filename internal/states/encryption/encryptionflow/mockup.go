package encryptionflow

import (
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

// MockUpLoggingFlow will be removed when we replace it with the real implementation.
type MockUpLoggingFlow struct {
	configKey                 string
	encryptionConfigs         map[ConfigurationSource]encryptionconfig.Config
	decryptionFallbackConfigs map[ConfigurationSource]encryptionconfig.Config
	logger                    hclog.Logger
}

func NewMock(configKey string) Flow {
	return &MockUpLoggingFlow{
		configKey:                 configKey,
		encryptionConfigs:         make(map[ConfigurationSource]encryptionconfig.Config),
		decryptionFallbackConfigs: make(map[ConfigurationSource]encryptionconfig.Config),
		logger:                    logging.HCLogger(),
	}
}

func (m *MockUpLoggingFlow) DecryptState(payload []byte) ([]byte, error) {
	m.logger.Trace("encryption:DecryptState", "key", m.configKey, "payloadSize", len(payload))
	if err := m.MergeAndValidateConfigurations(); err != nil {
		return []byte{}, err
	}
	return payload, nil
}

func (m *MockUpLoggingFlow) EncryptState(state []byte) ([]byte, error) {
	m.logger.Trace("encryption:EncryptState", "key", m.configKey, "stateSize", len(state))
	if err := m.MergeAndValidateConfigurations(); err != nil {
		return []byte{}, err
	}
	return state, nil
}

func (m *MockUpLoggingFlow) DecryptPlan(payload []byte) ([]byte, error) {
	m.logger.Trace("encryption:DecryptPlan", "key", m.configKey, "payloadSize", len(payload))
	if err := m.MergeAndValidateConfigurations(); err != nil {
		return []byte{}, err
	}
	return payload, nil
}

func (m *MockUpLoggingFlow) EncryptPlan(plan []byte) ([]byte, error) {
	m.logger.Trace("encryption:EncryptPlan", "key", m.configKey, "planSize", len(plan))
	if err := m.MergeAndValidateConfigurations(); err != nil {
		return []byte{}, err
	}
	return plan, nil
}

func (m *MockUpLoggingFlow) EncryptionConfiguration(source ConfigurationSource, config encryptionconfig.Config) error {
	if !source.IsValid() {
		panic("EncryptionConfiguration() called with invalid source value. This is a bug.")
	}
	m.logger.Trace("encryption:EncryptionConfiguration", "key", m.configKey, "source", source, "config", config)
	// for this simple mock, we just store the configuration, so we can later merge and validate it
	m.encryptionConfigs[source] = config
	return nil
}

func (m *MockUpLoggingFlow) DecryptionFallbackConfiguration(source ConfigurationSource, config encryptionconfig.Config) error {
	if !source.IsValid() {
		panic("DecryptionFallbackConfiguration() called with invalid source value. This is a bug.")
	}
	m.logger.Trace("encryption:DecryptionFallbackConfiguration", "key", m.configKey, "source", source, "config", config)
	// for this simple mock, we just store the configuration, so we can later merge and validate it
	m.decryptionFallbackConfigs[source] = config
	return nil
}

func (m *MockUpLoggingFlow) MergeAndValidateConfigurations() error {
	// this logic will appear in a similar form in the actual flow implementation. For now, we just merge
	// and validate the configuration.

	mergedEncryptionConfig := encryptionconfig.MergeConfigs(
		configOrNil(m.encryptionConfigs, ConfigurationSourceEnvDefault),
		configOrNil(m.encryptionConfigs, ConfigurationSourceCode),
		configOrNil(m.encryptionConfigs, ConfigurationSourceEnv),
	)
	encryptionconfig.InjectDefaultNamesIfUnset(mergedEncryptionConfig)

	mergedDecryptionFallbackConfig := encryptionconfig.MergeConfigs(
		configOrNil(m.decryptionFallbackConfigs, ConfigurationSourceEnvDefault),
		configOrNil(m.decryptionFallbackConfigs, ConfigurationSourceCode),
		configOrNil(m.decryptionFallbackConfigs, ConfigurationSourceEnv),
	)
	encryptionconfig.InjectDefaultNamesIfUnset(mergedDecryptionFallbackConfig)

	if mergedEncryptionConfig != nil {
		m.logger.Trace("encryption:MergeAndValidateConfigurations using encryption config", "key", m.configKey, "config", *mergedEncryptionConfig)
		if err := mergedEncryptionConfig.Validate(); err != nil {
			return fmt.Errorf("error invalid encryption configuration after merge: %s", err.Error())
		}
	}
	if mergedDecryptionFallbackConfig != nil {
		m.logger.Trace("encryption:MergeAndValidateConfigurations using fallback config", "key", m.configKey, "config", *mergedDecryptionFallbackConfig)
		if err := mergedDecryptionFallbackConfig.Validate(); err != nil {
			return fmt.Errorf("error invalid decryption fallback configuration after merge: %s", err.Error())
		}
	}

	return nil
}

func configOrNil(configs map[ConfigurationSource]encryptionconfig.Config, source ConfigurationSource) *encryptionconfig.Config {
	conf, ok := configs[source]
	if ok {
		return &conf
	} else {
		return nil
	}
}
