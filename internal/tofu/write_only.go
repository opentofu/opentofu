package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// validateWriteOnlyAttributes checks that the write-only attributes are not returned back with an actual value.
func validateWriteOnlyAttributes(schema *configschema.Block, v cty.Value, resCfg *configs.Resource) (diags tfdiags.Diagnostics) {
	if !schema.ContainsWriteOnly() {
		return diags
	}
	var subj *hcl.Range
	if resCfg != nil {
		subj = resCfg.DeclRange.Ptr()
	}
	paths := schema.WriteOnlyPaths(v, nil)
	for _, path := range paths {
		pathAsString := tfdiags.FormatCtyPath(path)

		pathVal, err := path.Apply(v)
		if err != nil {
			log.Printf("[WARN] Error when tried to get the path (%s) value from the given object: %s", pathAsString, err)
			continue
		}
		if pathVal.IsNull() {
			continue
		}
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider response",
			Detail:   fmt.Sprintf("Write-only attribute %q returned with a non-nil value. This is an issue in the provider SDK.", pathAsString),
			Subject:  subj,
		})
	}
	return diags
}
