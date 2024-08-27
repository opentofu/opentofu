// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2_test

import (
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider/pbkdf2"
)

func TestDescriptor_ID(t *testing.T) {
	if id := pbkdf2.New().ID(); id != "pbkdf2" {
		t.Fatalf("incorrect ID: %s", id)
	}
}
