// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statefile

import (
	"io"

	"github.com/terramate-io/opentofulib/internal/encryption"
	tfversion "github.com/terramate-io/opentofulib/version"
)

// Write writes the given state to the given writer in the current state
// serialization format.
func Write(s *File, w io.Writer, enc encryption.StateEncryption) error {
	// Always record the current tofu version in the state.
	s.TerraformVersion = tfversion.SemVer

	diags := writeStateV4(s, w, enc)
	return diags.Err()
}

// WriteForTest writes the given state to the given writer in the current state
// serialization format without recording the current tofu version. This is
// intended for use in tests that need to override the current tofu
// version.
func WriteForTest(s *File, w io.Writer) error {
	diags := writeStateV4(s, w, encryption.StateEncryptionDisabled())
	return diags.Err()
}
