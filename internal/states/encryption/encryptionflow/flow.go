package encryptionflow

import (
	"github.com/hashicorp/go-hclog"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

// EncryptionTopLevelJsonKey is the json key used to mark state or plans as encrypted.
//
// Changing this will invalidate all existing encrypted state, so migration code would have to be added
// that can deal with the old key.
//
// Unencrypted state must not contain this key, so you cannot choose any of the top-level keys of the current
// state data structure (currently stateV4).
const EncryptionTopLevelJsonKey = "encryption"

// New creates an encryption flow with the specified configuration.
func New(
	configKey encryptionconfig.Key,
	encryptionConfig *encryptionconfig.Config,
	fallbackConfig *encryptionconfig.Config,
	logger hclog.Logger,
) Flow {
	return &flow{
		configKey:        configKey,
		logger:           logger,
		encryptionConfig: encryptionConfig,
		fallbackConfig:   fallbackConfig,
	}
}

// Flow represents the top-level state or plan encryption/decryption flow
// for a particular encryption configuration.
//
// You can obtain a copy of this interface by creating a Builder first and supplying it with the correct
// encryption configuration, then calling the Build function.
//
// Note, that all encrypted data must be in JSON format and must contain a key specified in EncryptionTopLevelJsonKey
// to indicate that it is valid encrypted data.
type Flow interface {
	StateFlow
	PlanFlow
}

type StateFlow interface {
	// DecryptState decrypts encrypted state.
	//
	// You can pass it an encrypted or decrypted state in JSON format as a []byte. If the state is already decrypted
	// (has no EncryptionTopLevelJsonKey), the function will return the state unmodified.
	//
	// If the state is encrypted, the method will attempt to decrypt the state according to its internal configuration,
	// first with the primary encryption key, then with the fallback decryption key. If neither produces a valid result
	// it fails.
	DecryptState(payload []byte) ([]byte, error)

	// EncryptState encrypts plaintext state.
	//
	// To encrypt the state, pass the unencrypted state in JSON format to this function. This function will attempt to
	// encrypt it with the encryption key and return a valid, but encrypted JSON.
	//
	// If no encryption is configured, but "enforced" is enabled, the encryption will fail. Otherwise, this function
	// will return the state unmodified.
	//
	// Implementations must ensure that the first return value is always a valid JSON unless an error is returned,
	// because there are remote state backends that require state to be JSON.
	EncryptState(state []byte) ([]byte, error)
}

type PlanFlow interface {
	// DecryptPlan decrypts an encrypted plan.
	//
	// To decrypt an encrypted plan, pass the encrypted plan to this function. If the plan is not encrypted, but
	// encryption is configured, this function will fail. If no encryption is configured, this function will pass
	// the provided plan data through without modification.
	//
	// For plan decryption the fallback decryption configuration is not used, only the primary encryption configuration
	// applies.
	DecryptPlan(payload []byte) ([]byte, error)

	// EncryptPlan encrypts an unencrypted plan.
	//
	// To encrypt a plan, pass the plan data to this function. If no encryption is configured, the plan data is
	// passed through without modification. If encryption is configured, the data is encrypted and returned in JSON
	// format.
	//
	// A configuration that is not suitable for plan encryption results in an error. Whether a configuration is suitable
	// for plan encryption or not is determined by the encryption method. For example, plans cannot be partially
	// encrypted, because they are binary data.
	EncryptPlan(plan []byte) ([]byte, error)
}

// flow is currently not implemented and will be provided in a later pull request. The current implementation passes
// all data through.
type flow struct {
	configKey        encryptionconfig.Key
	logger           hclog.Logger
	encryptionConfig *encryptionconfig.Config
	fallbackConfig   *encryptionconfig.Config
}

func (m *flow) DecryptState(payload []byte) ([]byte, error) {
	m.logger.Trace("encryption:DecryptState", "key", m.configKey, "payloadSize", len(payload))
	return payload, nil
}

func (m *flow) EncryptState(state []byte) ([]byte, error) {
	m.logger.Trace("encryption:EncryptState", "key", m.configKey, "stateSize", len(state))
	return state, nil
}

func (m *flow) DecryptPlan(payload []byte) ([]byte, error) {
	m.logger.Trace("encryption:DecryptPlan", "key", m.configKey, "payloadSize", len(payload))
	return payload, nil
}

func (m *flow) EncryptPlan(plan []byte) ([]byte, error) {
	m.logger.Trace("encryption:EncryptPlan", "key", m.configKey, "planSize", len(plan))
	return plan, nil
}
