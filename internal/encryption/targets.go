package encryption

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/zclconf/go-cty/cty"
)

type targetBuilder struct {
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

func (base *baseEncryption) buildTargetMethods(meta map[string][]byte) ([]method.Method, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	builder := &targetBuilder{
		cfg: base.enc.cfg,
		reg: base.enc.reg,

		ctx: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
		},

		metadata: meta,
	}

	diags = append(diags, builder.setupKeyProviders()...)
	if diags.HasErrors() {
		return nil, diags
	}
	diags = append(diags, builder.setupMethods()...)
	if diags.HasErrors() {
		return nil, diags
	}

	return builder.build(base.target, base.name)
}

// build sets up a single target for encryption. It returns the primary and fallback methods for the target, as well
// as a list of diagnostics if the target is invalid.
// The targetName parameter is used for error messages only.
func (e *targetBuilder) build(target *TargetConfig, targetName string) (methods []method.Method, diags hcl.Diagnostics) {
	// Descriptor referenced by this target
	if target.Method != nil {
		var methodIdent string
		decodeDiags := gohcl.DecodeExpression(target.Method, e.ctx, &methodIdent)
		diags = append(diags, decodeDiags...)

		// Only attempt to fetch the method if the decoding was successful
		if !decodeDiags.HasErrors() {
			if method, ok := e.methods[methodIdent]; ok {
				methods = append(methods, method)
			} else {
				// We can't continue if the method is not found
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined encryption method",
					Detail:   fmt.Sprintf("Can not find %q for %q", methodIdent, targetName),
					Subject:  target.Method.Range().Ptr(),
				})
			}
		}
	} else {
		// nil is a nop method
		methods = append(methods, nil)
	}

	// Attempt to fetch the fallback method if it's been configured
	if target.Fallback != nil {
		fallback, fallbackDiags := e.build(target.Fallback, targetName+".fallback")
		diags = append(diags, fallbackDiags...)
		methods = append(methods, fallback...)
	}

	return methods, diags
}
