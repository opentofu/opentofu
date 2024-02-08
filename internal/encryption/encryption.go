package encryption

import (
	"errors"
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

// setupMethod sets up a single method for encryption. It returns a list of diagnostics if the method is invalid.
func (e *encryption) setupMethod(cfg MethodConfig) hcl.Diagnostics {
	// Ensure cfg.Type is in methodValues
	if _, ok := e.methodValues[cfg.Type]; !ok {
		e.methodValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Lookup the definition of the encryption method from the registry
	encryptionMethod, err := e.reg.GetMethod(method.ID(cfg.Type))
	if err != nil {

		// Handle if the method was not found
		var notFoundError *registry.MethodNotFoundError
		if errors.Is(err, notFoundError) {
			return hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unknown encryption method type",
				Detail:   fmt.Sprintf("Can not find %q", cfg.Type),
			}}
		}

		// Or, we don't know the error type, so we'll just return it as a generic error
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Error fetching encryption method %q", cfg.Type),
			Detail:   err.Error(),
		}}
	}

	// TODO: we could use varhcl here to provider better error messages
	methodConfig := encryptionMethod.ConfigStruct()
	diags := gohcl.DecodeBody(cfg.Body, e.ctx, methodConfig)
	if diags.HasErrors() {
		return diags
	}

	// Map from EvalContext vars -> Descriptor
	mIdent := MethodAddr(cfg.Type, cfg.Name)
	e.methodValues[cfg.Type][cfg.Name] = cty.StringVal(mIdent)
	m, err := methodConfig.Build()
	if err != nil {
		// TODO this error handling could use some work
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Method configuration failed",
			Detail:   err.Error(),
		}}
	}
	e.methods[mIdent] = m
	return nil
}

// setupTargets sets up the targets for encryption. It returns a list of diagnostics if any of the targets are invalid.
// It will set up the encryption targets for the state file, plan file, backend and remote state.
func (e *encryption) setupTargets() hcl.Diagnostics {
	var diags hcl.Diagnostics

	if e.cfg.StateFile != nil {
		m, fb, mDiags := e.setupTarget(e.cfg.StateFile, "statefile")
		diags = append(diags, mDiags...)
		e.stateFile = NewState(m, fb)
	}

	if e.cfg.PlanFile != nil {
		m, fb, mDiags := e.setupTarget(e.cfg.PlanFile, "planfile")
		diags = append(diags, mDiags...)
		e.planFile = NewPlan(m, fb)
	}

	if e.cfg.Backend != nil {
		m, fb, mDiags := e.setupTarget(e.cfg.Backend, "backend")
		diags = append(diags, mDiags...)
		e.backend = NewState(m, fb)
	}

	if e.cfg.Remote != nil {
		if e.cfg.Remote.Default != nil {
			m, fb, mDiags := e.setupTarget(e.cfg.Remote.Default, "remote_data_source.default")
			diags = append(diags, mDiags...)
			e.remoteDefault = NewState(m, fb)
		}
		for _, target := range e.cfg.Remote.Targets {
			m, fb, mDiags := e.setupTarget(&TargetConfig{
				Enforced: target.Enforced,
				Method:   target.Method,
				Fallback: target.Fallback,
			}, "remote_data_source."+target.Name)
			diags = append(diags, mDiags...)
			e.remote[target.Name] = NewState(m, fb)
		}
	}

	return diags
}

// setupTarget sets up a single target for encryption. It returns the primary and fallback methods for the target, as well
// as a list of diagnostics if the target is invalid.
// The targetName parameter is used for error messages only.
func (e *encryption) setupTarget(cfg *TargetConfig, targetName string) (primary method.Method, fallback method.Method, diags hcl.Diagnostics) {
	// ensure that the method is defined when Enforced is true
	if cfg.Enforced && cfg.Method == nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption method required",
			Detail:   fmt.Sprintf("%q is enforced, and therefore requires a method to be provided", targetName),
		})

		return nil, nil, diags
	}

	// Descriptor referenced by this target
	if cfg.Method != nil {
		var methodIdent string
		decodeDiags := gohcl.DecodeExpression(cfg.Method, e.ctx, &methodIdent)
		diags = append(diags, decodeDiags...)

		// Only attempt to fetch the method if the decoding was successful
		if !diags.HasErrors() {
			if method, ok := e.methods[methodIdent]; ok {
				primary = method
			} else {
				// We can't continue if the method is not found
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined encryption method",
					Detail:   fmt.Sprintf("Can not find %q for %q", methodIdent, targetName),
					Subject:  cfg.Method.Range().Ptr(),
				})
			}

		}

	}

	// Attempt to fetch the fallback method if it's been configured
	if cfg.Fallback != nil {
		fb, _, fallbackDiags := e.setupTarget(cfg.Fallback, targetName+".fallback")
		diags = append(diags, fallbackDiags...)
		fallback = fb
	}

	return primary, fallback, diags
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
