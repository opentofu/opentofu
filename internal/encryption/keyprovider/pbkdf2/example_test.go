// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2_test

import (
	"fmt"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/mitchellh/mapstructure"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"

	"github.com/opentofu/opentofu/internal/encryption/config"
)

var configuration = `key_provider "pbkdf2" "foo" {
  passphrase = "Hello world!"
}
`

// This example is a bare-bones configuration for a static key provider.
// It is mainly intended to demonstrate how you can use parse configuration
// and construct a static key provider from in.
// And is not intended to be used as a real-world example.
func Example_decrypt() {
	// Fill in the metadata stored with the encrypted form:
	decryptionMeta := map[string]any{
		"salt":          "10ec3d3fe02ad2bee6f1f5540f8e6bbe3b8b29445cf502d27d47ad554aa8971f",
		"iterations":    600000,
		"hash_function": "sha512",
	}

	configStruct := pbkdf2.New().ConfigStruct()

	// Parse the config:
	parsedConfig, diags := config.LoadConfigFromString("config.hcl", configuration)
	if diags.HasErrors() {
		panic(diags)
	}

	// Use gohcl to parse the hcl block from parsedConfig into the static configuration struct:
	if err := gohcl.DecodeBody(
		parsedConfig.KeyProviderConfigs[0].Body,
		nil,
		configStruct,
	); err != nil {
		panic(err)
	}

	// Map the encrypted metadata into the config structure:
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		// We want all metadata fields to be consumed:
		ErrorUnused: true,
		// Fill the results in this struct:
		Result: &configStruct,
		// Use the "meta" tag:
		TagName: "meta",
		// Ignore fields not tagged with "meta":
		IgnoreUntaggedFields: true,
	})
	if err != nil {
		panic(err)
	}
	if err := decoder.Decode(decryptionMeta); err != nil {
		panic(err)
	}

	// Create the actual key provider.
	keyProvider, err := configStruct.Build()
	if err != nil {
		panic(err)
	}

	// Get decryption key from the provider.
	_, decryptionKey, _, err := keyProvider.Provide()
	if err != nil {
		panic(err)
	}

	fmt.Printf("%x", decryptionKey)
	// Output: 7919af5a183ed2eb8bef7ab7555f5e9e3381afb91dbbc315be438a79de5c5fbd
}
