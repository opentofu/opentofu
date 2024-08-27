// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"fmt"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
	"github.com/terramate-io/opentofulib/internal/encryption/method"
)

// InvalidKeyProviderError indicates that the supplied keyprovider.Descriptor is invalid/misbehaving. Check the error
// message for details.
type InvalidKeyProviderError struct {
	KeyProvider keyprovider.Descriptor
	Cause       error
}

func (k InvalidKeyProviderError) Error() string {
	return fmt.Sprintf("the supplied key provider %T is invalid (%v)", k.KeyProvider, k.Cause)
}

func (k InvalidKeyProviderError) Unwrap() error {
	return k.Cause
}

// KeyProviderNotFoundError indicates that the requested key provider was not found in the registry.
type KeyProviderNotFoundError struct {
	ID keyprovider.ID
}

func (k KeyProviderNotFoundError) Error() string {
	return fmt.Sprintf("key provider with ID %s not found", k.ID)
}

// KeyProviderAlreadyRegisteredError indicates that the requested key provider was already registered in the registry.
type KeyProviderAlreadyRegisteredError struct {
	ID               keyprovider.ID
	CurrentProvider  keyprovider.Descriptor
	PreviousProvider keyprovider.Descriptor
}

func (k KeyProviderAlreadyRegisteredError) Error() string {
	return fmt.Sprintf(
		"error while registering key provider ID %s to %T, this ID is already registered by %T",
		k.ID, k.CurrentProvider, k.PreviousProvider,
	)
}

// InvalidMethodError indicates that the supplied method.Descriptor is invalid/misbehaving. Check the error message for
// details.
type InvalidMethodError struct {
	Method method.Descriptor
	Cause  error
}

func (k InvalidMethodError) Error() string {
	return fmt.Sprintf("the supplied encryption method %T is invalid (%v)", k.Method, k.Cause)
}

func (k InvalidMethodError) Unwrap() error {
	return k.Cause
}

// MethodNotFoundError indicates that the requested encryption method was not found in the registry.
type MethodNotFoundError struct {
	ID method.ID
}

func (m MethodNotFoundError) Error() string {
	return fmt.Sprintf("encryption method with ID %s not found", m.ID)
}

// MethodAlreadyRegisteredError indicates that the requested encryption method was already registered in the registry.
type MethodAlreadyRegisteredError struct {
	ID             method.ID
	CurrentMethod  method.Descriptor
	PreviousMethod method.Descriptor
}

func (m MethodAlreadyRegisteredError) Error() string {
	return fmt.Sprintf(
		"error while registering encryption method ID %s to %T, this ID is already registered by %T",
		m.ID, m.CurrentMethod, m.PreviousMethod,
	)
}
