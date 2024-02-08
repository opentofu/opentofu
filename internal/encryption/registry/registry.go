package registry

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// Registry collects all encryption methods and key providers
type Registry interface {
	// RegisterKeyProvider registers a key provider. Use the keyprovider.Any().
	// This function returns a *KeyProviderAlreadyRegisteredError error if a key provider with the
	// same ID is already registered.
	RegisterKeyProvider(keyProvider keyprovider.Descriptor) error
	// RegisterMethod registers an encryption method. Use the method.Any() function to convert your method into a
	// suitable format. This function returns a *MethodAlreadyRegisteredError error if a key provider with the same ID is
	// already registered.
	RegisterMethod(method method.Descriptor) error

	// GetKeyProvider returns the key provider with the specified ID. If the key provider is not registered,
	// it will return a *KeyProviderNotFound error.
	GetKeyProvider(id keyprovider.ID) (keyprovider.Descriptor, error)

	// GetMethod returns the method with the specified ID.
	// If the method is not registered, it will return a *MethodNotFoundError.
	GetMethod(id method.ID) (method.Descriptor, error)
}
