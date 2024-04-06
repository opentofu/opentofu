package azure_kms

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// skipCheck checks if the test should be skipped or not based on environment variables
func skipCheck(t *testing.T) {
	// check if TF_ACC and TF_AZURE_KMS_TEST are unset
	// if so, skip the test
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_AZURE_KMS_TEST") == "" {
		t.Log("Skipping test because TF_ACC or TF_AZURE_KMS_TEST is not set")
		t.Skip()
	}
}

const testVaultPrefix = "tf-acc-test-vault"
const testKeyPrefix = "tf-acc-test-kms-key"

func TestAzureKeyVaultProvider_Simple(t *testing.T) {
	skipCheck(t)
	ctx := context.TODO()

	resourceGroupName := fmt.Sprintf("rg-%s-%x", testVaultPrefix, time.Now().Unix())
	vaultName := fmt.Sprintf("%s-%x", testVaultPrefix, time.Now().Unix())
	keyName := fmt.Sprintf("%s-%x", testKeyPrefix, time.Now().Unix())

	// Constructs a aws kms key provider config that accepts the alias as the key id
	providerConfig := Config{
		VaultName:    vaultName,
		KeyName:      keyName,
		KeyAlgorithm: "AES_256",
	}

	creds, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		t.Fatalf("Unable to authenticate to azure %s", err)
	}

	vaultClientFactory, err := armkeyvault.NewClientFactory("", creds, nil)
	if err != nil {
		t.Fatalf("error creating key vault client: %s", err)
	}

	vaultClient := vaultClientFactory.NewVaultsClient()
	keyClient, err := azkeys.NewClient(vaultName, creds, nil)
	if err != nil {
		t.Fatalf("error creating key client: %s", err)
	}

	_, err = createKeyVault(ctx, t, vaultClient, resourceGroupName, vaultName)
	if err != nil {
		t.Fatalf("error creating key vault: %s", err)
	}
	// defer deleteKeyVault(ctx, t, nil, vault)

	_, err = createKeyVaultKey(ctx, t, keyClient, keyName)
	if err != nil {
		t.Fatalf("error creating key encryption key: %s", err)
	}
	// defer scheduleAzureKeyVaultKeyDeletion(ctx, t, nil, key)

	provider, metaIn, err := providerConfig.Build()
	if err != nil {
		t.Fatalf("error building provider: %s", err)
	}

	output, _, err := provider.Provide(metaIn)
	if err != nil {
		t.Fatalf("Error providing keys: %s", err)
	}

	if len(output.EncryptionKey) <= 0 {
		t.Fatalf("Unable to obtain encryption key")
	}
}

func createKeyVaultKey(ctx context.Context, t *testing.T, client *azkeys.Client, keyName string) (azkeys.CreateKeyResponse, error) {

	props := azkeys.CreateKeyParameters{
		Kty:     to.Ptr(azkeys.KeyTypeRSA),
		KeySize: to.Ptr(int32(2048)),
	}

	res, err := client.CreateKey(ctx, keyName, props, nil)
	if err != nil {
		t.Fatalf("error creating key: %s", err)
	}

	return res, nil
}

func createKeyVault(ctx context.Context, t *testing.T, client *armkeyvault.VaultsClient, resourceGroupName, vaultName string) (armkeyvault.VaultsClientCreateOrUpdateResponse, error) {

	props := armkeyvault.VaultProperties{
		SKU:                          &armkeyvault.SKU{},
		TenantID:                     new(string),
		AccessPolicies:               []*armkeyvault.AccessPolicyEntry{},
		CreateMode:                   to.Ptr(armkeyvault.CreateModeDefault),
		EnablePurgeProtection:        new(bool),
		EnableRbacAuthorization:      new(bool),
		EnableSoftDelete:             new(bool),
		EnabledForDeployment:         new(bool),
		EnabledForDiskEncryption:     new(bool),
		EnabledForTemplateDeployment: new(bool),
		NetworkACLs:                  &armkeyvault.NetworkRuleSet{},
		PublicNetworkAccess:          new(string),
		SoftDeleteRetentionInDays:    new(int32),
		VaultURI:                     new(string),
		HsmPoolResourceID:            new(string),
		PrivateEndpointConnections:   []*armkeyvault.PrivateEndpointConnectionItem{},
	}

	poller, err := client.BeginCreateOrUpdate(ctx, resourceGroupName, vaultName, armkeyvault.VaultCreateOrUpdateParameters{
		Location:   to.Ptr("eastus"),
		Properties: &props,
	}, nil)
	if err != nil {
		t.Fatalf("error creating key vault: %s", err)
	}

	res, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		t.Fatalf("error creating KeyVault: %s", err)
	}

	return res, nil
}
