// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/external"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/external/testprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

func TestChaining(t *testing.T) {
	testProviderBinaryPath := testprovider.Go(t)

	reg := lockingencryptionregistry.New()
	if err := reg.RegisterKeyProvider(external.New()); err != nil {
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
	testProviderBinaryPath = append(testProviderBinaryPath, "--hello-world")
	commandParts := make([]string, len(testProviderBinaryPath))
	for i, cmdPart := range testProviderBinaryPath {
		commandParts[i] = "\"" + cmdPart + "\""
	}

	configData := fmt.Sprintf(`key_provider "external" "test" {
	command = [%s]
}
key_provider "pbkdf2" "passphrase" {
	chain = key_provider.external.test
}
method "aes_gcm" "example" {
	keys = key_provider.pbkdf2.passphrase
}
state {
	method = method.aes_gcm.example
}
`, strings.Join(commandParts, ", "))
	cfg, diags := config.LoadConfigFromString("Test Config Source", configData)

	if diags.HasErrors() {
		t.Fatalf("%v", diags)
	}

	staticEval := configs.NewStaticEvaluator(nil, configs.RootModuleCallForTesting())

	enc, diags := encryption.New(t.Context(), reg, cfg, staticEval)
	if diags.HasErrors() {
		t.Fatalf("%v", diags)
	}

	stateEncryption := enc.State()

	fakeState := "{}"
	encryptedState, err := stateEncryption.EncryptState([]byte(fakeState))
	if err != nil {
		t.Fatalf("%v", err)
	}
	decryptedState, _, err := stateEncryption.DecryptState(encryptedState)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(decryptedState) != fakeState {
		t.Fatalf("Mismatching decrypted state: %s", decryptedState)
	}
}
