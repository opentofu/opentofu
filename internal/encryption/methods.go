// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/zclconf/go-cty/cty"
)

func (e *targetBuilder) setupMethods() hcl.Diagnostics {
	var diags hcl.Diagnostics

	e.methodValues = make(map[string]map[string]cty.Value)
	e.methods = make(map[method.Addr]method.Method)

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
func (e *targetBuilder) setupMethod(cfg config.MethodConfig) hcl.Diagnostics {
	addr, diags := cfg.Addr()
	if diags.HasErrors() {
		return diags
	}

	// Ensure cfg.Type is in methodValues
	if _, ok := e.methodValues[cfg.Type]; !ok {
		e.methodValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Lookup the definition of the encryption method from the registry
	encryptionMethod, err := e.reg.GetMethodDescriptor(method.ID(cfg.Type))
	if err != nil {

		// Handle if the method was not found
		var notFoundError *registry.MethodNotFoundError
		if errors.Is(err, notFoundError) {
			return append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unknown encryption method type",
				Detail:   fmt.Sprintf("Can not find %q", cfg.Type),
			})
		}

		// Or, we don't know the error type, so we'll just return it as a generic error
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Error fetching encryption method %q", cfg.Type),
			Detail:   err.Error(),
		})
	}

	// TODO: we could use varhcl here to provider better error messages
	methodConfig := encryptionMethod.ConfigStruct()
	methodDiags := gohcl.DecodeBody(cfg.Body, e.ctx, methodConfig)
	diags = append(diags, methodDiags...)
	if diags.HasErrors() {
		return diags
	}

	e.methodValues[cfg.Type][cfg.Name] = cty.StringVal(string(addr))
	m, err := methodConfig.Build()
	if err != nil {
		// TODO this error handling could use some work
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption method configuration failed",
			Detail:   err.Error(),
		})
	}
	e.methods[addr] = m

	if unencrypted.Is(m) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Unencrypted method configured",
			Detail:   "Method unencrypted is present in configuration. This is a security risk and should only be enabled during migrations.",
		})
	}

	return diags
}

// extractMethodsAddrs is preparing the values for the eval context that is used later for parsing.
func (base *baseEncryption) extractMethodsAddrs() (map[string]cty.Value, hcl.Diagnostics) {
	var (
		diags hcl.Diagnostics

		temp = map[string]map[string]cty.Value{}
	)

	for _, methodCfg := range base.enc.cfg.MethodConfigs {
		if _, ok := temp[methodCfg.Type]; !ok {
			temp[methodCfg.Type] = map[string]cty.Value{}
		}
		addr, diag := methodCfg.Addr()
		diags = append(diags, diag...)
		if diags.HasErrors() {
			return nil, diags
		}
		temp[methodCfg.Type][methodCfg.Name] = cty.StringVal(string(addr))
	}
	out := map[string]cty.Value{}
	for methodType, methods := range temp {
		out[methodType] = cty.ObjectVal(methods)
	}
	return out, diags
}

func (base *baseEncryption) requiredMethods(ctx *hcl.EvalContext, target *config.TargetConfig, targetName string) ([]config.MethodConfig, hcl.Diagnostics) {
	var (
		diags   hcl.Diagnostics
		methods []config.MethodConfig
	)
	methodByAddr := func(addr string) (*config.MethodConfig, hcl.Diagnostics) {
		for _, mc := range base.enc.cfg.MethodConfigs {
			mAddrs, diag := mc.Addr()
			if diag.HasErrors() {
				return nil, diag
			}
			if string(mAddrs) == addr {
				return &mc, nil
			}
		}
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined encryption method",
			Detail:   fmt.Sprintf("Can not find %q for %q", addr, targetName),
			Subject:  target.Method.Range().Ptr(),
		}}
	}

	var methodIdent *string
	decodeDiags := gohcl.DecodeExpression(target.Method, ctx, &methodIdent)
	diags = append(diags, decodeDiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	if methodIdent == nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing encryption method",
			Detail:   fmt.Sprintf("undefined or null method used for %q", targetName),
			Subject:  target.Method.Range().Ptr(),
		})
	}
	met, metDiag := methodByAddr(*methodIdent)
	diags = append(diags, metDiag...)
	if diags.HasErrors() {
		return nil, diags
	}
	methods = append(methods, *met)

	// Attempt to fetch the fallback method if it's been configured
	if target.Fallback != nil {
		fallback, fallbackDiags := base.requiredMethods(ctx, target.Fallback, targetName+".fallback")
		diags = append(diags, fallbackDiags...)
		methods = append(methods, fallback...)
	}

	return methods, diags
}
