package encryptionconfig

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type KeyProviderConfigValidationFunction func(config KeyProviderConfig) error

var keyProviderConfigValidation = make(map[KeyProviderName]KeyProviderConfigValidationFunction)

// RegisterKeyProviderConfigValidationFunction allows registering config validation for additional key providers.
//
// The default key providers KeyProviderPassphrase and KeyProviderDirect are automatically registered.
//
// Note: You must also register the key provider with the encryption flow, or it will still not be available.
func RegisterKeyProviderConfigValidationFunction(name KeyProviderName, validator KeyProviderConfigValidationFunction) error {
	_, conflict := keyProviderConfigValidation[name]
	if conflict {
		return fmt.Errorf("duplicate registration for key provider %s", name)
	}
	if validator == nil {
		return fmt.Errorf("missing validator during registration for key provider %s: nil", name)
	}
	keyProviderConfigValidation[name] = validator
	return nil
}

type EncryptionMethodConfigValidationFunction func(config EncryptionMethodConfig) error

var encryptionMethodConfigValidation = make(map[EncryptionMethodName]EncryptionMethodConfigValidationFunction)

// RegisterEncryptionMethodConfigValidationFunction allows registering config validation for additional encryption methods.
//
// The default encryption method EncryptionMethodFull is automatically registered.
//
// Note: You must also register the method with the encryption flow, or it will still not be available.
func RegisterEncryptionMethodConfigValidationFunction(name EncryptionMethodName, validator EncryptionMethodConfigValidationFunction) error {
	_, conflict := encryptionMethodConfigValidation[name]
	if conflict {
		return fmt.Errorf("duplicate registration for encryption method %s", name)
	}
	if validator == nil {
		return fmt.Errorf("missing validator during registration for encryption method %s: nil", name)
	}
	encryptionMethodConfigValidation[name] = validator
	return nil
}

// validation for the built-in key providers and encryption methods

func validateKPPassphraseConfig(k KeyProviderConfig) error {
	phrase, ok := k.Config["passphrase"]
	if !ok || phrase == "" {
		return errors.New("passphrase missing or empty")
	}

	if len(k.Config) > 1 {
		return errors.New("unexpected additional configuration fields, only 'passphrase' is allowed for this key provider")
	}

	return nil
}

func validateKPDirectConfig(k KeyProviderConfig) error {
	keyStr, ok := k.Config["key"]
	if !ok || keyStr == "" {
		return errors.New("field 'key' missing or empty")
	}

	key, err := hex.DecodeString(keyStr)
	if err != nil || len(key) != 32 {
		return errors.New("field 'key' is not a hex string representing 32 bytes")
	}

	if len(k.Config) > 1 {
		return errors.New("unexpected additional configuration fields, only 'key' is allowed for this key provider")
	}

	return nil
}

func validateEMFullConfig(m EncryptionMethodConfig) error {
	if len(m.Config) > 0 {
		return errors.New("unexpected fields, this method needs no configuration")
	}
	// no config fields
	return nil
}

func init() {
	_ = RegisterKeyProviderConfigValidationFunction(KeyProviderPassphrase, validateKPPassphraseConfig)
	_ = RegisterKeyProviderConfigValidationFunction(KeyProviderDirect, validateKPDirectConfig)

	_ = RegisterEncryptionMethodConfigValidationFunction(EncryptionMethodFull, validateEMFullConfig)
}
