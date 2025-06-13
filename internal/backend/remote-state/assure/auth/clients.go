package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func clientOptions(client *http.Client) policy.ClientOptions {
	return policy.ClientOptions{
		Telemetry: policy.TelemetryOptions{
			Disabled: true,
		},
		Transport: client,
	}
}

// NewResourceClient gets a client for resource groups. This is strictly only used in testing.
func NewResourceClient(client *http.Client, authCred azcore.TokenCredential, subscriptionID string) (*armresources.ResourceGroupsClient, error) {
	resourcesClientFactory, err := armresources.NewClientFactory(subscriptionID, authCred, &arm.ClientOptions{
		ClientOptions:         clientOptions(client),
		DisableRPRegistration: false,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting resource client factory: %w", err)
	}
	return resourcesClientFactory.NewResourceGroupsClient(), nil
}

// NewStorageAccountsClient gets a client for the storage account with the given auth credentials.
// This should only be used for testing and internally within this package.
func NewStorageAccountsClient(client *http.Client, authCred azcore.TokenCredential, subscriptionID string) (*armstorage.AccountsClient, error) {
	storageClientFactory, err := armstorage.NewClientFactory(subscriptionID, authCred, &arm.ClientOptions{
		ClientOptions:         clientOptions(client),
		DisableRPRegistration: false,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting storage client factory: %w", err)
	}
	return storageClientFactory.NewAccountsClient(), nil
}

type StorageAddresses struct {
	CustomResourceManagerEndpoint string // TODO check if used
	Environment                   string // TODO check if used
	MetadataHost                  string // TODO check if used
	ResourceGroup                 string
	StorageAccount                string
	StorageContainer              string
	SubscriptionID                string
	TenantID                      string
}

// NewContainerClientWithSharedKeyCredentialAndKey gets a container client authenticated with
// a shared Storage Account Access Key, using previously obtained authentication credentials to
// obtain said key from the Storage Account.
func NewContainerClientWithSharedKeyCredential(ctx context.Context, names StorageAddresses, authCred azcore.TokenCredential) (*container.Client, error) {
	containerClient, _, err := NewContainerClientWithSharedKeyCredentialAndKey(ctx, names, authCred)
	return containerClient, err
}

func checkNamesForAccessKeyCredentials(names StorageAddresses) error {
	var diags tfdiags.Diagnostics
	if names.ResourceGroup == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Resource Group is empty",
			"In order to obtain a Storage Account Access Key, a resource group is necessary",
		))
	}
	if names.StorageAccount == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Storage Account is empty",
			"In order to obtain a Storage Account Access Key, a storage account name is necessary",
		))
	}
	if names.SubscriptionID == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Subscription ID is empty",
			"In order to obtain a Storage Account Access Key, a subscription id is necessary",
		))
	}
	return diags.Err()
}

// NewContainerClientWithSharedKeyCredentialAndKey gets a container client and shared key
// that it's authenticated with. This function should only be used for testing and internally within this package.
func NewContainerClientWithSharedKeyCredentialAndKey(ctx context.Context, names StorageAddresses, authCred azcore.TokenCredential) (*container.Client, string, error) {
	client := httpclient.New(ctx)
	// Lookup the key with an account client
	accountsClient, err := NewStorageAccountsClient(client, authCred, names.SubscriptionID)
	if err != nil {
		return nil, "", err
	}
	keys, err := accountsClient.ListKeys(ctx, names.ResourceGroup, names.StorageAccount, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error listing access keys on the storage account: %w", err)
	}
	if len(keys.Keys) == 0 || keys.Keys[0] == nil || keys.Keys[0].Value == nil {
		return nil, "", fmt.Errorf("malformed structure returned from the ListKeys function")
	}

	storageAccessKey := *keys.Keys[0].Value

	return newContainerClientFromStorageAccessKey(client, names, storageAccessKey)
}

// NewContainerClientFromStorageAccessKey gets a container client authenticated with
// the provided Storage Account Access Key.
func NewContainerClientFromStorageAccessKey(ctx context.Context, names StorageAddresses, storageAccessKey string) (*container.Client, error) {
	client := httpclient.New(ctx)
	containerClient, _, err := newContainerClientFromStorageAccessKey(client, names, storageAccessKey)
	return containerClient, err
}

func containerURL(names StorageAddresses) string {
	// TODO we may want to do further error and name checking on this URL
	// Moreover, is this correct if the environment is different? If we're using Stack?
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s", names.StorageAccount, names.StorageContainer)
}

func newContainerClientFromStorageAccessKey(client *http.Client, names StorageAddresses, storageAccessKey string) (*container.Client, string, error) {
	sharedKeyCredential, err := container.NewSharedKeyCredential(names.StorageAccount, storageAccessKey)
	if err != nil {
		return nil, "", fmt.Errorf("error creating credential from shared access key: %w", err)
	}
	containerURL := containerURL(names)

	containerClient, err := container.NewClientWithSharedKeyCredential(containerURL, sharedKeyCredential, &container.ClientOptions{
		ClientOptions: clientOptions(client),
	})
	if err != nil {
		return nil, "", fmt.Errorf("error obtaining container client from access key: %w", err)
	}
	return containerClient, storageAccessKey, nil
}

// NewContainerClient gets a client authenticated with a Shared Access Signature
func NewContainerClientFromSAS(ctx context.Context, names StorageAddresses, sasToken string) (*container.Client, error) {
	client := httpclient.New(ctx)
	url := containerURL(names)

	containerURL := fmt.Sprintf("%s?%s", url, sasToken)

	return container.NewClientWithNoCredential(containerURL, &container.ClientOptions{
		ClientOptions: clientOptions(client),
	})
}

// NewContainerClient gets a client authenticated with the given auth credentials.
func NewContainerClient(ctx context.Context, names StorageAddresses, authCred azcore.TokenCredential) (*container.Client, error) {
	client := httpclient.New(ctx)
	return container.NewClient(containerURL(names), authCred, &container.ClientOptions{
		ClientOptions: clientOptions(client),
	})
}
