// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"testing"
)

func ComplianceTest(t *testing.T, factory func() Registry) {
	t.Run("returns-registry", func(t *testing.T) {
		registry := factory()
		if registry == nil {
			t.Fatalf("Calling the factory method did not return a valid registry.")
		}
	})

	t.Run("key_provider", func(t *testing.T) {
		complianceTestKeyProviders(t, factory)
	})
}
