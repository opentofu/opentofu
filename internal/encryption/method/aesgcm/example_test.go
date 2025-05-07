// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm_test

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
)

func Example() {
	descriptor := aesgcm.New()

	// Get the config struct. You can fill it manually by type-asserting it to aesgcm.Config, but you could also use
	// it as JSON.
	config := descriptor.ConfigStruct()

	if err := json.Unmarshal(
		// Set up a randomly generated 32-byte key. In JSON, you can base64-encode the value.
		[]byte(`{
    "keys": {
		"encryption_key": "Y29veTRhaXZ1NWFpeW9vMWlhMG9vR29vVGFlM1BhaTQ=",
		"decryption_key": "Y29veTRhaXZ1NWFpeW9vMWlhMG9vR29vVGFlM1BhaTQ="
	}
}`), &config); err != nil {
		panic(err)
	}

	method, err := config.Build()
	if err != nil {
		panic(err)
	}

	// Encrypt some data:
	encrypted, err := method.Encrypt([]byte("Hello world!"))
	if err != nil {
		panic(err)
	}

	// Now decrypt it:
	decrypted, err := method.Decrypt(encrypted)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}

func Example_config() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := aesgcm.New()

	// Obtain a modifiable, buildable config. Alternatively, you can also use ConfigStruct() method to obtain a
	// struct you can fill with HCL or JSON tags.
	config := descriptor.TypedConfig()

	// Set up an encryption key:
	config.Keys = keyprovider.Output{
		EncryptionKey: []byte("AiphoogheuwohShal8Aefohy7ooLeeyu"),
		DecryptionKey: []byte("AiphoogheuwohShal8Aefohy7ooLeeyu"),
	}

	// Now you can build a method:
	method, err := config.Build()
	if err != nil {
		panic(err)
	}

	// Encrypt something:
	encrypted, err := method.Encrypt([]byte("Hello world!"))
	if err != nil {
		panic(err)
	}

	// Decrypt it:
	decrypted, err := method.Decrypt(encrypted)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}

func Example_config_json() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := aesgcm.New()

	// Get an untyped config struct you can use for JSON unmarshalling:
	config := descriptor.ConfigStruct()

	// Unmarshal JSON into the config struct:
	if err := json.Unmarshal(
		// Set up a randomly generated 32-byte key. In JSON, you can base64-encode the value.
		[]byte(`{
    "keys": {
		"encryption_key": "Y29veTRhaXZ1NWFpeW9vMWlhMG9vR29vVGFlM1BhaTQ=",
		"decryption_key": "Y29veTRhaXZ1NWFpeW9vMWlhMG9vR29vVGFlM1BhaTQ="
	}
}`), &config); err != nil {
		panic(err)
	}

	// Now you can build a method:
	method, err := config.Build()
	if err != nil {
		panic(err)
	}

	// Encrypt something:
	encrypted, err := method.Encrypt([]byte("Hello world!"))
	if err != nil {
		panic(err)
	}

	// Decrypt it:
	decrypted, err := method.Decrypt(encrypted)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}

func Example_config_hcl() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := aesgcm.New()

	// Get an untyped config struct you can use for HCL unmarshalling:
	config := descriptor.ConfigStruct()

	// Unmarshal HCL code into the config struct. The input must be a list of bytes, so in a real world scenario
	// you may want to put in a hex-decoding function:
	rawHCLInput := `keys = {
	encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32],
	decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32]
}`
	file, diags := hclsyntax.ParseConfig(
		[]byte(rawHCLInput),
		"example.hcl",
		hcl.Pos{Byte: 0, Line: 1, Column: 1},
	)
	if diags.HasErrors() {
		panic(diags)
	}
	if diags := gohcl.DecodeBody(file.Body, nil, config); diags.HasErrors() {
		panic(diags)
	}

	// Now you can build a method:
	method, err := config.Build()
	if err != nil {
		panic(err)
	}

	// Encrypt something:
	encrypted, err := method.Encrypt([]byte("Hello world!"))
	if err != nil {
		panic(err)
	}

	// Decrypt it:
	decrypted, err := method.Decrypt(encrypted)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}
