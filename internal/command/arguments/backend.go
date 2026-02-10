// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"
	"time"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Backend struct {
	IgnoreRemoteVersion bool
	// StateLock indicates if the state should be locked or not.
	StateLock bool
	// StateLockTimeout configures the duration that it waits for the state lock to be acquired.
	StateLockTimeout time.Duration
	// ForceInitCopy controls if the prompts for state migration should be skipped or not.
	ForceInitCopy bool
	// Reconfigure controls if the reconfiguration of the backend should happen with discarding the old configurations.
	Reconfigure bool
	// MigrateState controls if during the reconfiguration of the backend a migration should be attempted.
	MigrateState bool
}

func (b *Backend) AddIgnoreRemoteVersionFlag(f *flag.FlagSet) {
	f.BoolVar(&b.IgnoreRemoteVersion, "ignore-remote-version", false, "continue even if remote and local OpenTofu versions are incompatible")
}

func (b *Backend) AddStateFlags(f *flag.FlagSet) {
	f.BoolVar(&b.StateLock, "lock", true, "lock state")
	f.DurationVar(&b.StateLockTimeout, "lock-timeout", 0, "lock timeout")
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
