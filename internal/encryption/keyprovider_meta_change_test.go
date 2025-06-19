// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/xor"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

func TestChangingKeyProviderAddr(t *testing.T) {
	sourceConfig := `key_provider "pbkdf2" "basic" {
			encrypted_metadata_alias = "foo"
			passphrase               = "Hello world! 123"
		}
		method "aes_gcm" "example" {
			keys = key_provider.pbkdf2.basic
		}
		state {
			method = method.aes_gcm.example
		}`
	dstConfig := `key_provider "pbkdf2" "simple" {
			encrypted_metadata_alias = "foo"
			passphrase               = "Hello world! 123"
		}
		method "aes_gcm" "example" {
			keys = key_provider.pbkdf2.simple
		}
		state {
			method = method.aes_gcm.example
		}`

	reg := lockingencryptionregistry.New()
	if err := reg.RegisterKeyProvider(pbkdf2.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(unencrypted.New()); err != nil {
		panic(err)
	}

	parsedSourceConfig, diags := config.LoadConfigFromString("source", sourceConfig)
	if diags.HasErrors() {
		t.Fatalf("%v", diags.Error())
	}
	parsedDestinationConfig, diags := config.LoadConfigFromString("destination", dstConfig)
	if diags.HasErrors() {
		t.Fatalf("%v", diags.Error())
	}

	staticEval := configs.NewStaticEvaluator(nil, configs.RootModuleCallForTesting())

	enc1, diags := New(t.Context(), reg, parsedSourceConfig, staticEval)
	if diags.HasErrors() {
		t.Fatalf("%v", diags.Error())
	}
	enc2, diags := New(t.Context(), reg, parsedDestinationConfig, staticEval)
	if diags.HasErrors() {
		t.Fatalf("%v", diags.Error())
	}

	sfe1 := enc1.State()
	sfe2 := enc2.State()

	testData := []byte(`{"serial": 42, "lineage": "magic"}`)
	encryptedState, err := sfe1.EncryptState(testData)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(encryptedState) == string(testData) {
		t.Fatalf("The state has not been encrypted.")
	}
	decryptedState, _, err := sfe2.DecryptState(encryptedState)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(decryptedState) != string(testData) {
		t.Fatalf("Incorrect decrypted state: %s", decryptedState)
	}
}

func TestDuplicateKeyProvider(t *testing.T) {
	// Note: the XOR provider is not available in final OpenTofu builds because its security constraints have not
	// been properly evaluated. The code below doesn't work in OpenTofu and is for tests only.
	sourceConfig := `key_provider "pbkdf2" "base1" {
			encrypted_metadata_alias = "foo"
			passphrase               = "Hello world! 123"
		}
		key_provider "pbkdf2" "base2" {
			encrypted_metadata_alias = "foo"
			passphrase               = "OpenTofu has Encryption"
		}
		key_provider "xor" "dualcustody" {
			a = key_provider.pbkdf2.base1
			b = key_provider.pbkdf2.base2
		}
		method "aes_gcm" "example" {
			keys = key_provider.xor.dualcustody
		}
		state {
			method = method.aes_gcm.example
		}`
	reg := lockingencryptionregistry.New()
	if err := reg.RegisterKeyProvider(xor.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterKeyProvider(pbkdf2.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(unencrypted.New()); err != nil {
		panic(err)
	}

	parsedSourceConfig, diags := config.LoadConfigFromString("source", sourceConfig)
	if diags.HasErrors() {
		t.Fatalf("%v", diags.Error())
	}

	staticEval := configs.NewStaticEvaluator(nil, configs.RootModuleCallForTesting())

	_, diags = New(t.Context(), reg, parsedSourceConfig, staticEval)
	if diags.HasErrors() {
		if !strings.Contains(diags.Error(), "Duplicate metadata key") {
			t.Fatalf("No error due to duplicate metadata key: %v", diags)
		}
	} else {
		t.Fatalf("Encrypted state despite duplicate metadata key.")
	}
}
