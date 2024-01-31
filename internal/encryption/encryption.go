package encryption

import (
	"fmt"

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
	stateFile     State
	planFile      Plan
	backend       State
	remoteDefault State
	remote        map[string]State
}

func New(reg Registry, cfg *Config) (Encryption, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	enc := &encryption{}

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

	var attemptedKeyProviders []KeyProviderConfig
	var loadKeyProvider func(kcfg KeyProviderConfig, stack []KeyProviderConfig) hcl.Diagnostics
	loadKeyProvider = func(kcfg KeyProviderConfig, stack []KeyProviderConfig) hcl.Diagnostics {
		// Have we already tried to load this?
		for _, kpc := range attemptedKeyProviders {
			if kpc == kcfg {
				return nil
			}
		}
		attemptedKeyProviders = append(attemptedKeyProviders, kcfg)

		// Prevent circular dependencies
		for _, s := range stack {
			if s == kcfg {
				panic("TODO diags: circular dependency")
			}
		}
		stack = append(stack, kcfg)

		// Lookup definition
		def := reg.KeyProviders[kcfg.Type]
		if def == nil {
			panic("TODO diags: missing key provider")
		}

		kp := def()

		// Locate Dependencies
		deps, depDiags := varhcl.VariablesInBody(kcfg.Body, kp)
		diags = append(diags, depDiags...)
		if depDiags.HasErrors() {
			return diags
		}

		// Required Dependencies
		for _, dep := range deps {
			// BUG: this is not defensive in the slightest...
			depType := (dep[1].(hcl.TraverseAttr)).Name
			depName := (dep[2].(hcl.TraverseAttr)).Name

			for _, kpc := range cfg.KeyProviders {
				if kpc.Type == depType && kpc.Name == depName {
					depDiags := loadKeyProvider(kpc, stack)
					diags = append(diags, depDiags...)
					break
				}
			}
		}
		if diags.HasErrors() {
			return diags
		}

		// Init Key Provider
		decodeDiags := gohcl.DecodeBody(kcfg.Body, ctx, kp)
		diags = append(diags, decodeDiags...)
		if diags.HasErrors() {
			return diags
		}

		data, err := kp.KeyData()
		if err != nil {
			panic(err) // TODO diags
		}
		keyProviders[kcfg.Type][kcfg.Name] = data

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

	for _, kpc := range cfg.KeyProviders {
		diags = append(diags, loadKeyProvider(kpc, nil)...)
	}

	if diags.HasErrors() {
		return nil, diags
	}

	// Process Methods

	methods := make(map[string]Method)
	methodVars := make(map[string]map[string]cty.Value)
	for name := range reg.Methods {
		methodVars[name] = make(map[string]cty.Value)
	}

	loadMethod := func(mcfg MethodConfig) hcl.Diagnostics {
		// Lookup definition
		def := reg.Methods[mcfg.Type]
		if def == nil {
			panic("TODO diags: missing method")
		}

		method := def()

		// TODO we could use varhcl here to provider better error messages

		decodeDiags := gohcl.DecodeBody(mcfg.Body, ctx, method)
		diags = append(diags, decodeDiags...)

		if diags.HasErrors() {
			return diags
		}

		// Map from EvalContext vars -> Method
		mIdent := fmt.Sprintf("method.%s.%s", mcfg.Type, mcfg.Name)
		methodVars[mcfg.Type][mcfg.Name] = cty.StringVal(mIdent)
		methods[mIdent] = method

		return diags
	}

	for _, m := range cfg.Methods {
		diags = append(diags, loadMethod(m)...)
	}

	// Regen ctx
	mMap := make(map[string]cty.Value)
	for name, ms := range methodVars {
		mMap[name] = cty.ObjectVal(ms)
	}

	ctx.Variables["method"] = cty.ObjectVal(mMap)

	if diags.HasErrors() {
		return nil, diags
	}

	// Load Targets

	var loadTarget func(target *TargetConfig) ([]Method, hcl.Diagnostics)
	loadTarget = func(target *TargetConfig) ([]Method, hcl.Diagnostics) {
		var diags hcl.Diagnostics
		result := make([]Method, 0)

		// Method referenced by this target
		if target.Method != nil {
			var methodIdent string
			decodeDiags := gohcl.DecodeExpression(target.Method, ctx, &methodIdent)
			diags = append(diags, decodeDiags...)
			if diags.HasErrors() {
				panic(diags.Error())
			}

			if method, ok := methods[methodIdent]; ok {
				result = append(result, method)
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
			result = append(result, fallback...)
		}

		return result, nil
	}

	if cfg.StateFile != nil {
		m, mDiags := loadTarget(cfg.StateFile)
		diags = append(diags, mDiags...)
		enc.stateFile = NewState(m)
	}
	if cfg.PlanFile != nil {
		m, mDiags := loadTarget(cfg.PlanFile)
		diags = append(diags, mDiags...)
		enc.planFile = NewPlan(m)
	}
	if cfg.Backend != nil {
		m, mDiags := loadTarget(cfg.Backend)
		diags = append(diags, mDiags...)
		enc.backend = NewState(m)
	}

	if cfg.Remote != nil {
		if cfg.Remote.Default != nil {
			m, mDiags := loadTarget(cfg.Remote.Default)
			diags = append(diags, mDiags...)
			enc.remoteDefault = NewState(m)
		}
		for _, target := range cfg.Remote.Targets {
			m, mDiags := loadTarget(&TargetConfig{
				Enforced: target.Enforced,
				Method:   target.Method,
				Fallback: target.Fallback,
			})
			diags = append(diags, mDiags...)
			enc.remote[target.Name] = NewState(m)
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
