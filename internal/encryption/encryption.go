// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/registry"
)

// Encryption contains the methods for obtaining a StateEncryption or PlanEncryption correctly configured for a specific
// purpose. If no encryption configuration is present, it should return a pass through method that doesn't do anything.
type Encryption interface {
	// State produces a StateEncryption overlay for encrypting and decrypting state files for local storage.
	State() StateEncryption

	// Plan produces a PlanEncryption overlay for encrypting and decrypting plan files.
	Plan() PlanEncryption

	// RemoteState produces a StateEncryption for reading remote states using the terraform_remote_state data
	// source.
	RemoteState(string) StateEncryption
}

type encryption struct {
	state         StateEncryption
	plan          PlanEncryption
	remoteDefault StateEncryption
	remotes       map[string]StateEncryption

	// Inputs
	cfg *config.EncryptionConfig
	reg registry.Registry
}

// New creates a new Encryption provider from the given configuration and registry.
func New(reg registry.Registry, cfg *config.EncryptionConfig, staticEval *configs.StaticEvaluator) (Encryption, hcl.Diagnostics) {
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

	if cfg.State != nil {
		enc.state, encDiags = newStateEncryption(enc, cfg.State.AsTargetConfig(), cfg.State.Enforced, "state", staticEval)
		diags = append(diags, encDiags...)
	} else {
		enc.state = StateEncryptionDisabled()
	}

	if cfg.Plan != nil {
		enc.plan, encDiags = newPlanEncryption(enc, cfg.Plan.AsTargetConfig(), cfg.Plan.Enforced, "plan", staticEval)
		diags = append(diags, encDiags...)
	} else {
		enc.plan = PlanEncryptionDisabled()
	}

	if cfg.Remote != nil && cfg.Remote.Default != nil {
		enc.remoteDefault, encDiags = newStateEncryption(enc, cfg.Remote.Default, false, "remote.default", staticEval)
		diags = append(diags, encDiags...)
	} else {
		enc.remoteDefault = StateEncryptionDisabled()
	}

	if cfg.Remote != nil {
		for _, remoteTarget := range cfg.Remote.Targets {
			// TODO the addr here should be generated in one place.
			addr := "remote.remote_state_datasource." + remoteTarget.Name
			enc.remotes[remoteTarget.Name], encDiags = newStateEncryption(enc, remoteTarget.AsTargetConfig(), false, addr, staticEval)
			diags = append(diags, encDiags...)
		}
	}
	if diags.HasErrors() {
		return nil, diags
	}
	return enc, diags
}

func (e *encryption) State() StateEncryption {
	return e.state
}

func (e *encryption) Plan() PlanEncryption {
	return e.plan
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
func (e *encryptionDisabled) State() StateEncryption { return StateEncryptionDisabled() }
func (e *encryptionDisabled) Plan() PlanEncryption   { return PlanEncryptionDisabled() }
func (e *encryptionDisabled) RemoteState(name string) StateEncryption {
	return StateEncryptionDisabled()
}
