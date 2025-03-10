// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
)

func methodConfigsFromTarget(cfg *config.EncryptionConfig, target *config.TargetConfig, targetName string, enforced bool) ([]config.MethodConfig, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	var methods []config.MethodConfig

	for target != nil {
		traversal, travDiags := hcl.AbsTraversalForExpr(target.Method)
		diags = diags.Extend(travDiags)
		if !travDiags.HasErrors() {
			if len(traversal) != 3 {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid encryption method identifier",
					Detail:   "Expected method of form method.<type>.<name>",
					Subject:  target.Method.Range().Ptr(),
				})
				continue
			}
			mType, okType := traversal[1].(hcl.TraverseAttr)
			mName, okName := traversal[2].(hcl.TraverseAttr)

			if traversal.RootName() != "method" || !okType || !okName {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid encryption method identifier",
					Detail:   "Expected method of form method.<type>.<name>",
					Subject:  target.Method.Range().Ptr(),
				})
			}

			foundMethod := false
			for _, method := range cfg.MethodConfigs {
				if method.Type == mType.Name && method.Name == mName.Name {
					foundMethod = true
					methods = append(methods, method)
					break
				}
			}

			if !foundMethod {
				// We can't continue if the method is not found
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Reference to undeclared encryption method",
					Detail:   fmt.Sprintf("There is no method %q %q block declared in the encryption block.", mType.Name, mName.Name),
					Subject:  target.Method.Range().Ptr(),
				})
			}
		}

		// Attempt to fetch the fallback method if it's been configured
		targetName += ".fallback"
		target = target.Fallback
	}

	if enforced {
		for _, m := range methods {
			if unencrypted.IsConfig(m) {
				return nil, append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Unencrypted method is forbidden",
					Detail:   `Unable to use unencrypted method since the enforced flag is set.`,
					Subject:  cfg.DeclRange.Ptr(),
				})
			}
		}
	}

	return methods, diags
}
