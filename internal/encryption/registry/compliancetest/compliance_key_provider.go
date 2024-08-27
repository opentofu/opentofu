// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"errors"
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
	"github.com/terramate-io/opentofulib/internal/encryption/registry"
)

func complianceTestKeyProviders(t *testing.T, factory func() registry.Registry) {
	t.Run("registration-and-return", func(t *testing.T) {
		complianceTestKeyProviderRegistrationAndReturn(t, factory)
	})
	t.Run("register-invalid-id", func(t *testing.T) {
		complianceTestKeyProviderInvalidID(t, factory)
	})
	t.Run("duplicate-registration", func(t *testing.T) {
		complianceTestKeyProviderDuplicateRegistration(t, factory)
	})
}

func complianceTestKeyProviderRegistrationAndReturn(t *testing.T, factory func() registry.Registry) {
	reg := factory()
	testKeyProvider := &testKeyProviderDescriptor{
		"test",
	}
	if err := reg.RegisterKeyProvider(testKeyProvider); err != nil {
		t.Fatalf("Failed to register test key provider with ID %s (%v)", testKeyProvider.id, err)
	}
	returnedKeyProvider, err := reg.GetKeyProviderDescriptor(testKeyProvider.id)
	if err != nil {
		t.Fatalf("The previously registered key provider with the ID %s couldn't be fetched from the registry (%v).", testKeyProvider.id, err)
	}
	returnedTypedKeyProvider, ok := returnedKeyProvider.(*testKeyProviderDescriptor)
	if !ok {
		t.Fatalf("The returned key provider was not of the expected type of %T, but instead it was %T.", testKeyProvider, returnedKeyProvider)
	}
	if returnedTypedKeyProvider.id != testKeyProvider.id {
		t.Fatalf("The returned key provider contained the wrong ID %s instead of %s", returnedTypedKeyProvider.id, testKeyProvider.id)
	}

	_, err = reg.GetKeyProviderDescriptor("nonexistent")
	if err == nil {
		t.Fatalf("Requesting a non-existent key provider from GetKeyProviderDescriptor did not return an error.")
	}
	var typedErr *registry.KeyProviderNotFoundError
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Requesting a non-existent key provider from GetKeyProviderDescriptor returned an incorrect error type of %T. This function should always return a *registry.KeyProviderNotFoundError if the key provider was not found.",
			err,
		)
	}
}

func complianceTestKeyProviderInvalidID(t *testing.T, factory func() registry.Registry) {
	reg := factory()
	testKeyProvider := &testKeyProviderDescriptor{
		"Hello world!",
	}
	err := reg.RegisterKeyProvider(testKeyProvider)
	if err == nil {
		t.Fatalf("Registering a key provider with the invalid ID of %s did not result in an error.", testKeyProvider.id)
	}
	var typedErr *registry.InvalidKeyProviderError
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Registering a key provider with an invalid ID of %s resulted in an error of type %T instead of %T. Please make sure to use the correct typed errors.",
			testKeyProvider.id,
			err,
			typedErr,
		)
	}
}

func complianceTestKeyProviderDuplicateRegistration(t *testing.T, factory func() registry.Registry) {
	reg := factory()
	testKeyProvider := &testKeyProviderDescriptor{
		"test",
	}
	testKeyProvider2 := &testKeyProviderDescriptor{
		"test",
	}
	if err := reg.RegisterKeyProvider(testKeyProvider); err != nil {
		t.Fatalf("Failed to register test key provider with ID %s (%v)", testKeyProvider.id, err)
	}
	err := reg.RegisterKeyProvider(testKeyProvider)
	if err == nil {
		t.Fatalf("Re-registering the same key provider again did not result in an error.")
	}
	var typedErr *registry.KeyProviderAlreadyRegisteredError
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Re-registering the same key provider twice resulted in an error of the type %T instead of %T. Please make sure to use the correct typed errors.",
			err,
			typedErr,
		)
	}

	err = reg.RegisterKeyProvider(testKeyProvider2)
	if err == nil {
		t.Fatalf("Re-registering the a provider with a duplicate ID did not result in an error.")
	}
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Re-registering the a key provider with a duplicate ID resulted in an error of the type %T instead of %T. Please make sure to use the correct typed errors.",
			err,
			typedErr,
		)
	}
}

type testKeyProviderDescriptor struct {
	id keyprovider.ID
}

func (t testKeyProviderDescriptor) ID() keyprovider.ID {
	return t.id
}

func (t testKeyProviderDescriptor) ConfigStruct() keyprovider.Config {
	return &testKeyProviderConfigStruct{}
}

type testKeyProviderConfigStruct struct {
}

func (t testKeyProviderConfigStruct) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	return nil, nil, keyprovider.ErrInvalidConfiguration{
		Message: "The Build() function is not implemented on the testKeyProviderConfigStruct",
	}
}
