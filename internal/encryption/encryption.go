// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/registry"
)

// Encryption contains the methods for obtaining a StateEncryption or PlanEncryption correctly configured for a specific
// purpose. If no encryption configuration is present, it should return a pass through method that doesn't do anything.
type Encryption interface {
	// StateFile produces a StateEncryption overlay for encrypting and decrypting state files for local storage.
	StateFile() StateEncryption

	// PlanFile produces a PlanEncryption overlay for encrypting and decrypting plan files.
	PlanFile() PlanEncryption

	// Backend produces a StateEncryption overlay for storing state files on remote backends, such as an S3 bucket.
	Backend() StateEncryption

	// RemoteState produces a StateEncryption for reading remote states using the terraform_remote_state data
	// source.
	RemoteState(string) StateEncryption
}

type encryption struct {
	statefile     StateEncryption
	planfile      PlanEncryption
	backend       StateEncryption
	remoteDefault StateEncryption
	remotes       map[string]StateEncryption

	// Inputs
	cfg *config.EncryptionConfig
	reg registry.Registry
}

// New creates a new Encryption provider from the given configuration and registry.
func New(reg registry.Registry, cfg *config.EncryptionConfig) (Encryption, hcl.Diagnostics) {
	if cfg == nil {
		return Disabled(), nil
	}

	enc := &encryption{
		cfg: cfg,
		reg: reg,

		remotes: make(map[string]StateEncryption),
	}
	var diags hcl.Diagnostics
	var encDiags hcl.Diagnostics

	if cfg.StateFile != nil {
		enc.statefile, encDiags = newStateEncryption(enc, cfg.StateFile.AsTargetConfig(), cfg.StateFile.Enforced, "statefile")
		diags = append(diags, encDiags...)
	} else {
		enc.statefile = StateEncryptionDisabled()
	}

	if cfg.PlanFile != nil {
		enc.planfile, encDiags = newPlanEncryption(enc, cfg.PlanFile.AsTargetConfig(), cfg.PlanFile.Enforced, "planfile")
		diags = append(diags, encDiags...)
	} else {
		enc.planfile = PlanEncryptionDisabled()
	}

	if cfg.Backend != nil {
		enc.backend, encDiags = newStateEncryption(enc, cfg.Backend.AsTargetConfig(), cfg.Backend.Enforced, "backend")
		diags = append(diags, encDiags...)
	} else {
		enc.backend = StateEncryptionDisabled()
	}

	if cfg.Remote != nil && cfg.Remote.Default != nil {
		enc.remoteDefault, encDiags = newStateEncryption(enc, cfg.Remote.Default, false, "remote.default")
		diags = append(diags, encDiags...)
	} else {
		enc.remoteDefault = StateEncryptionDisabled()
	}

	if cfg.Remote != nil {
		for _, remoteTarget := range cfg.Remote.Targets {
			// TODO the addr here should be generated in one place.
			addr := "remote.remote_state_datasource." + remoteTarget.Name
			enc.remotes[remoteTarget.Name], encDiags = newStateEncryption(enc, remoteTarget.AsTargetConfig(), false, addr)
			diags = append(diags, encDiags...)
		}
	}
	if diags.HasErrors() {
		return nil, diags
	}
	return enc, diags
}

func (e *encryption) StateFile() StateEncryption {
	return e.statefile
}

func (e *encryption) PlanFile() PlanEncryption {
	return e.planfile
}

func (e *encryption) Backend() StateEncryption {
	return e.backend
}

func (e *encryption) RemoteState(name string) StateEncryption {
	if enc, ok := e.remotes[name]; ok {
		return enc
	}
	return e.remoteDefault
}

// Mostly used in tests
type encryptionDisabled struct{}

func Disabled() Encryption {
	return &encryptionDisabled{}
}
func (e *encryptionDisabled) StateFile() StateEncryption { return StateEncryptionDisabled() }
func (e *encryptionDisabled) PlanFile() PlanEncryption   { return PlanEncryptionDisabled() }
func (e *encryptionDisabled) Backend() StateEncryption   { return StateEncryptionDisabled() }
func (e *encryptionDisabled) RemoteState(name string) StateEncryption {
	return StateEncryptionDisabled()
}
