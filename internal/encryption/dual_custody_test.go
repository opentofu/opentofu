// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"testing"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/xor"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

func TestDualCustody(t *testing.T) {
	// Note: the XOR provider is not available in final OpenTofu builds because its security constraints have not
	// been properly evaluated. The code below doesn't work in OpenTofu and is for tests only.
	sourceConfig := `key_provider "pbkdf2" "base1" {
			passphrase = "Hello world! 123"
		}
		key_provider "pbkdf2" "base2" {
			passphrase = "OpenTofu has Encryption"
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

	enc, diags := New(reg, parsedSourceConfig, staticEval)
	if diags.HasErrors() {
		t.Fatalf("%v", diags.Error())
	}

	sfe := enc.State()
	testData := []byte(`{"serial": 42, "lineage": "magic"}`)
	encryptedState, err := sfe.EncryptState(testData)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(encryptedState) == string(testData) {
		t.Fatalf("The state has not been encrypted.")
	}
	decryptedState, _, err := sfe.DecryptState(encryptedState)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(decryptedState) != string(testData) {
		t.Fatalf("Incorrect decrypted state: %s", decryptedState)
	}
}
