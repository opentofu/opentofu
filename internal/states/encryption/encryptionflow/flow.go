package encryptionflow

import (
	"encoding/json"
	"errors"
	"fmt"
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

// flow is the main implementation of both PlanFlow and StateFlow.
type flow struct {
	configKey        encryptionconfig.Key
	logger           hclog.Logger
	encryptionConfig *encryptionconfig.Config
	fallbackConfig   *encryptionconfig.Config
}

var (
	errNotEncrypted   = errors.New("not encrypted")
	errNotJson        = errors.New("not a json object")
	errMethodJsonOnly = errors.New("encryption method can only be used on json documents - not suitable for plans")
)

func (e *flow) DecryptState(state []byte) ([]byte, error) {
	e.logger.Trace("encryption:DecryptState", "key", e.configKey, "stateSize", len(state))
	return e.decrypt(state, true)
}

func (e *flow) EncryptState(state []byte) ([]byte, error) {
	e.logger.Trace("encryption:EncryptState", "key", e.configKey, "stateSize", len(state))
	return e.encrypt(state, true)
}

func (e *flow) DecryptPlan(payload []byte) ([]byte, error) {
	e.logger.Trace("encryption:DecryptPlan", "key", e.configKey, "payloadSize", len(payload))
	return e.decrypt(payload, false)
}

func (e *flow) EncryptPlan(plan []byte) ([]byte, error) {
	e.logger.Trace("encryption:EncryptPlan", "key", e.configKey, "planSize", len(plan))
	return e.encrypt(plan, false)
}

func (e *flow) tryDecryptionWithConfig(encrypted EncryptedDocument, info *EncryptionInfo, config *encryptionconfig.Config) ([]byte, error) {
	deepCopiedConfig := e.deepCopyConfig(config)

	method, err := constructMethod(config.Method.Name)
	if err != nil {
		return []byte{}, err
	}
	keyProvider, err := constructKeyProvider(config.KeyProvider.Name)
	if err != nil {
		return []byte{}, err
	}

	return method.Decrypt(encrypted, info, *deepCopiedConfig, keyProvider)
}

func (e *flow) tryEncryptionWithConfig(payload []byte, config *encryptionconfig.Config) ([]byte, error) {
	deepCopiedConfig := e.deepCopyConfig(config)

	method, err := constructMethod(config.Method.Name)
	if err != nil {
		return []byte{}, err
	}
	keyProvider, err := constructKeyProvider(config.KeyProvider.Name)
	if err != nil {
		return []byte{}, err
	}

	info := EncryptionInfo{
		Version: 1,
		Method: MethodInfo{
			Name: config.Method.Name,
		},
	}

	encrypted, err := method.Encrypt(payload, &info, *deepCopiedConfig, keyProvider)
	if err != nil {
		return []byte{}, err
	}

	return e.marshalEncrypted(encrypted, &info)
}

func (e *flow) unmarshalEncrypted(payload []byte) (EncryptedDocument, *EncryptionInfo, error) {
	document := make(map[string]any)

	err := json.Unmarshal(payload, &document)
	if err != nil {
		e.logger.Trace("encryption:unmarshalEncrypted not json - not encrypted")
		return nil, nil, errNotJson
	}

	infoRaw, ok := document[EncryptionTopLevelJsonKey]
	if !ok {
		e.logger.Trace("encryption:unmarshalEncrypted not encrypted")
		return nil, nil, errNotEncrypted
	}
	delete(document, EncryptionTopLevelJsonKey)

	info, err := e.convertToEncryptionInfo(infoRaw)
	return document, info, err
}

func (e *flow) convertToEncryptionInfo(raw any) (*EncryptionInfo, error) {
	// re-marshal into the data structure we need (it's small enough)
	infoJson, err := json.Marshal(raw)
	if err != nil {
		e.logger.Trace("encryption:convertToEncryptionInfo info marshal failed", "err", err.Error())
		return nil, fmt.Errorf("encryption marker in payload has invalid structure: %s", err.Error())
	}
	info := EncryptionInfo{}
	err = json.Unmarshal(infoJson, &info)
	if err != nil {
		e.logger.Trace("encryption:convertToEncryptionInfo info unmarshal failed", "err", err.Error())
		return nil, fmt.Errorf("encryption marker in payload has invalid structure: %s", err.Error())
	}
	return &info, nil
}

func (e *flow) marshalEncrypted(encrypted EncryptedDocument, info *EncryptionInfo) ([]byte, error) {
	_, ok := encrypted[EncryptionTopLevelJsonKey]
	if ok {
		return []byte{}, fmt.Errorf("encryption internal error, reserved key '%s' is not empty in encrypted "+
			"document produced by method '%s' - this is a bug in the encryption method",
			EncryptionTopLevelJsonKey, info.Method.Name)
	}

	encrypted[EncryptionTopLevelJsonKey] = *info

	return json.MarshalIndent(encrypted, "", "  ")
}

func mapCopy(original map[string]string) map[string]string {
	if original == nil {
		return nil
	}
	result := make(map[string]string)
	for k, v := range original {
		result[k] = v
	}
	return result
}

func (e *flow) deepCopyConfig(config *encryptionconfig.Config) *encryptionconfig.Config {
	return &encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Name:   config.KeyProvider.Name,
			Config: mapCopy(config.KeyProvider.Config),
		},
		Method: encryptionconfig.MethodConfig{
			Name:   config.Method.Name,
			Config: mapCopy(config.Method.Config),
		},
		Enforced: config.Enforced,
	}
}

func (e *flow) decrypt(payload []byte, tryFallback bool) ([]byte, error) {
	e.logger.Trace("encryption:decrypt", "key", e.configKey, "payloadSize", len(payload))

	doc, info, err := e.unmarshalEncrypted(payload)
	if err != nil {
		if errors.Is(err, errNotJson) {
			e.logger.Trace("found state or plan that is not json, therefore not encrypted, passing through to avoid changing behaviour", "key", e.configKey)
			return payload, nil
		}
		if errors.Is(err, errNotEncrypted) {
			e.logger.Info("found unencrypted state, passing through (possibly initial encryption)", "key", e.configKey)
			return payload, nil
		}

		e.logger.Trace("encryption:decrypt unmarshalEncrypted failed", "key", e.configKey, "error", err.Error())
		return []byte{}, err
	}

	if e.encryptionConfig != nil {
		e.logger.Trace("encryption:decrypt trying primary configuration", "key", e.configKey, "method", e.encryptionConfig.Method.Name, "key_provider", e.encryptionConfig.KeyProvider.Name)

		state, err := e.tryDecryptionWithConfig(doc, info, e.encryptionConfig)
		if err != nil {
			e.logger.Trace("encryption:decrypt primary configuration failed to decrypt", "key", e.configKey, "error", err.Error())
			firstError := err

			e.logger.Trace("")
			if tryFallback && e.fallbackConfig != nil {
				e.logger.Trace("encryption:decrypt now trying fallback configuration", "key", e.configKey, "method", e.fallbackConfig.Method.Name, "key_provider", e.fallbackConfig.KeyProvider.Name)
				state, err := e.tryDecryptionWithConfig(doc, info, e.fallbackConfig)
				if err != nil {
					e.logger.Trace("encryption:decrypt failed to decrypt state with fallback configuration", "key", e.configKey, "error", err.Error())
					e.logger.Error("failed to decrypt state with both primary and fallback configuration, bailing out", "key", e.configKey, "error", firstError.Error())
					return []byte{}, firstError
				}

				e.logger.Trace("encryption:decrypt fallback configuration success", "key", e.configKey)
				return state, nil
			}

			return []byte{}, firstError
		} else {
			e.logger.Trace("encryption:decrypt primary configuration success", "key", e.configKey)
			return state, nil
		}
	}

	if tryFallback && e.fallbackConfig != nil {
		e.logger.Trace("encryption:decrypt trying fallback configuration with no primary configuration", "key", e.configKey, "method", e.fallbackConfig.Method.Name, "key_provider", e.fallbackConfig.KeyProvider.Name)

		state, err := e.tryDecryptionWithConfig(doc, info, e.fallbackConfig)
		if err != nil {
			e.logger.Trace("encryption:decrypt fallback configuration failed to decrypt", "key", e.configKey, "error", err.Error())
			e.logger.Error("failed to decrypt state or plan with fallback configuration and no primary configuration present, bailing out", "key", e.configKey, "error", err.Error())
			return []byte{}, err
		} else {
			e.logger.Trace("encryption:decrypt fallback configuration success", "key", e.configKey)
			return state, nil
		}
	}

	e.logger.Trace("encryption:decrypt input is encrypted but neither primary nor fallback configuration present", "key", e.configKey)
	return []byte{}, errors.New("failed to decrypt encrypted state or plan - completely missing configuration, maybe forgot to set environment variables")
}

func (e *flow) encrypt(stateOrPlan []byte, isJson bool) ([]byte, error) {
	e.logger.Trace("encryption:encrypt", "key", e.configKey, "size", len(stateOrPlan))

	if e.encryptionConfig == nil {
		// this cannot happen if required = true, because then there WOULD be an encConfig, because required is a field inside it
		e.logger.Trace("encryption:encrypt no configuration, passing through", "key", e.configKey)
		return stateOrPlan, nil
	}

	if !isJson && methodIsJsonOnly(e.encryptionConfig.Method.Name) {
		e.logger.Trace("encryption:encrypt method does not apply to non-json payloads", "key", e.configKey)
		return []byte{}, errMethodJsonOnly
	}

	encrypted, err := e.tryEncryptionWithConfig(stateOrPlan, e.encryptionConfig)
	if err != nil {
		e.logger.Trace("encryption:encrypt failed to encrypt", "key", e.configKey, "error", err.Error())
		return []byte{}, err
	}

	e.logger.Trace("encryption:encrypt success", "key", e.configKey, "outputSize", len(encrypted))
	return encrypted, nil
}
