package aesgcm_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
)

func Example_config() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := aesgcm.New()

	// Obtain a modifiable, buildable config. Alternatively, you can also use ConfigStruct() method to obtain a
	// struct you can fill with HCL or JSON tags.
	config := descriptor.TypedConfig()

	// Set up an encryption key:
	config.WithKeys(keyprovider.Output{
		[]byte("AiphoogheuwohShal8Aefohy7ooLeeyu"),
		[]byte("AiphoogheuwohShal8Aefohy7ooLeeyu"),
		nil,
	})

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

type testCase struct {
	config    *aesgcm.Config
	errorType any
}

func TestConfigValidation(t *testing.T) {
	descriptor := aesgcm.New()
	var testCases = map[string]testCase{
		"key-32-bytes": {
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathi"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathi"),
				nil,
			}),
			errorType: nil,
		},
		"key-24-bytes": {
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1"),
				[]byte("bohwu9zoo7Zool5olaileef1"),
				nil,
			}),
			errorType: nil,
		},
		"key-16-bytes": {
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5o"),
				[]byte("bohwu9zoo7Zool5o"),
				nil,
			}),
			errorType: nil,
		},
		"no-key": {
			config:    descriptor.TypedConfig(),
			errorType: &method.ErrInvalidConfiguration{},
		},
		"key-15-bytes": {
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5"),
				[]byte("bohwu9zoo7Zool5"),
				nil,
			}),
			errorType: &method.ErrInvalidConfiguration{},
		},
		"aad": {
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathi"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathi"),
				nil,
			}).WithAAD([]byte("foobar")),
			errorType: nil,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := tc.config.Build()
			if tc.errorType == nil && err != nil {
				t.Fatalf("Unexpected error returned: %v", err)
			} else if tc.errorType != nil {
				if err == nil {
					t.Fatalf("Expected error, none received")
				}
				if !errors.As(err, &tc.errorType) {
					t.Fatalf("Incorrect error type received: %T", err)
				}
				t.Logf("Correct error of type %T received: %v", err, err)
			}
		})
	}
}
