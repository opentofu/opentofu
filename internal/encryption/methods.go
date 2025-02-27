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
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"
)

// setupMethod sets up a single method for encryption. It returns a list of diagnostics if the method is invalid.
func setupMethod(enc *config.EncryptionConfig, cfg config.MethodConfig, meta keyProviderMetadata, reg registry.Registry, staticEval *configs.StaticEvaluator) (method.Method, hcl.Diagnostics) {
	// Lookup the definition of the encryption method from the registry
	encryptionMethod, err := reg.GetMethodDescriptor(method.ID(cfg.Type))
	if err != nil {

		// Handle if the method was not found
		var notFoundError *registry.MethodNotFoundError
		if errors.Is(err, notFoundError) {
			return nil, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Unknown encryption method type",
				Detail:   fmt.Sprintf("Can not find %q", cfg.Type),
			}}
		}

		// Or, we don't know the error type, so we'll just return it as a generic error
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Error fetching encryption method %q", cfg.Type),
			Detail:   err.Error(),
		}}
	}

	methodConfig := encryptionMethod.ConfigStruct()

	deps, diags := gohcl.VariablesInBody(cfg.Body, methodConfig)
	if diags.HasErrors() {
		return nil, diags
	}

	kpConfigs, refs, kpDiags := filterKeyProviderReferences(enc, deps)
	diags = diags.Extend(kpDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	hclCtx, kpDiags := setupKeyProviders(enc, kpConfigs, meta, reg, staticEval)
	diags = diags.Extend(kpDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	hclCtx, evalDiags := staticEval.EvalContextWithParent(hclCtx, configs.StaticIdentifier{
		Module:    addrs.RootModule,
		Subject:   fmt.Sprintf("encryption.method.%s.%s", cfg.Type, cfg.Name),
		DeclRange: enc.DeclRange,
	}, refs)
	diags = diags.Extend(evalDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	methodDiags := gohcl.DecodeBody(cfg.Body, hclCtx, methodConfig)
	diags = diags.Extend(methodDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	m, err := methodConfig.Build()
	if err != nil {
		// TODO this error handling could use some work
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption method configuration failed",
			Detail:   err.Error(),
		})
	}

	return m, diags
}
