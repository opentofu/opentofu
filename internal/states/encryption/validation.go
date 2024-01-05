package encryption

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ValidateAllCachedInstances validates the configuration of all known flow.Flow instances.
//
// Call this after tofu is done with the configuration phase and all instances are configured
// to get early errors.
//
// This avoids that errors will occur when first reading state, with possibly misleading
// error message titles such as "could not acquire state lock". The detailed error
// messages will still be printed, but further down, confusing the user.
func ValidateAllCachedInstances() tfdiags.Diagnostics {
	if !environmentParsedSuccessfully {
		panic("call to ValidateAllCachedInstances() before ParseEnvironmentVariables(). This is a bug.")
	}

	var diags tfdiags.Diagnostics

	for configKey, instance := range instanceCache {
		if err := instance.MergeAndValidateConfigurations(); err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Invalid state encryption configuration for configuration key %s", configKey),
				err.Error(),
			))
		}
	}

	return diags
}
