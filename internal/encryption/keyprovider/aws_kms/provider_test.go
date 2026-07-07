// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"os"
	"testing"
)

func getKey(t *testing.T) string {
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_KMS_TEST") == "" {
		return ""
	}
	return os.Getenv("TF_AWS_KMS_KEY_ID")
}

func TestKMSProvider_Simple(t *testing.T) {
	testKeyId := getKey(t)

	var captured capturedKMSCalls

	if testKeyId == "" {
		testKeyId = "alias/my-mock-key"
		captured = injectCapturingMock(t, testKeyId)
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "accesskey")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secretkey")
	}

	encryptionContext := map[string]string{"test": "test"}

	// Constructs a aws kms key provider config that accepts the key id
	providerConfig := Config{
		KMSKeyID:            testKeyId,
		KeySpec:             "AES_256",
		SkipCredsValidation: captured.GenKeyContext != nil, // only skip validation when mocking
		EncryptionContext:   encryptionContext,
	}

	// Now that we have the config, we can build the provider
	provider, metaIn, err := providerConfig.Build()
	if err != nil {
		t.Fatalf("Error building provider: %s", err)
	}

	// Now we can test the provider
	output, meta, err := provider.Provide(metaIn)
	if err != nil {
		t.Fatalf("Error providing keys: %s", err)
	}

	if len(output.EncryptionKey) == 0 {
		t.Fatalf("No encryption key provided")
	}

	if len(output.DecryptionKey) != 0 {
		t.Fatalf("Decryption key provided and should not be")
	}

	if len(meta.(*keyMeta).CiphertextBlob) == 0 {
		t.Fatalf("No ciphertext blob provided")
	}

	if meta.(*keyMeta).EncryptionContext["test"] != "test" {
		t.Fatal("Expected encryption context to be stored in meta")
	}

	if captured.GenKeyContext != nil && (*captured.GenKeyContext)["test"] != "test" {
		t.Fatalf("Expected encryption context to be passed to GenerateDataKey")
	}

	t.Log("Continue to meta -> decryption key")

	// Now that we have a encryption key and it's meta, let's get the decryption key
	output, meta, err = provider.Provide(meta)
	if err != nil {
		t.Fatalf("Error providing keys: %s", err)
	}

	if len(output.EncryptionKey) == 0 {
		t.Fatalf("No encryption key provided")
	}

	if len(output.DecryptionKey) == 0 {
		t.Fatalf("No decryption key provided")
	}

	if len(meta.(*keyMeta).CiphertextBlob) == 0 {
		t.Fatalf("No ciphertext blob provided")
	}

	if captured.DecryptContext != nil && (*captured.DecryptContext)["test"] != "test" {
		t.Fatalf("Expected decryption context to be passed to Decrypt")
	}
}

func TestKMSProvider_EncryptionContextRotation(t *testing.T) {
	testKeyId := getKey(t)

	var captured capturedKMSCalls

	if testKeyId == "" {
		testKeyId = "alias/my-mock-key"
		captured = injectCapturingMock(t, testKeyId)
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "accesskey")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secretkey")
	}

	skipCreds := captured.GenKeyContext != nil
	oldContext := map[string]string{"env": "dev"}
	newContext := map[string]string{"env": "prod"}

	oldProvider, metaIn, err := (Config{
		KMSKeyID:            testKeyId,
		KeySpec:             "AES_256",
		SkipCredsValidation: skipCreds,
		EncryptionContext:   oldContext,
	}).Build()
	if err != nil {
		t.Fatalf("Error building provider: %s", err)
	}

	_, oldMeta, err := oldProvider.Provide(metaIn)
	if err != nil {
		t.Fatalf("Error providing keys with old context: %s", err)
	}

	if oldMeta.(*keyMeta).EncryptionContext["env"] != "dev" {
		t.Fatal("Expected old encryption context to be stored in meta")
	}

	newProvider, _, err := (Config{
		KMSKeyID:            testKeyId,
		KeySpec:             "AES_256",
		SkipCredsValidation: skipCreds,
		EncryptionContext:   newContext,
	}).Build()
	if err != nil {
		t.Fatalf("Error building provider: %s", err)
	}

	output, newMeta, err := newProvider.Provide(oldMeta)
	if err != nil {
		t.Fatalf("Error decrypting with rotated config: %s", err)
	}

	if len(output.DecryptionKey) == 0 {
		t.Fatal("No decryption key provided after rotation")
	}

	if captured.DecryptContext != nil && (*captured.DecryptContext)["env"] != "dev" {
		t.Fatal("Decrypt should use context from meta, not current config")
	}

	if newMeta.(*keyMeta).EncryptionContext["env"] != "prod" {
		t.Fatal("Expected new encryption context to be stored in meta")
	}

	if captured.GenKeyContext != nil && (*captured.GenKeyContext)["env"] != "prod" {
		t.Fatal("GenerateDataKey should use current config context")
	}
}
