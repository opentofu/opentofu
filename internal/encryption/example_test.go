// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption_test

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

var (
	ConfigA = `
state {
	enforced = true
}
`
	ConfigB = `
key_provider "static" "basic" {
	key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
}
method "aes_gcm" "example" {
	keys = key_provider.static.basic
}
state {
	method = method.aes_gcm.example
}
`
)

// This example demonstrates how to use the encryption package to encrypt and decrypt data.
func Example() {
	// Construct a new registry
	// the registry is where we store the key providers and methods
	reg := lockingencryptionregistry.New()
	if err := reg.RegisterKeyProvider(static.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}

	// Load the 2 different configurations
	cfgA, diags := config.LoadConfigFromString("Test Source A", ConfigA)
	handleDiags(diags)

	cfgB, diags := config.LoadConfigFromString("Test Source B", ConfigB)
	handleDiags(diags)

	// Merge the configurations
	cfg := config.MergeConfigs(cfgA, cfgB)

	// Construct static evaluator to pass additional values into encryption configuration.
	staticEval := configs.NewStaticEvaluator(nil, configs.RootModuleCallForTesting())

	// Construct the encryption object
	enc, diags := encryption.New(reg, cfg, staticEval)
	handleDiags(diags)

	sfe := enc.State()

	// Encrypt the data, for this example we will be using the string `{"serial": 42, "lineage": "magic"}`,
	// but in a real world scenario this would be the plan file.
	sourceData := []byte(`{"serial": 42, "lineage": "magic"}`)
	encrypted, err := sfe.EncryptState(sourceData)
	if err != nil {
		panic(err)
	}

	if string(encrypted) == `{"serial": 42, "lineage": "magic"}` {
		panic("The data has not been encrypted!")
	}

	// Decrypt
	decryptedState, err := sfe.DecryptState(encrypted)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", decryptedState)
	// Output: {"serial": 42, "lineage": "magic"}
}

func handleDiags(diags hcl.Diagnostics) {
	for _, d := range diags {
		println(d.Error())
	}
	if diags.HasErrors() {
		panic(diags.Error())
	}
}
