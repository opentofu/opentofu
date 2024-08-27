// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lockingencryptionregistry_test

import (
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/registry/compliancetest"
	"github.com/terramate-io/opentofulib/internal/encryption/registry/lockingencryptionregistry"
)

func TestCompliance(t *testing.T) {
	compliancetest.ComplianceTest(t, lockingencryptionregistry.New)
}
