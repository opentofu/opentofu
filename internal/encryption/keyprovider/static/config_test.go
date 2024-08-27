// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static_test

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/gohcl"

	config2 "github.com/terramate-io/opentofulib/internal/encryption/config"
	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider/static"
)

var exampleConfig = `key_provider "static" "foo" {
  key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
}
`

// This example is a bare-bones configuration for a static key provider.
// It is mainly intended to demonstrate how you can use parse configuration
// and construct a static key provider from in.
// And is not intended to be used as a real-world example.
func ExampleConfig() {
	staticConfig := static.New().ConfigStruct()

	// Parse the config:
	parsedConfig, diags := config2.LoadConfigFromString("config.hcl", exampleConfig)
	if diags.HasErrors() {
		panic(diags)
	}

	if len(parsedConfig.KeyProviderConfigs) != 1 {
		panic("Expected 1 key provider")
	}
	// Grab the KeyProvider from the parsed config:
	keyProvider := parsedConfig.KeyProviderConfigs[0]

	// assert the Type is "static" and the Name is "foo"
	if keyProvider.Type != "static" {
		panic("Expected key provider type to be 'static'")
	}
	if keyProvider.Name != "foo" {
		panic("Expected key provider name to be 'foo'")
	}

	// Use gohcl to parse the hcl block from parsedConfig into the static configuration struct
	// This is not the intended path, and it should be handled by the implementation of the Encryption
	// interface.
	//
	// This is just an example of how to use the static configuration struct, and this is how testing
	// may be carried out.
	if err := gohcl.DecodeBody(parsedConfig.KeyProviderConfigs[0].Body, nil, staticConfig); err != nil {
		panic(err)
	}

	// Cast the static configuration struct to a static.Config so that we can assert against the key
	// value
	s := staticConfig.(*static.Config)

	fmt.Printf("%s\n", s.Key)
	// Output: 6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169
}
