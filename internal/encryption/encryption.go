package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/varhcl"
	"github.com/zclconf/go-cty/cty"
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
	methods map[string]Method

	stateFile     State
	planFile      Plan
	backend       State
	remoteDefault State
	remote        map[string]State
}

func New(reg Registry, cfg *Config) (Encryption, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	enc := &encryption{
		methods: make(map[string]Method),
	}

	// This is a hairy ugly monster that is duck-taped together
	// It is here to show the flow of cfg(reg) -> encryption
	// Please rip out and rewrite this function

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
	}

	// Process Key Providers

	keyProviders := make(map[string]map[string][]byte)
	for name := range reg.KeyProviders {
		keyProviders[name] = make(map[string][]byte)
	}

	var attemptedKeyProviders []string
	var loadKeyProvider func(name string, stack []string) hcl.Diagnostics
	loadKeyProvider = func(name string, stack []string) hcl.Diagnostics {
		// Have we already tried to load this?
		for _, kpn := range attemptedKeyProviders {
			if kpn == name {
				return nil
			}
		}
		attemptedKeyProviders = append(attemptedKeyProviders, name)

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

		kp := def()
		body := cfg.KeyProviders[name]

		// Locate Dependencies
		deps, depDiags := varhcl.VariablesInBody(body, kp)
		diags = append(diags, depDiags...)
		if depDiags.HasErrors() {
			return diags
		}

		// Required Dependencies
		for _, dep := range deps {
			// BUG: this is not defensive in the slightest...
			depType := (dep[1].(hcl.TraverseAttr)).Name
			depName := (dep[2].(hcl.TraverseAttr)).Name
			depIdent := KeyProviderAddr(depType, depName)

			depDiags := loadKeyProvider(depIdent, stack)
			diags = append(diags, depDiags...)
		}
		if diags.HasErrors() {
			return diags
		}

		// Init Key Provider
		decodeDiags := gohcl.DecodeBody(body, ctx, kp)
		diags = append(diags, decodeDiags...)
		if diags.HasErrors() {
			return diags
		}

		data, err := kp.KeyData()
		if err != nil {
			panic(err) // TODO diags
		}
		keyProviders[KeyProviderType(name)][KeyProviderName(name)] = data

		// Regen ctx
		kpMap := make(map[string]cty.Value)
		for name, kps := range keyProviders {
			subMap := make(map[string]cty.Value)
			for kpn, bytes := range kps {
				// This is super weird, but it works
				bl := make([]cty.Value, len(bytes))
				for i, b := range bytes {
					bl[i] = cty.NumberIntVal(int64(b))
				}

				subMap[kpn] = cty.ListVal(bl)
			}
			kpMap[name] = cty.ObjectVal(subMap)
		}

		ctx.Variables["key_provider"] = cty.ObjectVal(kpMap)

		return diags
	}

	for name, _ := range cfg.KeyProviders {
		kpd := loadKeyProvider(name, nil)
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

		method := def()

		// TODO we could use varhcl here to provider better error messages

		decodeDiags := gohcl.DecodeBody(body, ctx, method)
		diags = append(diags, decodeDiags...)

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

	// TODO inject methods into ctx for use in loadTarget

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
