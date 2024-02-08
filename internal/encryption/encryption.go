package encryption

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/varhcl"
	"github.com/zclconf/go-cty/cty"
)

// Encryption contains the methods for obtaining a StateEncryption or PlanEncryption correctly configured for a specific
// purpose. If no encryption configuration is present, it returns a passthru method that doesn't do anything.
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

func (e *encryption) setupKeyProviders() hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, kpc := range e.cfg.KeyProviders {
		diags = append(diags, e.setupKeyProvider(kpc, nil)...)
	}
	return diags
}

func (e *encryption) setupKeyProvider(cfg KeyProviderConfig, stack []KeyProviderConfig) hcl.Diagnostics {
	// Ensure cfg.Type is in keyValues
	if _, ok := e.keyValues[cfg.Type]; !ok {
		e.keyValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Check if we have already setup this Descriptor (due to dependency loading)
	if _, ok := e.keyValues[cfg.Type][cfg.Name]; ok {
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
	keyProviderDescriptor, err := e.reg.GetKeyProvider(keyprovider.ID(cfg.Type))
	if err != nil {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unknown key_provider type",
			Detail:   err.Error(),
		}}
	}

	kpcfg := keyProviderDescriptor.ConfigStruct()

	// Locate Dependencies
	deps, diags := varhcl.VariablesInBody(cfg.Body, kpcfg)
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

		for _, kpc := range e.cfg.KeyProviders {
			if kpc.Type == depType && kpc.Name == depName {
				depDiags := e.setupKeyProvider(kpc, stack)
				diags = append(diags, depDiags...)
				break
			}
		}
	}
	if diags.HasErrors() {
		return diags
	}

	// Init Key Provider
	decodeDiags := gohcl.DecodeBody(cfg.Body, e.ctx, kpcfg)
	diags = append(diags, decodeDiags...)
	if diags.HasErrors() {
		return diags
	}

	// Execute Key Provider
	keyProvider, err := kpcfg.Build()
	if err != nil {
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch key data",
			Detail:   fmt.Sprintf("key_provider.%s.%s failed with error: %s", cfg.Type, cfg.Name, err.Error()),
		})
	}
	data, err := keyProvider.Provide()
	if err != nil {
		e.keyValues[cfg.Type][cfg.Name] = cty.UnknownVal(cty.DynamicPseudoType)
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch key data",
			Detail:   fmt.Sprintf("key_provider.%s.%s failed with error: %s", cfg.Type, cfg.Name, err.Error()),
		})
	}

	// Convert data into cty equivalent
	ctyData := make([]cty.Value, len(data))
	for i, b := range data {
		ctyData[i] = cty.NumberIntVal(int64(b))
	}
	e.keyValues[cfg.Type][cfg.Name] = cty.ListVal(ctyData)

	// Regen ctx
	kpMap := make(map[string]cty.Value)
	for name, kps := range e.keyValues {
		kpMap[name] = cty.ObjectVal(kps)
	}
	e.ctx.Variables["key_provider"] = cty.ObjectVal(kpMap)

	return diags
}

func (e *encryption) setupMethods() hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, m := range e.cfg.Methods {
		diags = append(diags, e.setupMethod(m)...)
	}

	// Regen ctx
	mMap := make(map[string]cty.Value)
	for name, ms := range e.methodValues {
		mMap[name] = cty.ObjectVal(ms)
	}
	e.ctx.Variables["method"] = cty.ObjectVal(mMap)

	return diags
}

func (e *encryption) setupMethod(cfg MethodConfig) hcl.Diagnostics {
	// Ensure cfg.Type is in methodValues
	if _, ok := e.methodValues[cfg.Type]; !ok {
		e.methodValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Lookup definition
	encryptionMethod, err := e.reg.GetMethod(method.ID(cfg.Type))
	if err != nil {
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unknown method type",
			Detail:   err.Error(),
		}}
	}

	methodcfg := encryptionMethod.ConfigStruct()

	// TODO we could use varhcl here to provider better error messages
	diags := gohcl.DecodeBody(cfg.Body, e.ctx, methodcfg)
	if diags.HasErrors() {
		return diags
	}

	// Map from EvalContext vars -> Descriptor
	mIdent := MethodAddr(cfg.Type, cfg.Name)
	e.methodValues[cfg.Type][cfg.Name] = cty.StringVal(mIdent)
	e.methods[mIdent], err = methodcfg.Build()
	if err != nil {
		// TODO this error handling could use some work
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Method configuration failed",
			Detail:   err.Error(),
		}}
	}

	return diags

}

func (e *encryption) setupTargets() hcl.Diagnostics {
	var diags hcl.Diagnostics

	if e.cfg.StateFile != nil {
		m, mDiags := e.setupTarget(e.cfg.StateFile, "statefile")
		diags = append(diags, mDiags...)
		e.stateFile = NewState(m)
	}
	if e.cfg.PlanFile != nil {
		m, mDiags := e.setupTarget(e.cfg.PlanFile, "planfile")
		diags = append(diags, mDiags...)
		e.planFile = NewPlan(m)
	}
	if e.cfg.Backend != nil {
		m, mDiags := e.setupTarget(e.cfg.Backend, "backend")
		diags = append(diags, mDiags...)
		e.backend = NewState(m)
	}

	if e.cfg.Remote != nil {
		if e.cfg.Remote.Default != nil {
			m, mDiags := e.setupTarget(e.cfg.Remote.Default, "remote_data_source.default")
			diags = append(diags, mDiags...)
			e.remoteDefault = NewState(m)
		}
		for _, target := range e.cfg.Remote.Targets {
			m, mDiags := e.setupTarget(&TargetConfig{
				Enforced: target.Enforced,
				Method:   target.Method,
				Fallback: target.Fallback,
			}, "remote_data_source."+target.Name)
			diags = append(diags, mDiags...)
			e.remote[target.Name] = NewState(m)
		}
	}

	return diags
}

func (e *encryption) setupTarget(cfg *TargetConfig, name string) ([]method.Method, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	target := make([]method.Method, 0)

	// Descriptor referenced by this target
	if cfg.Method != nil {
		var methodIdent string
		decodeDiags := gohcl.DecodeExpression(cfg.Method, e.ctx, &methodIdent)
		diags = append(diags, decodeDiags...)
		if !diags.HasErrors() {
			if method, ok := e.methods[methodIdent]; ok {
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
		fallback, fallbackDiags := e.setupTarget(cfg.Fallback, name+".fallback")
		diags = append(diags, fallbackDiags...)
		target = append(target, fallback...)
	}

	return target, nil
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
