// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/zclconf/go-cty/cty"
)

type targetBuilder struct {
	cfg *config.EncryptionConfig
	reg registry.Registry

	// Used to evaluate hcl expressions
	ctx *hcl.EvalContext

	keyProviderMetadata map[keyprovider.Addr][]byte

	// Used to build EvalContext (and related mappings)
	keyValues    map[string]map[string]cty.Value
	methodValues map[string]map[string]cty.Value
	methods      map[method.Addr]method.Method
}

func (base *baseEncryption) buildTargetMethods(meta map[keyprovider.Addr][]byte) ([]method.Method, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	builder := &targetBuilder{
		cfg: base.enc.cfg,
		reg: base.enc.reg,

		ctx: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
		},

		keyProviderMetadata: meta,
	}

	keyDiags := append(diags, builder.setupKeyProviders()...)
	diags = append(diags, keyDiags...)
	if diags.HasErrors() {
		return nil, diags
	}
	methodDiags := append(diags, builder.setupMethods()...)
	diags = append(diags, methodDiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	methods, targetDiags := builder.build(base.target, base.name)
	diags = append(diags, targetDiags...)

	if base.enforced {
		for _, m := range methods {
			if unencrypted.Is(m) {
				return nil, append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Unencrypted method is forbidden",
					Detail:   "Unable to use `unencrypted` method since the `enforced` flag is used.",
				})
			}
		}
	}

	return methods, diags
}

// build sets up a single target for encryption. It returns the primary and fallback methods for the target, as well
// as a list of diagnostics if the target is invalid.
// The targetName parameter is used for error messages only.
func (e *targetBuilder) build(target *config.TargetConfig, targetName string) (methods []method.Method, diags hcl.Diagnostics) {

	// gohcl has some weirdness around attributes that are not provided, but are hcl.Expressions
	// They will set the attribute field to a static null expression
	// https://github.com/hashicorp/hcl/blob/main/gohcl/decode.go#L112-L118

	// Descriptor referenced by this target
	var methodIdent *string
	decodeDiags := gohcl.DecodeExpression(target.Method, e.ctx, &methodIdent)
	diags = append(diags, decodeDiags...)

	// Only attempt to fetch the method if the decoding was successful
	if !decodeDiags.HasErrors() {
		if methodIdent != nil {
			if method, ok := e.methods[method.Addr(*methodIdent)]; ok {
				methods = append(methods, method)
			} else {
				// We can't continue if the method is not found
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Undefined encryption method",
					Detail:   fmt.Sprintf("Can not find %q for %q", *methodIdent, targetName),
					Subject:  target.Method.Range().Ptr(),
				})
			}
		} else {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Missing encryption method",
				Detail:   fmt.Sprintf("undefined or null method used for %q", targetName),
				Subject:  target.Method.Range().Ptr(),
			})
		}
	}

	// Attempt to fetch the fallback method if it's been configured
	if target.Fallback != nil {
		fallback, fallbackDiags := e.build(target.Fallback, targetName+".fallback")
		diags = append(diags, fallbackDiags...)
		methods = append(methods, fallback...)
	}

	return methods, diags
}
