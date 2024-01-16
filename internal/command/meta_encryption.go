package command

import (
	"github.com/opentofu/opentofu/internal/states/encryption"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"log"
)

// SetupEncryption initializes the encryption instance cache and validates the
// environment variables used to configure state and plan encryption.
//
// A side-effect of this method is the creation of the encryption instance cache.
func (m *Meta) SetupEncryption() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	encryption.EnableSingletonCaching() // idempotent
	err := encryption.ParseEnvironmentVariables()
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error parsing environment variables for state encryption",
			err.Error(),
		))
		return diags
	}

	log.Print("[TRACE] Meta.SetupEncryption: state encryption environment is valid")
	return nil
}
