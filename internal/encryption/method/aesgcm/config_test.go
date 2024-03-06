package aesgcm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

func Example_config() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := New()

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
	descriptor := New()

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
	descriptor := New()

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

func TestConfig_Build(t *testing.T) {
	descriptor := New()
	var testCases = []struct {
		name      string
		config    *Config
		errorType any
		expected  aesgcm
	}{
		{
			name: "key-32-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				nil,
			},
		},
		{
			name: "key-24-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileefe"),
				[]byte("bohwu9zoo7Zool5olaileefd"),
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5olaileefe"),
				[]byte("bohwu9zoo7Zool5olaileefd"),
				nil,
			},
		},
		{
			name: "key-16-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5e"),
				[]byte("bohwu9zoo7Zool5d"),
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5e"),
				[]byte("bohwu9zoo7Zool5d"),
				nil,
			},
		},
		{
			name:      "no-key",
			config:    descriptor.TypedConfig(),
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "encryption-key-15-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Ze15"),
				[]byte("bohwu9zoo7Zod16"),
				nil,
			}),
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "decryption-key-15-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zooe16"),
				[]byte("bohwu9zoo7Zod15"),
				nil,
			}),
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "decryption-key-fallback",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zooe16"),
				nil,
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zooe16"),
				[]byte("bohwu9zoo7Zooe16"),
				nil,
			},
		},
		{
			name: "aad",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				nil,
			}).WithAAD([]byte("foobar")),
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				[]byte("foobar"),
			},
			errorType: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			built, err := tc.config.Build()
			if tc.errorType == nil {
				if err != nil {
					t.Fatalf("Unexpected error returned: %v", err)
				}

				built := built.(*aesgcm)

				if !bytes.Equal(tc.expected.encryptionKey, built.encryptionKey) {
					t.Fatalf("Incorrect encryption key built: %v != %v", tc.expected.encryptionKey, built.encryptionKey)
				}
				if !bytes.Equal(tc.expected.decryptionKey, built.decryptionKey) {
					t.Fatalf("Incorrect decryption key built: %v != %v", tc.expected.decryptionKey, built.decryptionKey)
				}
				if !bytes.Equal(tc.expected.aad, built.aad) {
					t.Fatalf("Incorrect aad built: %v != %v", tc.expected.aad, built.aad)
				}

			} else if tc.errorType != nil {
				if err == nil {
					t.Fatal("Expected error, none received")
				}
				if !errors.As(err, &tc.errorType) {
					t.Fatalf("Incorrect error type received: %T", err)
				}
				t.Logf("Correct error of type %T received: %v", err, err)
			}

		})
	}
}
