package encryptionflow

import (
	"errors"
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

// KeyProvider is the interface that must be implemented for a key provider.
type KeyProvider interface {
	// ProvideKey should obtain the key and add it to the configuration.
	//
	// If encrypting, info will only contain the encryption method name and nothing else.
	// You can add information to it, but you do not have to if your KeyProvider can
	// provide the key from the configuration and nothing else. If you need to persist
	// some information so that decryption can provide the same key again, assign
	// a value to info.KeyProvider.
	//
	// Warning: Do NOT persist the actual key or anything that allows reconstructing it.
	// The information under info is written to the encrypted state or plan in plain.
	//
	// If decrypting, info.KeyProvider will be set to whatever you set it to during encryption.
	//
	// You can also alter the configuration, although most key providers will not need this.
	//
	// If you do not return an error, you must return a valid key.
	ProvideKey(info *EncryptionInfo, configuration *encryptionconfig.Config) ([]byte, error)
}

// KeyProviderMetadata provides the configuration parser and encryption Flow with information
// about a KeyProvider.
//
// See also RegisterKeyProvider().
type KeyProviderMetadata struct {
	// Name is the name of the key provider, as it is referred to in the configuration.
	Name encryptionconfig.KeyProviderName

	// Constructor should return a ready-to-go instance of your KeyProvider.
	Constructor func() (KeyProvider, error)

	// Validator should validate that the configuration is suitable for this key provider.
	//
	// You do not have to check that the Name matches, that is done for you.
	//
	// Do not reject a configuration because it is missing a value that is added dynamically
	// during encryption / decryption.
	ConfigValidator encryptionconfig.KeyProviderValidator
}

var registeredKeyProviders = make(map[encryptionconfig.KeyProviderName]KeyProviderMetadata)
var registeredKeyProvidersMutex = sync.RWMutex{}

// RegisterKeyProvider allows external packages to register additional key providers.
func RegisterKeyProvider(metadata KeyProviderMetadata) error {
	if metadata.Name == "" {
		return errors.New("invalid metadata: cannot register a key provider with empty name")
	}
	if metadata.Constructor == nil || metadata.ConfigValidator == nil {
		return errors.New("invalid metadata: Constructor and ConfigValidator are mandatory when registering a key provider")
	}

	err := encryptionconfig.RegisterKeyProviderValidator(metadata.Name, metadata.ConfigValidator)
	if err != nil {
		return err
	}

	registeredKeyProvidersMutex.Lock()
	defer registeredKeyProvidersMutex.Unlock()

	registeredKeyProviders[metadata.Name] = metadata
	return nil
}

func constructKeyProvider(name encryptionconfig.KeyProviderName) (KeyProvider, error) {
	registeredKeyProvidersMutex.RLock()
	defer registeredKeyProvidersMutex.RUnlock()

	metadata, ok := registeredKeyProviders[name]
	if !ok {
		return nil, fmt.Errorf("no registered key provider '%s'", name)
	}

	return metadata.Constructor()
}
