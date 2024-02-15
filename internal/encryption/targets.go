package encryption

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// setupTarget sets up a single target for encryption. It returns the primary and fallback methods for the target, as well
// as a list of diagnostics if the target is invalid.
// The targetName parameter is used for error messages only.
func (e *encryptor) setupTarget(cfg *TargetConfig, targetName string) (primary method.Method, fallback method.Method, diags hcl.Diagnostics) {
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
