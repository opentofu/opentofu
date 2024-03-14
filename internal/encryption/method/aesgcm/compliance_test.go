// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/method/compliancetest"
)

func TestCompliance(t *testing.T) {
	compliancetest.ComplianceTest[*Config](t, compliancetest.TestDescriptor{
		Descriptor: New(),
	})
}
