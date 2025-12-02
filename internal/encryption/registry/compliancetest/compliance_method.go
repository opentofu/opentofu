// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"
)

func complianceTestMethods(t *testing.T, factory func() registry.Registry) {
	t.Run("registration-and-return", func(t *testing.T) {
		complianceTestMethodRegistrationAndReturn(t, factory)
	})
	t.Run("register-invalid-id", func(t *testing.T) {
		complianceTestMethodInvalidID(t, factory)
	})
	t.Run("duplicate-registration", func(t *testing.T) {
		complianceTestMethodDuplicateRegistration(t, factory)
	})
}

func complianceTestMethodRegistrationAndReturn(t *testing.T, factory func() registry.Registry) {
	reg := factory()
	testMethod := &testMethodDescriptor{
		"test",
	}
	if err := reg.RegisterMethod(testMethod); err != nil {
		t.Fatalf("Failed to register test method with ID %s (%v)", testMethod.id, err)
	}
	returnedMethod, err := reg.GetMethodDescriptor(testMethod.id)
	if err != nil {
		t.Fatalf("The previously registered method with the ID %s couldn't be fetched from the registry (%v).", testMethod.id, err)
	}
	returnedTypedMethod, ok := returnedMethod.(*testMethodDescriptor)
	if !ok {
		t.Fatalf("The returned method was not of the expected type of %T, but instead it was %T.", testMethod, returnedMethod)
	}
	if returnedTypedMethod.id != testMethod.id {
		t.Fatalf("The returned method contained the wrong ID %s instead of %s", returnedTypedMethod.id, testMethod.id)
	}

	_, err = reg.GetMethodDescriptor("nonexistent")
	if err == nil {
		t.Fatalf("Requesting a non-existent method from GetMethodDescriptor did not return an error.")
	}
	var typedErr *registry.MethodNotFoundError
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Requesting a non-existent method from GetMethodDescriptor returned an incorrect error type of %T. This function should always return a *registry.MethodNotFoundError if the method was not found.",
			err,
		)
	}
}

func complianceTestMethodInvalidID(t *testing.T, factory func() registry.Registry) {
	reg := factory()
	testMethod := &testMethodDescriptor{
		"Hello world!",
	}
	err := reg.RegisterMethod(testMethod)
	if err == nil {
		t.Fatalf("Registering a method with the invalid ID of %s did not result in an error.", testMethod.id)
	}
	var typedErr *registry.InvalidMethodError
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Registering a method with an invalid ID of %s resulted in an error of type %T instead of %T. Please make sure to use the correct typed errors.",
			testMethod.id,
			err,
			typedErr,
		)
	}
}

func complianceTestMethodDuplicateRegistration(t *testing.T, factory func() registry.Registry) {
	reg := factory()
	testMethod := &testMethodDescriptor{
		"test",
	}
	testMethod2 := &testMethodDescriptor{
		"test",
	}
	if err := reg.RegisterMethod(testMethod); err != nil {
		t.Fatalf("Failed to register test method with ID %s (%v)", testMethod.id, err)
	}
	err := reg.RegisterMethod(testMethod)
	if err == nil {
		t.Fatalf("Re-registering the same method again did not result in an error.")
	}
	var typedErr *registry.MethodAlreadyRegisteredError
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Re-registering the same method twice resulted in an error of the type %T instead of %T. Please make sure to use the correct typed errors.",
			err,
			typedErr,
		)
	}

	err = reg.RegisterMethod(testMethod2)
	if err == nil {
		t.Fatalf("Re-registering the a provider with a duplicate ID did not result in an error.")
	}
	if !errors.As(err, &typedErr) {
		t.Fatalf(
			"Re-registering the a method with a duplicate ID resulted in an error of the type %T instead of %T. Please make sure to use the correct typed errors.",
			err,
			typedErr,
		)
	}
}

type testMethodDescriptor struct {
	id method.ID
}

func (t testMethodDescriptor) ID() method.ID {
	return t.id
}

func (t testMethodDescriptor) DecodeConfig(method.EvalContext, hcl.Body) (method.Config, hcl.Diagnostics) {
	return &testMethodConfig{}, nil
}

type testMethodConfig struct {
}

func (t testMethodConfig) Build() (method.Method, error) {
	return nil, method.ErrInvalidConfiguration{
		Cause: fmt.Errorf("build not implemented for test method"),
	}
}
