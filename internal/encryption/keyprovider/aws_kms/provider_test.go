package aws_kms

import (
	"os"
	"testing"
)

// skipCheck checks if the test should be skipped or not based on environment variables
func skipCheckGetKey(t *testing.T) string {
	// check if TF_ACC and TF_KMS_TEST are unset
	// if so, skip the test
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_KMS_TEST") == "" {
		t.Log("Skipping test because TF_ACC or TF_KMS_TEST is not set")
		t.Skip()
	}
	key := os.Getenv("TF_AWS_KMS_KEY_ID")
	if key == "" {
		t.Log("Skipping test because TF_AWS_KMS_KEY_ID is not set")
		t.Skip()
	}
	return key
}

func TestKMSProvider_Simple(t *testing.T) {
	testKeyId := skipCheckGetKey(t)

	// Constructs a aws kms key provider config that accepts the key id
	providerConfig := Config{
		KMSKeyID: testKeyId,
		KeySpec:  "AES_256",
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
