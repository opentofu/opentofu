// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

// This file contains all the Backend-related function calls on Meta,
// exported and private.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	backend2 "github.com/opentofu/opentofu/internal/command/backend"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/command/workspace"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	legacy "github.com/opentofu/opentofu/internal/legacy/tofu"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Operation initializes a new backend.Operation struct.
//
// This prepares the operation. After calling this, the caller is expected
// to modify fields of the operation such as Sequence to specify what will
// be called.
func (m *Meta) Operation(ctx context.Context, b backend.Backend, vt arguments.ViewOptions, enc encryption.Encryption) *backend.Operation {
	schema := b.ConfigSchema()
	workspace, err := m.Workspace.Workspace(ctx)
	if err != nil {
		// An invalid workspace error would have been raised when creating the
		// backend, and the caller should have already exited. Seeing the error
		// here first is a bug, so panic.
		panic(fmt.Sprintf("invalid workspace: %s", err))
	}
	planOutBackend, err := m.backendState.ForPlan(schema, workspace)
	if err != nil {
		// Always indicates an implementation error in practice, because
		// errors here indicate invalid encoding of the backend configuration
		// in memory, and we should always have validated that by the time
		// we get here.
		panic(fmt.Sprintf("failed to encode backend configuration for plan: %s", err))
	}

	stateLocker := clistate.NewNoopLocker()
	if m.stateLock {
		view := views.NewStateLocker(vt, m.View)
		stateLocker = clistate.NewLocker(m.stateLockTimeout, view)
	}

	depLocks, diags := m.lockedDependencies()
	if diags.HasErrors() {
		// We can't actually report errors from here, but m.lockedDependencies
		// should always have been called earlier to prepare the "ContextOpts"
		// for the backend anyway, so we should never actually get here in
		// a real situation. If we do get here then the backend will inevitably
		// fail downstream somewhere if it tries to use the empty depLocks.
		log.Printf("[WARN] Failed to load dependency locks while preparing backend operation (ignored): %s", diags.Err().Error())
	}

	return &backend.Operation{
		Encryption:      enc,
		PlanOutBackend:  planOutBackend,
		Targets:         m.targets,
		Excludes:        m.excludes,
		UIIn:            m.UIInput(),
		UIOut:           m.Ui,
		Workspace:       workspace,
		StateLocker:     stateLocker,
		DependencyLocks: depLocks,
	}
}

// backendCLIOpts returns a backend.CLIOpts object that should be passed to
// a backend that supports local CLI operations.
func (m *Meta) backendCLIOpts(ctx context.Context) (*backend.CLIOpts, error) {
	contextOpts, err := m.contextOpts(ctx)
	if contextOpts == nil && err != nil {
		return nil, err
	}
	return &backend.CLIOpts{
		CLI:                 m.Ui,
		CLIColor:            m.Colorize(),
		Streams:             m.Streams,
		StatePath:           m.statePath,
		StateOutPath:        m.stateOutPath,
		StateBackupPath:     m.backupPath,
		ContextOpts:         contextOpts,
		Input:               m.Input.Input(test),
		RunningInAutomation: m.RunningInAutomation,
	}, err
}

func buildCliOpts(m *Meta) backend2.BackendCLIOptsBuilder {
	return func(ctx context.Context, opts *backend2.BackendOpts) (cliOpts *backend.CLIOpts, diags tfdiags.Diagnostics) {
		cliOpts, err := m.backendCLIOpts(ctx)
		if err != nil {
			if errs := providerPluginErrors(nil); errors.As(err, &errs) {
				// This is a special type returned by m.providerFactories, which
				// indicates one or more inconsistencies between the dependency
				// lock file and the provider plugins actually available in the
				// local cache directory.
				//
				// If initialization is allowed, we ignore this error, as it may
				// be resolved by the later step where providers are fetched.
				if !opts.Init {
					var buf bytes.Buffer
					for addr, err := range errs {
						fmt.Fprintf(&buf, "\n  - %s: %s", addr, err)
					}
					suggestion := "To download the plugins required for this configuration, run:\n  tofu init"
					if m.RunningInAutomation {
						// Don't mention "tofu init" specifically if we're running in an automation wrapper
						suggestion = "You must install the required plugins before running OpenTofu operations."
					}
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Required plugins are not installed",
						fmt.Sprintf(
							"The installed provider plugins are not consistent with the packages selected in the dependency lock file:%s\n\nOpenTofu uses external plugins to integrate with a variety of different infrastructure services. %s",
							buf.String(), suggestion,
						),
					))
					return nil, diags
				}
			} else {
				// All other errors just get generic handling.
				diags = diags.Append(err)
				return nil, diags
			}
		}
		return cliOpts, diags
	}
}

func buildBackendFlags(m *Meta) *backend2.BackendFlags {
	return &backend2.BackendFlags{
		AllowExperimentalFeatures: m.AllowExperimentalFeatures,
		ConfigLoader: func(ctx context.Context) (*configs.Backend, tfdiags.Diagnostics) {
			return m.loadBackendConfig(ctx, ".")
		},
		Reconfigure:   m.reconfigure,
		MigrateState:  m.migrateState,
		ForceInitCopy: m.forceInitCopy,
		SetBackendStateCb: func(b *legacy.BackendState) {
			m.backendState = b
		},
		Workspace:               workspace.ConfiguredWorkspace(m.Workspace, m.Input, m.UIInput()),
		InputForcefullyDisabled: test,
		Input:                   m.Input,
		Ui:                      m.Ui, // TODO andrei this needs to be done differently
		View:                    m.View,
		Colorize:                m.Colorize,
		ShowDiagnostics:         m.showDiagnostics,
		UIInput:                 m.UIInput,
		Services:                m.Services,
		IgnoreRemoteVersion:     m.ignoreRemoteVersion,
		// TODO andrei this is ugly and should be handled separately
		LegacyStateCb: func() {
			// If we got here from backendFromConfig returning nil then m.backendState
			// won't be set, since that codepath considers that to be no backend at all,
			// but our caller considers that to be the local backend with no config
			// and so we'll synthesize a backend state so other code doesn't need to
			// care about this special case.
			//
			// FIXME: We should refactor this so that we more directly and explicitly
			// treat the local backend as the default, including in the UI shown to
			// the user, since the local backend should only be used when learning or
			// in exceptional cases and so it's better to help the user learn that
			// by introducing it as a concept.
			if m.backendState == nil {
				// NOTE: This synthetic object is intentionally _not_ retained in the
				// on-disk record of the backend configuration, which was already dealt
				// with inside backendFromConfig, because we still need that codepath
				// to be able to recognize the lack of a config as distinct from
				// explicitly setting local until we do some more refactoring here.
				m.backendState = &legacy.BackendState{
					Type:      "local",
					ConfigRaw: json.RawMessage("{}"),
				}
			}
		},
		WorkdirFetcher: func() *workdir.Dir {
			return m.WorkingDir
		},
		CLIOptsBuilder:   buildCliOpts(m),
		StateLock:        m.stateLock,
		StateLockTimeout: m.stateLockTimeout,
	}
}
