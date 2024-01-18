package encryptionflow

import (
	"errors"
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

type EncryptedDocument map[string]any

// Method is the interface that must be implemented for a state encryption method.
type Method interface {
	// Decrypt the state or plan.
	//
	// encrypted and info are what your own Encrypt method returned.
	// The encryption Flow takes care of extracting these from the json document for you.
	// Note that encrypted will not contain the key EncryptionTopLevelJsonKey, that part is in info.
	//
	// configuration is a deep copy of the configuration. It is safe to make changes to it.
	//
	// keyProvider is the KeyProvider instance. You should call it to receive the key, passing on info and configuration.
	//
	// If you do not return an error, you must ensure you return the same []byte that
	// your own Encrypt method received.
	Decrypt(encrypted EncryptedDocument, info *EncryptionInfo, configuration encryptionconfig.Config, keyProvider KeyProvider) ([]byte, error)

	// Encrypt a state or plan.
	//
	// payload is the unencrypted state (a json document) or plan (zip format).
	//
	// info is pre-filled with the encryption version and encryption method name. Usually all a method does is
	// pass it on to the keyProvider, but you are allowed to add info to it. The encryption Flow takes care
	// of adding info to the final json document under the key EncryptionTopLevelJsonKey.
	//
	// configuration is a deep copy of the configuration. It is safe to make changes to it.
	//
	// keyProvider is the KeyProvider instance. You should call it to receive the key.
	//
	// If you do not return an error, you must ensure that you return a non-nil EncryptedDocument
	// which is not allowed to contain the key EncryptionTopLevelJsonKey.
	Encrypt(payload []byte, info *EncryptionInfo, configuration encryptionconfig.Config, keyProvider KeyProvider) (EncryptedDocument, error)
}

// MethodMetadata provides the configuration parser and encryption Flow with information
// about an encryption Method.
//
// See also RegisterMethod().
type MethodMetadata struct {
	// Name is the name of the method, as it is referred to in the configuration.
	Name encryptionconfig.MethodName

	// JsonOnly indicates your Method can only operate on json documents.
	//
	// If set to true, this method is not valid for plan encryption (plans are zip, not json).
	JsonOnly bool

	// Constructor should return a ready-to-go instance of your Method.
	Constructor func() (Method, error)

	// ConfigValidator should validate that the configuration is suitable for this encryption method.
	//
	// You do not have to check that the Name matches, that is done for you.
	//
	// Do not reject a configuration because it is missing a value that is added by the key provider,
	// such as the encryption key.
	ConfigValidator encryptionconfig.MethodValidator
}

var registeredMethods = make(map[encryptionconfig.MethodName]MethodMetadata)
var registeredMethodsMutex = sync.RWMutex{}

// RegisterMethod allows external packages to register additional encryption methods.
func RegisterMethod(metadata MethodMetadata) error {
	if metadata.Name == "" {
		return errors.New("invalid metadata: cannot register a method with empty name")
	}
	if metadata.Constructor == nil || metadata.ConfigValidator == nil {
		return errors.New("invalid metadata: Constructor and ConfigValidator are mandatory when registering a method")
	}

	err := encryptionconfig.RegisterMethodValidator(metadata.Name, metadata.ConfigValidator)
	if err != nil {
		return err
	}

	registeredMethodsMutex.Lock()
	defer registeredMethodsMutex.Unlock()

	registeredMethods[metadata.Name] = metadata
	return nil
}

func constructMethod(name encryptionconfig.MethodName) (Method, error) {
	registeredMethodsMutex.RLock()
	defer registeredMethodsMutex.RUnlock()

	metadata, ok := registeredMethods[name]
	if !ok {
		return nil, fmt.Errorf("no registered encryption method '%s'", name)
	}

	return metadata.Constructor()
}

func methodIsJsonOnly(name encryptionconfig.MethodName) bool {
	registeredMethodsMutex.RLock()
	defer registeredMethodsMutex.RUnlock()

	metadata, ok := registeredMethods[name]
	if !ok {
		return false
	}
	return metadata.JsonOnly
}
