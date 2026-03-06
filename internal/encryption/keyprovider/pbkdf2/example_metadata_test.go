// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2_test

import (
	"bytes"
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"
)

var metadataExampleConfiguration = `key_provider "pbkdf2" "foo" {
  passphrase = "correct-horse-battery-staple"
}
`

func ExampleMetadata() {
	configStruct := pbkdf2.New().ConfigStruct()

	// Parse the config:
	parsedConfig, diags := config.LoadConfigFromString("config.hcl", metadataExampleConfiguration)
	if diags.HasErrors() {
		panic(diags)
	}

	// The `pbkdf2` key provider config has its own decoding so use that instead of gohcl
	decoder := configStruct.(keyprovider.SelfDecodingConfig)
	if err := decoder.DecodeConfig(parsedConfig.KeyProviderConfigs[0].Body, nil); err != nil {
		panic(err)
	}

	// Create the actual key provider.
	keyProvider, keyMeta, err := configStruct.Build()
	if err != nil {
		panic(err)
	}

	// The first time around, let's get an encryption key:
	oldKeys, oldMeta, err := keyProvider.Provide(keyMeta)
	if err != nil {
		panic(err)
	}

	// The second time, you can pass in the metadata from the previous encryption:
	newKeys, _, err := keyProvider.Provide(oldMeta)
	if err != nil {
		panic(err)
	}

	// The old encryption and new decryption key will be the same:
	if bytes.Equal(oldKeys.EncryptionKey, newKeys.DecryptionKey) {
		fmt.Println("The keys match!")
	}
	// Output: The keys match!
}
