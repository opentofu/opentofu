package tofu

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func ValidateEnabledReferences(ctx context.Context, config *configs.Config, addrs []addrs.ConfigResource, evalCtx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	for _, addr := range addrs {
		resourceConfig := config.Module.ResourceByAddr(addr.Resource)

		enabled, evalDiags := evaluateEnabledExpression(ctx, resourceConfig.Enabled, evalCtx)
		diags = diags.Append(evalDiags)
		if diags.HasErrors() {
			return diags
		}

		if !enabled {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Reference to disabled resource`,
				Detail:   fmt.Sprintf(`A resource %s is not enabled for usage.`, addr),
				Subject:  resourceConfig.Enabled.Range().Ptr(),
			})
		}
	}

	return diags
}
