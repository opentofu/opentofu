package encryptionconfig

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

type KeyProviderValidator func(config KeyProviderConfig) error

// RegisterKeyProviderValidator allows registering config validation for additional key providers.
//
// The default key providers KeyProviderPassphrase and KeyProviderDirect are automatically registered.
//
// Note: You must also register the key provider with the encryption flow, or it will still not be available.
func RegisterKeyProviderValidator(name KeyProviderName, validator KeyProviderValidator) error {
	if validator == nil {
		return fmt.Errorf("missing validator during registration for key provider \"%s\": nil", name)
	}

	conflict := keyProviderConfigValidators.set(name, validator)
	if conflict {
		return fmt.Errorf("duplicate registration for key provider \"%s\"", name)
	}

	return nil
}

type MethodValidator func(config MethodConfig) error

// RegisterMethodValidator allows registering config validation for additional encryption methods.
//
// The default encryption method MethodFull is automatically registered.
//
// Note: You must also register the method with the encryption flow, or it will still not be available.
func RegisterMethodValidator(name MethodName, validator MethodValidator) error {
	if validator == nil {
		return fmt.Errorf("missing validator during registration for encryption method \"%s\": nil", name)
	}

	conflict := methodConfigValidators.set(name, validator)
	if conflict {
		return fmt.Errorf("duplicate registration for encryption method \"%s\"", name)
	}

	return nil
}

// low level implementation and locking for validators

func newLockingMap[K comparable, V any]() *lockingMap[K, V] {
	return &lockingMap[K, V]{
		make(map[K]V),
		sync.RWMutex{},
	}
}

type lockingMap[K comparable, V any] struct {
	data map[K]V
	lock sync.RWMutex
}

func (l *lockingMap[K, V]) get(name K) (value V, ok bool) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	value, ok = l.data[name]
	return
}

func (l *lockingMap[K, V]) set(name K, value V) (conflict bool) {
	l.lock.Lock()
	defer l.lock.Unlock()

	_, conflict = l.data[name]
	if !conflict {
		l.data[name] = value
	}
	return
}

var keyProviderConfigValidators = newLockingMap[KeyProviderName, KeyProviderValidator]()
var methodConfigValidators = newLockingMap[MethodName, MethodValidator]()

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

func validateEMFullConfig(m MethodConfig) error {
	if len(m.Config) > 0 {
		return errors.New("unexpected fields, this method needs no configuration")
	}
	// no config fields
	return nil
}

func init() {
	_ = RegisterKeyProviderValidator(KeyProviderPassphrase, validateKPPassphraseConfig)
	_ = RegisterKeyProviderValidator(KeyProviderDirect, validateKPDirectConfig)

	_ = RegisterMethodValidator(MethodFull, validateEMFullConfig)
}
