package encryptionconfig

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

type KeyProviderConfigValidationFunction func(config KeyProviderConfig) error

// RegisterKeyProviderConfigValidationFunction allows registering config validation for additional key providers.
//
// The default key providers KeyProviderPassphrase and KeyProviderDirect are automatically registered.
//
// Note: You must also register the key provider with the encryption flow, or it will still not be available.
func RegisterKeyProviderConfigValidationFunction(name KeyProviderName, validator KeyProviderConfigValidationFunction) error {
	if validator == nil {
		return fmt.Errorf("missing validator during registration for key provider %s: nil", name)
	}

	conflict := setKeyProviderConfigValidation(name, validator)
	if conflict {
		return fmt.Errorf("duplicate registration for key provider %s", name)
	}

	return nil
}

type EncryptionMethodConfigValidationFunction func(config EncryptionMethodConfig) error

// RegisterEncryptionMethodConfigValidationFunction allows registering config validation for additional encryption methods.
//
// The default encryption method EncryptionMethodFull is automatically registered.
//
// Note: You must also register the method with the encryption flow, or it will still not be available.
func RegisterEncryptionMethodConfigValidationFunction(name EncryptionMethodName, validator EncryptionMethodConfigValidationFunction) error {
	if validator == nil {
		return fmt.Errorf("missing validator during registration for encryption method %s: nil", name)
	}

	conflict := setEncryptionMethodConfigValidation(name, validator)
	if conflict {
		return fmt.Errorf("duplicate registration for encryption method %s", name)
	}

	return nil
}

// low level implementation and locking for validators

var keyProviderConfigValidation_useGetAndSet = make(map[KeyProviderName]KeyProviderConfigValidationFunction)
var keyProviderConfigValidationMutex = sync.RWMutex{}

func getKeyProviderConfigValidation(name KeyProviderName) (validator KeyProviderConfigValidationFunction, ok bool) {
	keyProviderConfigValidationMutex.RLock()
	defer keyProviderConfigValidationMutex.RUnlock()

	validator, ok = keyProviderConfigValidation_useGetAndSet[name]
	return
}

func setKeyProviderConfigValidation(name KeyProviderName, validator KeyProviderConfigValidationFunction) (conflict bool) {
	keyProviderConfigValidationMutex.Lock()
	defer keyProviderConfigValidationMutex.Unlock()

	_, conflict = keyProviderConfigValidation_useGetAndSet[name]
	if !conflict {
		keyProviderConfigValidation_useGetAndSet[name] = validator
	}
	return
}

var encryptionMethodConfigValidation_useGetAndSet = make(map[EncryptionMethodName]EncryptionMethodConfigValidationFunction)
var encryptionMethodConfigValidationMutex = sync.RWMutex{}

func getEncryptionMethodConfigValidation(name EncryptionMethodName) (validator EncryptionMethodConfigValidationFunction, ok bool) {
	encryptionMethodConfigValidationMutex.RLock()
	defer encryptionMethodConfigValidationMutex.RUnlock()

	validator, ok = encryptionMethodConfigValidation_useGetAndSet[name]
	return
}

func setEncryptionMethodConfigValidation(name EncryptionMethodName, validator EncryptionMethodConfigValidationFunction) (conflict bool) {
	encryptionMethodConfigValidationMutex.Lock()
	defer encryptionMethodConfigValidationMutex.Unlock()

	_, conflict = encryptionMethodConfigValidation_useGetAndSet[name]
	if !conflict {
		encryptionMethodConfigValidation_useGetAndSet[name] = validator
	}
	return
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
