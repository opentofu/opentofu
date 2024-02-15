package encryption

import (
	"github.com/opentofu/opentofu/internal/encryption/registry"
)

// Encryption contains the methods for obtaining a StateEncryption or PlanEncryption correctly configured for a specific
// purpose. If no encryption configuration is present, it should return a pass through method that doesn't do anything.
type Encryption interface {
	// TODO either the Encryption interface, or the New func and encryption struct should be moved to a different
	// to avoid nasty circular dependency issues.

	StateFile() StateEncryption
	PlanFile() PlanEncryption
	Backend() StateEncryption
	RemoteState(string) StateEncryption
}
type encryption struct {
	// Inputs
	cfg *Config
	reg registry.Registry
}

// New creates a new Encryption instance from the given configuration and registry.
func New(reg registry.Registry, cfg *Config) Encryption {
	return &encryption{
		cfg: cfg,
		reg: reg,
	}
}
func (e *encryption) StateFile() StateEncryption {
	return NewEnforcableState(e, e.cfg.StateFile, "statefile")
}

func (e *encryption) PlanFile() PlanEncryption {
	return NewPlan(e, e.cfg.PlanFile, "planfile")
}

func (e *encryption) Backend() StateEncryption {
	return NewEnforcableState(e, e.cfg.Backend, "backend")
}
func (e *encryption) RemoteState(name string) StateEncryption {
	for _, remoteTarget := range e.cfg.Remote.Targets {
		if remoteTarget.Name == name {
			return NewState(e, remoteTarget.AsTargetConfig(), name)
		}
	}
	return NewState(e, e.cfg.Remote.Default, "remote default")
}
