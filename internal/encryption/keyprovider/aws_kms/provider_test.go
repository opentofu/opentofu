package aws_kms

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
)

// skipCheck checks if the test should be skipped or not based on environment variables
func skipCheck(t *testing.T) {
	// check if TF_ACC and TF_KMS_TEST are unset
	// if so, skip the test
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_KMS_TEST") == "" {
		t.Log("Skipping test because TF_ACC or TF_KMS_TEST is not set")
		t.Skip()
	}
}

const testKeyPrefix = "tf-acc-test-kms-key"
const testAliasPrefix = "alias/my-key-alias"

func TestKMSProvider_Simple(t *testing.T) {
	skipCheck(t)
	ctx := context.TODO()

	keyName := fmt.Sprintf("%s-%x", testKeyPrefix, time.Now().Unix())
	alias := fmt.Sprintf("%s-%x", testAliasPrefix, time.Now().Unix())

	// Constructs a aws kms key provider config that accepts the alias as the key id
	providerConfig := Config{
		KMSKeyID: alias,
		KeySpec:  "AES_256",
	}

	// Mimic the creation of the aws client here via providerConfig.asAWSBase() so that
	// we create a key in the same way that it will be read
	awsBaseConfig, err := providerConfig.asAWSBase()
	if err != nil {
		t.Fatalf("Error creating AWS config: %s", err)
	}
	_, awsConfig, awsDiags := awsbase.GetAwsConfig(ctx, awsBaseConfig)
	if awsDiags.HasError() {
		t.Fatalf("Error creating AWS config: %v", awsDiags)
	}

	kmsClient := kms.NewFromConfig(awsConfig)

	// Create the key
	keyId := createKMSKey(ctx, t, kmsClient, keyName, awsBaseConfig.Region)
	defer scheduleKMSKeyDeletion(ctx, t, kms.NewFromConfig(awsConfig), keyId)

	// Create an alias for the key
	createAlias(ctx, t, kmsClient, keyId, &alias)
	defer deleteAlias(ctx, t, kms.NewFromConfig(awsConfig), &alias)

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

// createKMSKey creates a KMS key with the given name and region
func createKMSKey(ctx context.Context, t *testing.T, kmsClient *kms.Client, keyName string, region string) (keyID string) {
	createKeyReq := kms.CreateKeyInput{
		Tags: []types.Tag{
			{
				TagKey:   aws.String("Name"),
				TagValue: aws.String(keyName),
			},
		},
	}

	t.Logf("Creating KMS key %s in %s", keyName, region)

	created, err := kmsClient.CreateKey(ctx, &createKeyReq)
	if err != nil {
		t.Fatalf("Error creating KMS key: %s", err)
	}

	return *created.KeyMetadata.KeyId
}

// createAlias creates a KMS alias for the given key
func createAlias(ctx context.Context, t *testing.T, kmsClient *kms.Client, keyID string, alias *string) {
	if alias == nil {
		return
	}

	t.Logf("Creating KMS alias %s for key %s", *alias, keyID)

	aliasReq := kms.CreateAliasInput{
		AliasName:   aws.String(*alias),
		TargetKeyId: aws.String(keyID),
	}

	_, err := kmsClient.CreateAlias(ctx, &aliasReq)
	if err != nil {
		t.Fatalf("Error creating KMS alias: %s", err)
	}
}

// scheduleKMSKeyDeletion schedules the deletion of a KMS key
// this attempts to delete it in the fastest possible way (7 days)
func scheduleKMSKeyDeletion(ctx context.Context, t *testing.T, kmsClient *kms.Client, keyID string) {
	deleteKeyReq := kms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(keyID),
		PendingWindowInDays: aws.Int32(7),
	}

	t.Logf("Scheduling KMS key %s for deletion", keyID)

	_, err := kmsClient.ScheduleKeyDeletion(ctx, &deleteKeyReq)
	if err != nil {
		t.Fatalf("Error deleting KMS key: %s", err)
	}
}

// deleteAlias deletes a KMS alias
func deleteAlias(ctx context.Context, t *testing.T, kmsClient *kms.Client, alias *string) {
	if alias == nil {
		return
	}

	t.Logf("Deleting KMS alias %s", *alias)

	deleteAliasReq := kms.DeleteAliasInput{
		AliasName: aws.String(*alias),
	}

	_, err := kmsClient.DeleteAlias(ctx, &deleteAliasReq)
	if err != nil {
		t.Fatalf("Error deleting KMS alias: %s", err)
	}
}
