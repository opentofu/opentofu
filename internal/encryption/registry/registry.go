package registry

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// Registry collects all encryption methods and key providers
type Registry interface {
	// RegisterKeyProvider registers a key provider. Use the keyprovider.Any() function to convert your key provider
	// into a suitable format. This function returns a *KeyProviderAlreadyRegistered error if a key provider with the
	// same ID is already registered.
	RegisterKeyProvider(keyProvider keyprovider.Factory) error
	// RegisterMethod registers an encryption method. Use the method.Any() function to convert your method into a
	// suitable format. This function returns a *MethodAlreadyRegistered error if a key provider with the same ID is
	// already registered.
	RegisterMethod(method method.Factory) error

	// GetKeyProvider returns the key provider with the specified ID. If the key provider is not registered,
	// it will return a *KeyProviderNotFound error.
	GetKeyProvider(id keyprovider.ID) (keyprovider.Factory, error)
	// GetMethod returns the method with the specified ID. If the method is not registered, it will return a
	// *MethodNotFound error.
	GetMethod(id method.ID) (method.Factory, error)
}
