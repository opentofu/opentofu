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
	StateFile() (StateEncryption, hcl.Diagnostics)

	// PlanFile produces a PlanEncryption overlay for encrypting and decrypting plan files.
	PlanFile() (PlanEncryption, hcl.Diagnostics)

	// Backend produces a StateEncryption overlay for storing state files on remote backends, such as an S3 bucket.
	Backend() (StateEncryption, hcl.Diagnostics)

	// RemoteState produces a ReadOnlyStateEncryption for reading remote states using the terraform_remote_state data
	// source.
	RemoteState(string) (ReadOnlyStateEncryption, hcl.Diagnostics)
}

type encryption struct {
	// Inputs
	cfg *config.Config
	reg registry.Registry
}

// New creates a new Encryption provider from the given configuration and registry.
func New(reg registry.Registry, cfg *config.Config) Encryption {
	return &encryption{
		cfg: cfg,
		reg: reg,
	}
}

func (e *encryption) StateFile() (StateEncryption, hcl.Diagnostics) {
	return newStateEncryption(e, e.cfg.StateFile.AsTargetConfig(), e.cfg.StateFile.Enforced, "statefile")
}

func (e *encryption) PlanFile() (PlanEncryption, hcl.Diagnostics) {
	return newPlanEncryption(e, e.cfg.PlanFile.AsTargetConfig(), e.cfg.PlanFile.Enforced, "planfile")
}

func (e *encryption) Backend() (StateEncryption, hcl.Diagnostics) {
	return newStateEncryption(e, e.cfg.StateFile.AsTargetConfig(), e.cfg.StateFile.Enforced, "backend")
}

func (e *encryption) RemoteState(name string) (ReadOnlyStateEncryption, hcl.Diagnostics) {
	for _, remoteTarget := range e.cfg.Remote.Targets {
		if remoteTarget.Name == name {
			// TODO the addr here should be generated in one place.
			addr := "remote.remote_state_datasource." + remoteTarget.Name
			return newStateEncryption(
				e, remoteTarget.AsTargetConfig(), false, addr,
			)
		}
	}
	return newStateEncryption(e, e.cfg.Remote.Default, false, "remote.default")
}
