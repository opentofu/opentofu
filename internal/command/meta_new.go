// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans/planfile"
)

// NOTE: Temporary file until this branch is cleaned up.

// PlanFile loads the plan file at the given path, which might be either a local
// or cloud plan.
//
// If the return value and error are both nil, the given path exists but seems
// to be a configuration directory instead.
//
// Error will be non-nil if path refers to something which looks like a plan
// file and loading the file fails.
func (m *Meta) PlanFile(path string, enc encryption.PlanEncryption) (*planfile.WrappedPlanFile, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		// Looks like a configuration directory.
		return nil, nil
	}

	return planfile.OpenWrapped(path, enc)
}
