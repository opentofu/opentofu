// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
)

// This file contains some re-usable predictors for auto-complete. The
// command-specific autocomplete configurations live within each command's
// own source file, as AutocompleteArgs and AutocompleteFlags methods on each
// Command implementation.

func (m *Meta) completePredictWorkspaceName(ctx context.Context) []string {
	// There are lot of things that can fail in here, so if we encounter
	// any error then we'll just return nothing and not support autocomplete
	// until whatever error is fixed. (The user can't actually see the error
	// here, but other commands should produce a user-visible error before
	// too long.)

	// We assume here that we want to autocomplete for the current working
	// directory, since we don't have enough context to know where to
	// find any config path argument, and it might be _after_ the argument
	// we're trying to complete here anyway.
	configPath, err := modulePath(nil)
	if err != nil {
		return nil
	}

	backendConfig, diags := m.loadBackendConfig(ctx, configPath)
	if diags.HasErrors() {
		return nil
	}

	// Load the encryption configuration
	enc, encDiags := m.Encryption(ctx)
	if encDiags.HasErrors() {
		return nil
	}

	b, diags := m.Backend(ctx, &BackendOpts{
		Config: backendConfig,
	}, enc.State())
	if diags.HasErrors() {
		return nil
	}

	names, _ := b.Workspaces(ctx)
	return names
}
