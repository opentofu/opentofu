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
	if testKeyId == "" {
		testKeyId = "alias/my-mock-key"
		injectDefaultMock()

		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "accesskey")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secretkey")
	}

	// Constructs a aws kms key provider config that accepts the key id
	providerConfig := Config{
		KMSKeyID: testKeyId,
		KeySpec:  "AES_256",

		SkipCredsValidation: true, // Required for mocking
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

	t.Log("Continue to meta -> decryption key")

	// Now that we have a encyption key and it's meta, let's get the decryption key
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
}
