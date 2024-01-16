package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/states/encryption"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func handleStateEncryptionBlock(block *hcl.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	// here we would parse the contents of the block, but for now let's just set some config

	// this example is for the "statefile" subblock, the others would be handled in very similar fashion

	mockConfig := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Name: encryptionconfig.KeyProviderPassphrase,
			// passphrase intentionally missing, will let that come from env to try config merge
		},
		Method: encryptionconfig.EncryptionMethodConfig{
			Name: encryptionconfig.EncryptionMethodFull,
		},
	}

	instance, err := encryption.GetStatefileSingleton()
	if err != nil {
		// this shouldn't happen because we've already validated the environment variables much earlier
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Failed to construct statefile encryption instance",
			Detail:   err.Error(),
			Subject:  &block.DefRange,
		})
		return diags
	}

	if err := instance.EncryptionConfiguration(encryptionflow.ConfigurationSourceCode, mockConfig); err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Failed to add encryption configuration",
			Detail:   err.Error(),
			Subject:  &block.DefRange,
		})
	}

	return diags
}

// same for fallback block (omitted)
