// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"

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

func (b *Backend) AddIgnoreRemoteVersionFlag(f *flag.FlagSet) {
	f.BoolVar(&b.IgnoreRemoteVersion, "ignore-remote-version", false, "continue even if remote and local OpenTofu versions are incompatible")
}

func (b *Backend) AddMigrationFlags(f *flag.FlagSet) {
	f.BoolVar(&b.ForceInitCopy, "force-copy", false, "suppress prompts about copying state data")
	f.BoolVar(&b.Reconfigure, "reconfigure", false, "reconfigure")
	f.BoolVar(&b.MigrateState, "migrate-state", false, "migrate state")
}

func (b *Backend) migrationFlagsCheck() (diags tfdiags.Diagnostics) {
	if b.MigrateState && b.Reconfigure {
		return diags.Append(tfdiags.Sourceless(
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
	return diags
}
