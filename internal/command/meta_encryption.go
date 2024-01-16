package command

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/states/encryption"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"os"
)

// SetupEncryption initializes the encryption instance cache and validates the
// environment variables used to configure state and plan encryption.
//
// A side effect of this method is the creation of the encryption singleton.
func (m *Meta) SetupEncryption() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	singleton := encryption.GetSingleton()

	encryptionConfigs, err := encryptionconfig.ConfigurationFromEnv(os.Getenv(encryptionconfig.ConfigEnvName))
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf("Error parsing environment variable %s for state encryption", encryptionconfig.ConfigEnvName),
			err.Error(),
		))
	}

	decryptionFallbackConfigs, err := encryptionconfig.ConfigurationFromEnv(os.Getenv(encryptionconfig.FallbackConfigEnvName))
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf("Error parsing environment variable %s for state encryption", encryptionconfig.FallbackConfigEnvName),
			err.Error(),
		))
	}

	if len(diags) > 0 {
		return diags
	}

	err = singleton.ApplyEnvConfigurations(encryptionConfigs, decryptionFallbackConfigs)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf("Error parsing environment variable %s for state encryption", encryptionconfig.FallbackConfigEnvName),
			err.Error(),
		))
	}

	return diags
}
