package encryption

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/zclconf/go-cty/cty"
)

func (e *targetBuilder) setupMethods() hcl.Diagnostics {
	var diags hcl.Diagnostics

	e.methodValues = make(map[string]map[string]cty.Value)
	e.methods = make(map[string]method.Method)

	for _, m := range e.cfg.MethodConfigs {
		diags = append(diags, e.setupMethod(m)...)
	}

	// Regenerate the context now that the method is loaded
	mMap := make(map[string]cty.Value)
	for name, ms := range e.methodValues {
		mMap[name] = cty.ObjectVal(ms)
	}
	e.ctx.Variables["method"] = cty.ObjectVal(mMap)

	return diags
}

// setupMethod sets up a single method for encryption. It returns a list of diagnostics if the method is invalid.
func (e *targetBuilder) setupMethod(cfg MethodConfig) hcl.Diagnostics {
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
			Summary:  "Encryption method configuration failed",
			Detail:   err.Error(),
		}}
	}
	e.methods[mIdent] = m
	return nil
}
