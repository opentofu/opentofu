// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Backend struct {
	// IgnoreRemoteVersion is used with commands which write state to allow users to write remote
	// state even if the remote and local OpenTofu versions don't match.
	IgnoreRemoteVersion bool
	// ForceInitCopy suppresses confirmation for copying state data during init.
	ForceInitCopy bool
	// Reconfigure forces init to ignore any stored configuration.
	Reconfigure bool
	// MigrateState confirms the user wishes to migrate from the prior backend configuration to a new configuration.
	MigrateState bool
}

func BindBackend(cli *CommandLine) *Backend {
	var b Backend
	cli.Backend = &b
	cli.BoolVar(&b.IgnoreRemoteVersion, "ignore-remote-version", false, "A rare option used for the remote backend only. See the remote backend documentation for more information.")
	return &b
}

func BindBackendWithMigration(cli *CommandLine) *Backend {
	b := BindBackend(cli)

	cli.BoolVar(&b.ForceInitCopy, "force-copy", false, `Suppress prompts about copying state data when initializing a new state backend. This is equivalent to providing a "yes" to all confirmation prompts.`)
	cli.BoolVar(&b.Reconfigure, "reconfigure", false, `Reconfigure a backend, ignoring any saved configuration.`)
	cli.BoolVar(&b.MigrateState, "migrate-state", false, `Reconfigure a backend, and attempt to migrate any existing state.`)

	cli.PreHook(func() tfdiags.Diagnostics {
		if b.MigrateState && b.Reconfigure {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Wrong combination of options",
				"The -migrate-state and -reconfigure options are mutually-exclusive",
			))
		}

		// Copying the state only happens during backend migration, so setting
		// -force-copy implies -migrate-state
		if b.ForceInitCopy {
			b.MigrateState = true
		}
		return nil
	})

	return b
}
