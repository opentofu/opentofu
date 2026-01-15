// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm_test

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/zclconf/go-cty/cty"
)

func Example_config() {
	// Obtain a modifiable, buildable config.
	config := aesgcm.Config{}

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

func Example_config_hcl() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := aesgcm.New()

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

	methodCtx := method.EvalContext{ValueForExpression: func(expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
		return expr.Value(nil)
	}}
	config, diags := descriptor.DecodeConfig(methodCtx, file.Body)
	if diags.HasErrors() {
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
