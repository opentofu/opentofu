package encryption

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ValidateAllCachedInstances validates the configuration of all known encryptionflow.FlowBuilder instances.
//
// Call this after tofu is done with the configuration phase to get early errors as soon as all configurations
// are known.
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

	cache.mutex.RLock()
	defer cache.mutex.RUnlock()

	for configKey, instance := range cache.instances {
		if _, err := instance.Build(); err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Invalid state encryption configuration for configuration key %s", configKey),
				err.Error(),
			))
		}
	}

	return diags
}
