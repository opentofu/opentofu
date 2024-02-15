package encryption

import (
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
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

type encryptor struct {
	cfg *Config
	reg registry.Registry

	// Used to evaluate hcl expressions
	ctx *hcl.EvalContext

	metadata map[string][]byte

	// Used to build EvalContext (and related mappings)
	keyValues    map[string]map[string]cty.Value
	methodValues map[string]map[string]cty.Value
	methods      map[string]method.Method
}

// New creates a new Encryption instance from the given configuration and registry.
func New(reg registry.Registry, cfg *Config) Encryption {
	return &encryption{
		cfg: cfg,
		reg: reg,
	}
}
func (e *encryption) newEncryptor(meta map[string][]byte) (*encryptor, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	enc := &encryptor{
		cfg: e.cfg,
		reg: e.reg,

		ctx: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
		},

		metadata: meta,

		keyValues:    make(map[string]map[string]cty.Value),
		methodValues: make(map[string]map[string]cty.Value),
		methods:      make(map[string]method.Method),
	}

	diags = append(diags, enc.setupKeyProviders()...)
	if diags.HasErrors() {
		return nil, diags
	}
	diags = append(diags, enc.setupMethods()...)
	if diags.HasErrors() {
		return nil, diags
	}

	return enc, diags
}

func (e *encryption) StateFile() StateEncryption {
	return NewState(e, e.cfg.StateFile, "statefile")
}

func (e *encryption) PlanFile() PlanEncryption {
	return NewPlan(e, e.cfg.PlanFile, "planfile")
}

func (e *encryption) Backend() StateEncryption {
	return NewState(e, e.cfg.Backend, "backend")
}

func (e *encryption) RemoteState(name string) StateEncryption {
	for _, remoteTarget := range e.cfg.Remote.Targets {
		if remoteTarget.Name == name {
			return NewState(e, remoteTarget.AsTargetConfig(), name)
		}
	}
	return NewState(e, e.cfg.Remote.Default, "remote default")
}
