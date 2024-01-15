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
//
// Obviously, this will only work if the cache is enabled, otherwise this emits a warning.
func ValidateAllCachedInstances() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	if cache == nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"no encryption instance cache available, cannot validate configurations",
			"this warning may be an indication of a bug. ValidateAllCachedInstances() was called, but the cache is not enabled",
		))
		return diags
	}

	for configKey, instance := range cache.instances {
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
