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
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
)

func methodConfigsFromTarget(cfg *config.EncryptionConfig, target *config.TargetConfig, targetName string, enforced bool) ([]config.MethodConfig, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	hclCtx, methodLookup, methodDiags := methodContextAndConfigs(cfg)
	diags = diags.Extend(methodDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	var methods []config.MethodConfig

	for target != nil {
		// gohcl has some weirdness around attributes that are not provided, but are hcl.Expressions
		// They will set the attribute field to a static null expression
		// https://github.com/hashicorp/hcl/blob/main/gohcl/decode.go#L112-L118

		// Descriptor referenced by this target
		var methodIdent *string
		decodeDiags := gohcl.DecodeExpression(target.Method, hclCtx, &methodIdent)
		diags = append(diags, decodeDiags...)

		// Only attempt to fetch the method if the decoding was successful
		if !decodeDiags.HasErrors() {
			if methodIdent != nil {
				if method, ok := methodLookup[method.Addr(*methodIdent)]; ok {
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
		targetName = targetName + ".fallback"
		target = target.Fallback
	}

	if enforced {
		for _, m := range methods {
			if unencrypted.IsConfig(m) {
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
