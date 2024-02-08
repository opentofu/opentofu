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

	// Used to evaluate hcl expressions
	ctx *hcl.EvalContext

	// Used to build EvalContext (and related mappings)
	keyValues    map[string]map[string]cty.Value
	methodValues map[string]map[string]cty.Value
	methods      map[string]method.Method

	stateFile     StateEncryption
	planFile      PlanEncryption
	backend       StateEncryption
	remoteDefault StateEncryption
	remote        map[string]StateEncryption
}

// New creates a new Encryption instance from the given configuration and registry. It returns a list of diagnostics if
// the configuration is invalid.
func New(reg registry.Registry, cfg *Config) (Encryption, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	enc := &encryption{
		cfg: cfg,
		reg: reg,

		ctx: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
		},
		keyValues:    make(map[string]map[string]cty.Value),
		methodValues: make(map[string]map[string]cty.Value),
		methods:      make(map[string]method.Method),

		remote: make(map[string]StateEncryption),
	}

	if cfg == nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid encryption configuration",
			Detail:   "No configuration provided",
		})
		return nil, diags
	}

	diags = append(diags, enc.setupKeyProviders()...)
	if diags.HasErrors() {
		return nil, diags
	}
	diags = append(diags, enc.setupMethods()...)
	if diags.HasErrors() {
		return nil, diags
	}
	diags = append(diags, enc.setupTargets()...)
	if diags.HasErrors() {
		return nil, diags
	}

	return enc, diags
}

func (e *encryption) StateFile() StateEncryption {
	return e.stateFile
}

func (e *encryption) PlanFile() PlanEncryption {
	return e.planFile
}

func (e *encryption) Backend() StateEncryption {
	return e.backend
}

func (e *encryption) RemoteState(name string) StateEncryption {
	if state, ok := e.remote[name]; ok {
		return state
	}
	return e.remoteDefault
}
