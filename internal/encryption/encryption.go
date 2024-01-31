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
	RemoteState(string) State
}

type encryption struct {
	// Inputs
	cfg *Config
	reg Registry

	// Used to evaluate hcl expressions
	ctx *hcl.EvalContext
	// Used to build EvalContext (and related mappings)
	keyValues    map[string]map[string]cty.Value
	methodValues map[string]map[string]cty.Value
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
		cfg: cfg,
		reg: reg,

		ctx: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
		},
		keyValues:    make(map[string]map[string]cty.Value),
		methodValues: make(map[string]map[string]cty.Value),
		methods:      make(map[string]Method),

		remote: make(map[string]State),
	}

	// TODO handle cfg == nil

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

func (enc encryption) setupKeyProviders() hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, kpc := range enc.cfg.KeyProviders {
		diags = append(diags, enc.setupKeyProvider(kpc, nil)...)
	}
	return diags
}

func (enc encryption) setupKeyProvider(cfg KeyProviderConfig, stack []KeyProviderConfig) hcl.Diagnostics {
	// Ensure cfg.Type is in keyValues
	if _, ok := enc.keyValues[cfg.Type]; !ok {
		enc.keyValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Check if we have already setup this KeyProvider (due to dependency loading)
	if _, ok := enc.keyValues[cfg.Type][cfg.Name]; ok {
		return nil
	}

	// Check for circular references
	for _, s := range stack {
		if s == cfg {
			return hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Circular reference detected",
				// TODO add the stack trace to the detail message
				Detail: fmt.Sprintf("Can not load key_provider %q %q due to circular reference", cfg.Type, cfg.Name),
			}}
		}
	}
	stack = append(stack, cfg)

	// Lookup definition
	def := enc.reg.KeyProviders[cfg.Type]
	if def == nil {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unknown key_provider type",
			Detail:   fmt.Sprintf("Can not find key_provider type %q for %q in the encryption registry", cfg.Type, cfg.Name),
		}}
	}

	kp := def()

	// Locate Dependencies
	deps, diags := varhcl.VariablesInBody(cfg.Body, kp)
	if diags.HasErrors() {
		return diags
	}

	// Required Dependencies
	for _, dep := range deps {
		if len(dep) != 3 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid key_provider reference",
				Detail:   "Expected reference in form key_provider.type.name",
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		// TODO this should be more defensive
		depRoot := (dep[0].(hcl.TraverseRoot)).Name
		depType := (dep[1].(hcl.TraverseAttr)).Name
		depName := (dep[2].(hcl.TraverseAttr)).Name

		if depRoot != "key_provider" {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid key_provider reference",
				Detail:   "Expected reference in form key_provider.type.name",
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		for _, kpc := range enc.cfg.KeyProviders {
			if kpc.Type == depType && kpc.Name == depName {
				depDiags := enc.setupKeyProvider(kpc, stack)
				diags = append(diags, depDiags...)
				break
			}
		}
	}
	if diags.HasErrors() {
		return diags
	}

	// Init Key Provider
	decodeDiags := gohcl.DecodeBody(cfg.Body, enc.ctx, kp)
	diags = append(diags, decodeDiags...)
	if diags.HasErrors() {
		return diags
	}

	// Execute Key Provider
	data, err := kp.KeyData()
	if err != nil {
		enc.keyValues[cfg.Type][cfg.Name] = cty.UnknownVal(cty.DynamicPseudoType)
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch key data",
			Detail:   fmt.Sprintf("key_provider.%s.%s failed with error: %s", cfg.Type, cfg.Name, err.Error()),
		})
	}

	// Convert data into cty equvalent
	ctyData := make([]cty.Value, len(data))
	for i, b := range data {
		ctyData[i] = cty.NumberIntVal(int64(b))
	}
	enc.keyValues[cfg.Type][cfg.Name] = cty.ListVal(ctyData)

	// Regen ctx
	kpMap := make(map[string]cty.Value)
	for name, kps := range enc.keyValues {
		kpMap[name] = cty.ObjectVal(kps)
	}
	enc.ctx.Variables["key_provider"] = cty.ObjectVal(kpMap)

	return diags
}

func (enc encryption) setupMethods() hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, m := range enc.cfg.Methods {
		diags = append(diags, enc.setupMethod(m)...)
	}

	// Regen ctx
	mMap := make(map[string]cty.Value)
	for name, ms := range enc.methodValues {
		mMap[name] = cty.ObjectVal(ms)
	}
	enc.ctx.Variables["method"] = cty.ObjectVal(mMap)

	return diags
}

func (enc encryption) setupMethod(cfg MethodConfig) hcl.Diagnostics {
	// Ensure cfg.Type is in methodValues
	if _, ok := enc.methodValues[cfg.Type]; !ok {
		enc.methodValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Lookup definition
	def := enc.reg.Methods[cfg.Type]
	if def == nil {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unknown method type",
			Detail:   fmt.Sprintf("Can not find method type %q for %q in the encryption registry", cfg.Type, cfg.Name),
		}}
	}

	method := def()

	// TODO we could use varhcl here to provider better error messages
	diags := gohcl.DecodeBody(cfg.Body, enc.ctx, method)
	if diags.HasErrors() {
		return diags
	}

	// Map from EvalContext vars -> Method
	mIdent := fmt.Sprintf("method.%s.%s", cfg.Type, cfg.Name)
	enc.methodValues[cfg.Type][cfg.Name] = cty.StringVal(mIdent)
	enc.methods[mIdent] = method

	return diags

}

func (enc encryption) setupTargets() hcl.Diagnostics {
	var diags hcl.Diagnostics

	if enc.cfg.StateFile != nil {
		m, mDiags := enc.setupTarget(enc.cfg.StateFile, "statefile")
		diags = append(diags, mDiags...)
		enc.stateFile = NewState(m)
	}
	if enc.cfg.PlanFile != nil {
		m, mDiags := enc.setupTarget(enc.cfg.PlanFile, "planfile")
		diags = append(diags, mDiags...)
		enc.planFile = NewPlan(m)
	}
	if enc.cfg.Backend != nil {
		m, mDiags := enc.setupTarget(enc.cfg.Backend, "backend")
		diags = append(diags, mDiags...)
		enc.backend = NewState(m)
	}

	if enc.cfg.Remote != nil {
		if enc.cfg.Remote.Default != nil {
			m, mDiags := enc.setupTarget(enc.cfg.Remote.Default, "remote_data_source.default")
			diags = append(diags, mDiags...)
			enc.remoteDefault = NewState(m)
		}
		for _, target := range enc.cfg.Remote.Targets {
			m, mDiags := enc.setupTarget(&TargetConfig{
				Enforced: target.Enforced,
				Method:   target.Method,
				Fallback: target.Fallback,
			}, "remote_data_source."+target.Name)
			diags = append(diags, mDiags...)
			enc.remote[target.Name] = NewState(m)
		}
	}

	return diags
}

func (enc encryption) setupTarget(cfg *TargetConfig, name string) ([]Method, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	target := make([]Method, 0)

	// Method referenced by this target
	if cfg.Method != nil {
		var methodIdent string
		decodeDiags := gohcl.DecodeExpression(cfg.Method, enc.ctx, &methodIdent)
		diags = append(diags, decodeDiags...)
		if !diags.HasErrors() {
			if method, ok := enc.methods[methodIdent]; ok {
				target = append(target, method)
			} else {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined encryption method",
					Detail:   fmt.Sprintf("Can not find %q for %q", methodIdent, name),
					Subject:  cfg.Method.Range().Ptr(),
				})
			}
		}
	} else if cfg.Enforced {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption method required",
			Detail:   fmt.Sprintf("%q is enforced, and therefore requires a method to be provided", name),
		})
	}

	// Fallback methods
	if cfg.Fallback != nil {
		fallback, fallbackDiags := enc.setupTarget(cfg.Fallback, name+".fallback")
		diags = append(diags, fallbackDiags...)
		target = append(target, fallback...)
	}

	return target, nil
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
