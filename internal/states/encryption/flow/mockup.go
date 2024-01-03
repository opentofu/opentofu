package flow

import (
	"github.com/hashicorp/go-hclog"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

// MockUpLoggingFlow will be removed when we replace it with the real implementation.
type MockUpLoggingFlow struct {
	configKey string
	logger    hclog.Logger
}

func NewMock(configKey string) Flow {
	return &MockUpLoggingFlow{
		configKey: configKey,
		logger:    logging.HCLogger(),
	}
}

func (m *MockUpLoggingFlow) DecryptState(payload []byte) ([]byte, error) {
	m.logger.Trace("encryption:DecryptState", "payloadSize", len(payload))
	return payload, nil
}

func (m *MockUpLoggingFlow) EncryptState(state []byte) ([]byte, error) {
	m.logger.Trace("encryption:EncryptState", "stateSize", len(state))
	return state, nil
}

func (m *MockUpLoggingFlow) DecryptPlan(payload []byte) ([]byte, error) {
	m.logger.Trace("encryption:DecryptPlan", "payloadSize", len(payload))
	return payload, nil
}

func (m *MockUpLoggingFlow) EncryptPlan(plan []byte) ([]byte, error) {
	m.logger.Trace("encryption:EncryptPlan", "planSize", len(plan))
	return plan, nil
}

func (m *MockUpLoggingFlow) EncryptionConfiguration(source ConfigurationSource, config encryptionconfig.Config) error {
	m.logger.Trace("encryption:EncryptionConfiguration", "source", source, "config", config)
	return nil
}

func (m *MockUpLoggingFlow) DecryptionFallbackConfiguration(source ConfigurationSource, config encryptionconfig.Config) error {
	m.logger.Trace("encryption:DecryptionFallbackConfiguration", "source", source, "config", config)
	return nil
}
