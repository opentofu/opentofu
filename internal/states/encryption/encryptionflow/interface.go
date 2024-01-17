// Package encryptionflow contains the top-level flow for client-side state encryption and the interfaces an encryption
// or key derivation method need to implement.
package encryptionflow

import "github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"

type ConfigurationSource string

const (
	ConfigurationSourceEnvDefault ConfigurationSource = "env-default"
	ConfigurationSourceCode       ConfigurationSource = "code"
	ConfigurationSourceEnv        ConfigurationSource = "env"
)

func (s ConfigurationSource) IsValid() bool {
	return s == ConfigurationSourceEnvDefault || s == ConfigurationSourceCode || s == ConfigurationSourceEnv
}

// IsForExternalUse checks that a ConfigurationSource is suitable for production-code outside this package tree.
//
// Provided mainly for documentation purposes.
//
// All other ConfigurationSource values are used internally, though you may need them when writing tests,
// which is why they are exported.
func (s ConfigurationSource) IsForExternalUse() bool {
	return s == ConfigurationSourceCode
}

// EncryptionTopLevelJsonKey is the json key used to mark state or plans as encrypted.
//
// Changing this will invalidate all existing encrypted state, so migration code would have to be added
// that can deal with the old key.
//
// Unencrypted state must not contain this key, so you cannot choose any of the top-level keys of the current
// state data structure (currently stateV4).
var EncryptionTopLevelJsonKey = "encryption"

// Flow represents the top-level state or plan encryption/decryption flow
// for a particular encryption configuration.
//
// Instances of Flow are kept in an internal singleton cache per configuration key.
// Unless you are writing tests, you should not create them in your code.
//
// See also encryption.GetSingleton().
type Flow interface {
	// DecryptState decrypts encrypted state.
	//
	// If the state is not actually encrypted, it will be returned
	// as-is. DecryptState will first try decryption using the state
	// encryption configuration. If this fails, it tries the decryption
	// fallback configuration. If neither produces a valid result it fails.
	//
	// payload must be a json document passed in as a []byte.
	// This can be encrypted or unencrypted state. Encryption is detected
	// by presence of the EncryptionTopLevelJsonKey key at the 1st level.
	//
	// If no error is returned, then the first return value will always
	// be a json document, possibly the same one passed in as payload
	// if the payload was not actually encrypted.
	DecryptState(payload []byte) ([]byte, error)

	// EncryptState encrypts plaintext state.
	//
	// There are special configurations that will not actually encrypt
	// state. This happens when you only configure a decryption fallback,
	// but not encryption. This is not an error.
	//
	// state is a json document passed in as a []byte. It is an error
	// if this document already contains the EncryptionTopLevelJsonKey key
	// at the 1st level.
	//
	// If no error is returned, then the first return value will always
	// be a json document as a []byte. If encryption took place,
	// this json document will have the EncryptionTopLevelJsonKey key
	// at the 1st level.
	EncryptState(state []byte) ([]byte, error)

	// DecryptPlan decrypts an encrypted plan.
	//
	// The presence of encryption is detected by attempting to parse
	// payload as a json document and looking at the EncryptionTopLevelJsonKey key
	// at the 1st level.
	//
	// If the plan is not actually encrypted, but plan encryption is configured,
	// this will fail to prevent working with invalid plans (plans are binary data).
	//
	// Note that decryption fallback configurations are not considered for plans.
	// Plans are not stored for a long time, so key rotation is not an issue for them.
	//
	// If DecryptPlan returns no error, then
	//  - either there is no configuration for plan encryption, and
	//    payload is returned as-is
	//  - or it has successfully decrypted the payload using the configuration
	//    for plan encryption.
	DecryptPlan(payload []byte) ([]byte, error)

	// EncryptPlan encrypts a plaintext plan.
	//
	// If no configuration for plan encryption is specified, the plan
	// is returned as-is. This is not an error.
	//
	// In the presence of a configuration suitable for plan encryption, EncryptPlan
	// returns a json document which contains the EncryptionTopLevelJsonKey key
	// at the 1st level.
	//
	// A configuration that is not suitable for plan encryption is treated as
	// an error. Whether a configuration is suitable for plan encryption or not is mostly determined
	// by the encryption method. For example, plans cannot be partially encrypted, because
	// they are binary data.
	EncryptPlan(plan []byte) ([]byte, error)

	// EncryptionConfiguration provides this Flow with configuration for
	// encryption and decryption.
	//
	// Configurations are deep-merged with ConfigurationSourceEnv taking precedence
	// over ConfigurationSourceCode.
	//
	// The configuration from ConfigurationSourceEnvDefault is only considered if
	// no specific configuration is present.
	//
	// Note: Some errors in the configuration can only be detected once it is used.
	EncryptionConfiguration(source ConfigurationSource, config encryptionconfig.Config) error

	// DecryptionFallbackConfiguration provides this Flow with a fallback configuration
	// for decryption.
	//
	// Configurations are deep-merged with ConfigurationSourceEnv taking precedence
	// over ConfigurationSourceCode.
	//
	// The configuration from ConfigurationSourceEnvDefault is only considered if
	// no specific configuration is present.
	//
	// Note: Some errors in the configuration can only be detected once it is used.
	DecryptionFallbackConfiguration(source ConfigurationSource, config encryptionconfig.Config) error

	// MergeAndValidateConfigurations merges and validates all configurations of this Flow.
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
	MergeAndValidateConfigurations() error
}
