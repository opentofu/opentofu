package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

// Encryption contains the methods for obtaining a State or Plan correctly configured for a specific
// purpose. If no encryption configuration is present, it returns a passthru method that doesn't do anything.
type Encryption interface {
	StateFile() State
	PlanFile() Plan
	Backend() State
	// RemoteState returns a State suitable for a remote state data source. Note: this function panics
	// if the path of the remote state data source is invalid, but does not panic if it is incorrect.
	RemoteState(string) State
}

type encryption struct {
	// These could technically be local to the ctr, but I've got plans to use them later on in RemoteState
	keyProviders map[string]KeyProvider
	methods      map[string]Method

	stateFile     State
	planFile      Plan
	backend       State
	remoteDefault State
	remote        map[string]State
}

func New(reg Registry, cfg *Config) (Encryption, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	enc := &encryption{
		keyProviders: make(map[string]KeyProvider),
		methods:      make(map[string]Method),
	}

	// BUG: gohcl.DecodeExpression thinks method.foo.bar is a variable.  We will need to build and maintain an EvalContext for this
	// to function properly.  That'll make this code even more fun, but provide for pretty good errors.  Lots of lessons learned from RFC #1042.
	// For now, just pass them in as strings

	// This is a hairy ugly monster that is duck-taped together
	// It is here to show the flow of cfg(reg) -> encryption
	// Please rip out and rewrite this function

	// Process Key Providers

	var loadKeyProvider func(name string, stack []string) (KeyProvider, hcl.Diagnostics)
	loadKeyProvider = func(name string, stack []string) (KeyProvider, hcl.Diagnostics) {
		if found, ok := enc.keyProviders[name]; ok {
			return found, nil
		}
		// BUG: early returns here combined with loading from dependencies will cause the same key provider load to be attempted if it is failing
		// Mostly just a diags issue, but should be fixed at some point with better code

		// Prevent circular dependencies
		for _, s := range stack {
			if s == name {
				panic("TODO diags: circular dependency")
			}
		}
		stack = append(stack, name)

		// Lookup definition
		def := reg.KeyProviders[KeyProviderType(name)]
		if def == nil {
			panic("TODO diags: missing key provider")
		}

		// Decode body -> block
		body := cfg.KeyProviders[name]
		contents, diags := body.Content(def.Schema().BodySchema)
		if diags.HasErrors() {
			return nil, diags
		}

		// Required Dependencies
		deps := make(map[string]KeyProvider)
		for _, depField := range def.Schema().KeyProviderFields {
			if attr, ok := contents.Attributes[depField]; ok {
				var depName string
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &depName)
				diags = append(diags, valDiags...)
				if valDiags.HasErrors() {
					continue
				}

				dep, depDiags := loadKeyProvider(depName, stack)
				diags = append(diags, depDiags...)
				deps[depName] = dep
			}
		}
		if diags.HasErrors() {
			return nil, diags
		}

		// Init Key Provider
		kp, kpDiags := def.Configure(contents, deps)
		diags = append(diags, kpDiags...)
		if diags.HasErrors() {
			return nil, diags
		}

		enc.keyProviders[name] = kp

		return kp, diags
	}

	for name, _ := range cfg.KeyProviders {
		_, kpd := loadKeyProvider(name, nil)
		diags = append(diags, kpd...)
	}

	if diags.HasErrors() {
		return nil, diags
	}

	// Process Methods

	loadMethod := func(name string, body hcl.Body) (Method, hcl.Diagnostics) {
		// Lookup definition
		def := reg.Methods[MethodType(name)]
		if def == nil {
			panic("TODO diags: missing method")
		}

		// Decode body -> block
		contents, diags := body.Content(def.Schema().BodySchema)
		if diags.HasErrors() {
			return nil, diags
		}

		// Required Dependencies
		deps := make(map[string]KeyProvider)
		for _, depField := range def.Schema().KeyProviderFields {
			if attr, ok := contents.Attributes[depField]; ok {
				var depName string
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &depName)
				diags = append(diags, valDiags...)
				if valDiags.HasErrors() {
					continue
				}

				dep, ok := enc.keyProviders[depName]
				if !ok {
					panic("TODO diags: missing key provider for method")
				}
				deps[depName] = dep
			}
		}
		if diags.HasErrors() {
			return nil, diags
		}

		// Init Method

		method, methodDiags := def.Configure(contents, deps)
		diags = append(diags, methodDiags...)
		if diags.HasErrors() {
			return nil, diags
		}

		return method, diags
	}

	for name, body := range cfg.Methods {
		method, mDiags := loadMethod(name, body)
		diags = append(diags, mDiags...)
		enc.methods[name] = method
	}

	if diags.HasErrors() {
		return nil, diags
	}

	var loadTarget func(target *TargetConfig) ([]Method, hcl.Diagnostics)
	loadTarget = func(target *TargetConfig) ([]Method, hcl.Diagnostics) {
		var diags hcl.Diagnostics
		methods := make([]Method, 0)

		// Method referenced by this target
		if len(target.Method) != 0 {
			if method, ok := enc.methods[target.Method]; ok {
				methods = append(methods, method)
			} else {
				// Undefined
				panic("TODO diags: missing method from target")
			}
		} else if target.Enforced {
			panic("TODO diags: enforced")
		}

		// Fallback methods
		if target.Fallback != nil {
			fallback, fallbackDiags := loadTarget(target.Fallback)
			diags = append(diags, fallbackDiags...)
			methods = append(methods, fallback...)
		}

		return methods, nil
	}

	if target, ok := cfg.Targets[ConfigKeyStateFile]; ok {
		m, mDiags := loadTarget(target)
		diags = append(diags, mDiags...)
		enc.stateFile = NewState(m)
	}
	if target, ok := cfg.Targets[ConfigKeyPlanFile]; ok {
		m, mDiags := loadTarget(target)
		diags = append(diags, mDiags...)
		enc.planFile = NewPlan(m)
	}
	if target, ok := cfg.Targets[ConfigKeyBackend]; ok {
		m, mDiags := loadTarget(target)
		diags = append(diags, mDiags...)
		enc.backend = NewState(m)
	}

	if cfg.RemoteTargets != nil {
		if cfg.RemoteTargets.Default != nil {
			m, mDiags := loadTarget(cfg.RemoteTargets.Default)
			diags = append(diags, mDiags...)
			enc.remoteDefault = NewState(m)
		}
		for name, target := range cfg.RemoteTargets.Targets {
			m, mDiags := loadTarget(target)
			diags = append(diags, mDiags...)
			enc.remote[name] = NewState(m)
		}
	}

	return enc, diags
}

func (e *encryption) StateFile() State {
	return e.stateFile
}
func (e *encryption) PlanFile() Plan {
	return e.planFile

}
func (e *encryption) Backend() State {
	return e.backend
}
func (e *encryption) RemoteState(name string) State {
	if state, ok := e.remote[name]; ok {
		return state
	}
	return e.remoteDefault
}
