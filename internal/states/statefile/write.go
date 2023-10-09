// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statefile

import (
	"io"

	tfversion "github.com/opentofu/opentofu/version"
)

// Write writes the given state to the given writer in the current state
// serialization format.
func Write(s *File, w io.Writer) error {
	// Always record the current tofu version in the state.
	s.TerraformVersion = tfversion.SemVer

	diags := writeStateV4(s, w)
	return diags.Err()
}

// WriteForTest writes the given state to the given writer in the current state
// serialization format without recording the current tofu version. This is
// intended for use in tests that need to override the current tofu
// version.
func WriteForTest(s *File, w io.Writer) error {
	diags := writeStateV4(s, w)
	return diags.Err()
}
