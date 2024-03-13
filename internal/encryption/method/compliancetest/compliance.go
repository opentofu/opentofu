// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/compliancetest"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

type TestDescriptor struct {
	Descriptor method.Descriptor
}

// TestCase describes a single compliance test case.
type TestCase struct {
}

// ComplianceTest tests the functionality of a method to make sure it conforms to the expectations of the method
// interface.
func ComplianceTest(t *testing.T, testDescriptor TestDescriptor) {
	t.Run("id", func(t *testing.T) {
		complianceTestID(t, testDescriptor)
	})
	t.Run("config-struct", func(t *testing.T) {
		compliancetest.ConfigStruct(t, testDescriptor.Descriptor.ConfigStruct())
	})
	t.Run("behavior", func(t *testing.T) {

	})
}

func complianceTestID(t *testing.T, descriptor TestDescriptor) {
	id := descriptor.Descriptor.ID()
	if err := id.Validate(); err != nil {
		compliancetest.Fail(t, "Invalid ID returned from method descriptor: %s (%v)", id, err)
	} else {
		compliancetest.Log(t, "The ID provided by the method descriptor is valid: %s", id)
	}
}
