package registry

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// InvalidKeyProvider indicates that the supplied keyprovider.Factory is invalid/misbehaving. Check the error
// message for details.
type InvalidKeyProvider struct {
	KeyProvider keyprovider.Factory
	Cause       error
}

func (k InvalidKeyProvider) Error() string {
	return fmt.Sprintf("the supplied key provider %T is invalid (%v)", k.KeyProvider, k.Cause)
}

func (k InvalidKeyProvider) Unwrap() error {
	return k.Cause
}

type KeyProviderNotFound struct {
	ID keyprovider.ID
}

func (k KeyProviderNotFound) Error() string {
	return fmt.Sprintf("key provider with ID %s not found", k.ID)
}

type KeyProviderAlreadyRegistered struct {
	ID               keyprovider.ID
	CurrentProvider  keyprovider.Factory
	PreviousProvider keyprovider.Factory
}

func (k KeyProviderAlreadyRegistered) Error() string {
	return fmt.Sprintf(
		"error while registering key provider ID %s to %T, this ID is already registered by %T",
		k.ID, k.CurrentProvider, k.PreviousProvider,
	)
}

// InvalidMethod indicates that the supplied method.Factory is invalid/misbehaving. Check the error message for
// details.
type InvalidMethod struct {
	Method method.Factory
	Cause  error
}

func (k InvalidMethod) Error() string {
	return fmt.Sprintf("the supplied encryption method %T is invalid (%v)", k.Method, k.Cause)
}

func (k InvalidMethod) Unwrap() error {
	return k.Cause
}

type MethodNotFound struct {
	ID method.ID
}

func (m MethodNotFound) Error() string {
	return fmt.Sprintf("encryption method with ID %s not found", m.ID)
}

type MethodAlreadyRegistered struct {
	ID             method.ID
	CurrentMethod  method.Factory
	PreviousMethod method.Factory
}

func (m MethodAlreadyRegistered) Error() string {
	return fmt.Sprintf(
		"error while registering encryption method ID %s to %T, this ID is already registered by %T",
		m.ID, m.CurrentMethod, m.PreviousMethod,
	)
}
