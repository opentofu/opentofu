package encryption

import (
	"fmt"
	"os"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Note: at the moment, we are still using a mockup Flow, which does not actually encrypt anything.

func ExampleGetSingleton() {
	defer ClearSingleton() // only needed in tests, not part of the example

	// this is an example how the rest of tofu's codebase should interact with this package
	// (case of our own remote state)

	// before starting tofu, environment variables TF_STATE_ENCRYPTION and TF_STATE_DECRYPTION_FALLBACK
	// are set by the user executing tofu. It is ok to not set either of them.

	// Here is an example value that affects our own remote state (providing a passphrase):
	TF_STATE_ENCRYPTION := fmt.Sprintf(`{
		"%s": {
			"key_provider": {
				"config": {
					"passphrase": "the current passphrase"
				}
			}
		}
	}`, encryptionconfig.KeyBackend)
	TF_STATE_DECRYPTION_FALLBACK := fmt.Sprintf(`{
		"%s": {
			"key_provider": {
				"config": {
					"passphrase": "the previous passphrase for key rotation"
				}
			}
		}
	}`, encryptionconfig.KeyBackend)

	// For the sake of this example, we set them here, but YOU WOULD NOT DO THAT IN YOUR CODE.
	_ = os.Setenv(encryptionconfig.ConfigEnvName, TF_STATE_ENCRYPTION)
	defer os.Unsetenv(encryptionconfig.ConfigEnvName)

	_ = os.Setenv(encryptionconfig.FallbackConfigEnvName, TF_STATE_DECRYPTION_FALLBACK)
	defer os.Unsetenv(encryptionconfig.FallbackConfigEnvName)

	// -----------------------------------------
	// during tofu startup (in the meta command)
	// -----------------------------------------

	{
		// code block to isolate variables in sections of this example that would normally
		// be in different places in the tofu code

		singleton := GetSingleton()

		encryptionConfigsFromEnv, err := encryptionconfig.ConfigurationFromEnv(os.Getenv(encryptionconfig.ConfigEnvName))
		if err != nil {
			fmt.Println("environment variables are syntactically invalid or have unexpected fields - abort and print the error", err.Error())
			return
		}
		decryptionFallbackConfiggFromEnv, err := encryptionconfig.ConfigurationFromEnv(os.Getenv(encryptionconfig.FallbackConfigEnvName))
		if err != nil {
			fmt.Println("environment variables are syntactically invalid or have unexpected fields - abort and print the error", err.Error())
			return
		}

		err = singleton.ApplyEnvConfigurations(encryptionConfigsFromEnv, decryptionFallbackConfiggFromEnv)
		if err != nil {
			fmt.Println("environment variables have invalid keys - abort and print the error", err.Error())
			return
		}
	}

	// -----------------------------------------------------------------------------------
	// while parsing terraform {...} block (assuming a remote state backend is configured)
	// -----------------------------------------------------------------------------------

	{
		// code block to isolate variables in sections of this example that would normally
		// be in different places in the tofu code

		// this is what you might parse from terraform.state_encryption (if set)
		config := encryptionconfig.Config{
			KeyProvider: encryptionconfig.KeyProviderConfig{
				Name: "passphrase",
			},
			Method: encryptionconfig.MethodConfig{
				Name: "full",
			},
			Enforced: true,
		}

		// this is what you might parse from terraform.state_decryption_fallback (if set)
		fallbackConfig := encryptionconfig.Config{
			KeyProvider: encryptionconfig.KeyProviderConfig{
				Name: "passphrase",
			},
			Method: encryptionconfig.MethodConfig{
				Name: "full",
			},
			Enforced: true,
		}
		// yes, it's the same configuration in this example. The difference might be a different passphrase
		// which should come from the TF_STATE_DECRYPTION_FALLBACK environment variable.

		singleton := GetSingleton()

		// only call this if terraform.state_encryption block was present
		err := singleton.ApplyHCLEncryptionConfiguration(encryptionconfig.KeyBackend, config)
		if err != nil {
			fmt.Println("errors here mean the supplied configuration was invalid (not all problems can be detected at this time)", err.Error())
			return
		}

		// only call this if terraform.state_decryption_fallback block was present
		err = singleton.ApplyHCLDecryptionFallbackConfiguration(encryptionconfig.KeyBackend, fallbackConfig)
		if err != nil {
			fmt.Println("errors here mean the supplied fallback configuration was invalid (not all problems can be detected at this time)", err.Error())
			return
		}
	}

	// ---------------------------------------------
	// at the end of tofu configuration / parse time
	// ---------------------------------------------

	{
		// code block to isolate variables in sections of this example that would normally
		// be in different places in the tofu code

		singleton := GetSingleton()

		diags := singleton.Validate()
		// errors here mean that configurations for state encryption was invalid
		//
		// At this time, practically all static configuration errors can be detected.
		// Of course, key providers that access external systems could still fail later.
		for _, d := range diags {
			if d.Severity() == tfdiags.Error {
				fmt.Println("error in configuration: ", d.Description())
				return
			}
			// Note: warnings can also occur, and should be printed, but the run can continue.
			fmt.Println("warning: ", d.Description())
		}
	}

	// -----------------------------------------------------------------
	// while working with remote state (internal/states/remote/state.go)
	// -----------------------------------------------------------------

	{
		// code block to isolate variables in sections of this example that would normally
		// be in different places in the tofu code

		singleton := GetSingleton()

		flow, err := singleton.RemoteState()
		if err != nil {
			// errors here should not normally happen after Validate() was successful, but if they do, fail state read or write
			fmt.Println("error building encryption flow, most likely invalid configuration in terraform block, "+
				"or merged configuration between environment variable and terraform block was incomplete or invalid", err.Error())
			return
		}

		// during state write:

		stateToEncrypt := []byte(`{"version":"4"}`) // omitted a lot more fields in stateV4

		encrypted, err := flow.EncryptState(stateToEncrypt)
		if err != nil {
			fmt.Println("error writing remote state", err.Error())
			return
		}

		// during state read:

		// for the sake of the example we simply decrypt what we just encrypted

		decrypted, err := flow.DecryptState(encrypted)
		if err != nil {
			fmt.Println("error reading remote state", err.Error())
			return
		}

		// success

		fmt.Println(string(decrypted))
	}

	// Output: {"version":"4"}
}
