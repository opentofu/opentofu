// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/registry"
)

func ComplianceTest(t *testing.T, factory func() registry.Registry) {
	t.Run("returns-registry", func(t *testing.T) {
		reg := factory()
		if reg == nil {
			t.Fatalf("Calling the factory method did not return a valid registry.")
		}
	})

	t.Run("key_provider", func(t *testing.T) {
		complianceTestKeyProviders(t, factory)
	})

	t.Run("method", func(t *testing.T) {
		complianceTestMethods(t, factory)
	})
}
