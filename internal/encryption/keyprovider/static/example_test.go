package static_test

import (
	"bytes"
	"fmt"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

func Example() {
	config := `key_provider "static" "foo" {
  key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
}

method "aes_gcm" "foo" {
  key = key_provider.static.foo
}`

	// Set up the registry with the correct key provider and method:
	registry := lockingencryptionregistry.New()
	if err := registry.RegisterKeyProvider(static.New()); err != nil {
		panic(err)
	}
	if err := registry.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}

	// Parse the config:
	parsedConfig, diags := encryption.LoadConfigFromString("config.hcl", config)
	if diags.HasErrors() {
		panic(diags)
	}

	// Set up the encryption:
	enc, diags := encryption.New(registry, parsedConfig)
	if diags.HasErrors() {
		panic(diags)
	}

	// Encrypt:
	planFileEncryption := enc.PlanFile()
	// TODO this is not a valid ZIP file.
	sourceData := []byte("Hello world!")
	encryptedPlan, err := planFileEncryption.EncryptPlan(sourceData)
	if err != nil {
		panic(err)
	}
	if bytes.Equal(encryptedPlan, sourceData) {
		panic("The data has not been encrypted!")
	}

	// Decrypt:
	decryptedPlan, err := planFileEncryption.DecryptPlan(encryptedPlan)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", decryptedPlan)
	// Output: Hello world!
}
