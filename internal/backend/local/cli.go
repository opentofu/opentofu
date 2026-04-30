// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"log"

	"github.com/opentofu/opentofu/internal/backend"
)

// backend.CLI impl.
func (b *Local) CLIInit(opts *backend.CLIOpts) error {
	b.ContextOpts = opts.ContextOpts
	b.OpInput = opts.Input
	b.OpValidation = opts.Validation

	// configure any new cli options
	if opts.StateArgs.StatePath != "" {
		log.Printf("[TRACE] backend/local: CLI option -state is overriding state path to %s", opts.StateArgs.StatePath)
		b.OverrideStatePath = opts.StateArgs.StatePath
	}

	if opts.StateArgs.StateOutPath != "" {
		log.Printf("[TRACE] backend/local: CLI option -state-out is overriding state output path to %s", opts.StateArgs.StateOutPath)
		b.OverrideStateOutPath = opts.StateArgs.StateOutPath
	}

	if opts.StateArgs.BackupPath != "" {
		log.Printf("[TRACE] backend/local: CLI option -backup is overriding state backup path to %s", opts.StateArgs.BackupPath)
		b.OverrideStateBackupPath = opts.StateArgs.BackupPath
	}

	return nil
}
