// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func clientOptions(client *http.Client, cloudConfig cloud.Configuration) policy.ClientOptions {
	return policy.ClientOptions{
		Telemetry: policy.TelemetryOptions{
			Disabled: true,
		},
		Transport: client,
		Cloud:     cloudConfig,
	}
}

// NewResourceClient gets a client for resource groups. This is strictly only used in testing.
func NewResourceClient(client *http.Client, authCred azcore.TokenCredential, subscriptionID string) (*armresources.ResourceGroupsClient, error) {
	resourceClient, err := armresources.NewResourceGroupsClient(subscriptionID, authCred, &arm.ClientOptions{
		ClientOptions:         clientOptions(client, cloud.AzurePublic),
		DisableRPRegistration: false,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting resource client: %w", err)
	}
	return resourceClient, nil
}

// NewStorageAccountsClient gets a client for the storage account with the given auth credentials.
// This should only be used for testing and internally within this package.
func NewStorageAccountsClient(client *http.Client, authCred azcore.TokenCredential, cloudConfig cloud.Configuration, subscriptionID string) (*armstorage.AccountsClient, error) {
	storageClient, err := armstorage.NewAccountsClient(subscriptionID, authCred, &arm.ClientOptions{
		ClientOptions:         clientOptions(client, cloudConfig),
		DisableRPRegistration: false,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting storage client: %w", err)
	}
	return storageClient, nil
}

type StorageAddresses struct {
	CloudConfig      cloud.Configuration
	ResourceGroup    string
	StorageAccount   string
	StorageContainer string
	StorageSuffix    string
	SubscriptionID   string
	TenantID         string
}

// NewContainerClientWithSharedKeyCredential gets a container client authenticated with
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
	accountsClient, err := NewStorageAccountsClient(client, authCred, names.CloudConfig, names.SubscriptionID)
	if err != nil {
		return nil, "", err
	}
	keys, err := accountsClient.ListKeys(ctx, names.ResourceGroup, names.StorageAccount, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error listing access keys on the storage account: %w", err)
	}
	if len(keys.Keys) == 0 || keys.Keys[0] == nil || keys.Keys[0].Value == nil {
		return nil, "", errors.New("malformed structure returned from the ListKeys function")
	}

	storageAccessKey := *keys.Keys[0].Value

	return newContainerClientFromStorageAccessKey(client, names, storageAccessKey)
}

const STORAGE cloud.ServiceName = "storage"

func CloudConfigFromAddresses(ctx context.Context, environment, metadataHost string) (cloud.Configuration, string, error) {
	if metadataHost != "" {
		config, err := CloudConfigFromMetadataHost(ctx, metadataHost)
		return config, config.Services[STORAGE].Endpoint, err
	}

	// These environments come from the hamilton Azure library, which was the predecessor to this implementation
	// https://github.com/manicminer/hamilton/blob/v0.44.0/environments/environments.go#L103-L118
	switch environment {
	case "", "public", "global", "canary":
		return cloud.AzurePublic, "core.windows.net", nil
	case "usgovernment", "usgovernmentl4", "dod", "usgovernmentl5":
		return cloud.AzureGovernment, "core.usgovcloudapi.net", nil
	case "china":
		return cloud.AzureChina, "core.chinacloudapi.cn", nil
	}
	return cloud.Configuration{}, "", fmt.Errorf("unknown environment identifier: %s", environment)
}

type Authentication struct {
	LoginEndpoint string   `json:"loginEndpoint"`
	Audiences     []string `json:"audiences"`
}

type Environment struct {
	Authentication  Authentication    `json:"authentication"`
	ResourceManager string            `json:"resourceManager"`
	Suffixes        map[string]string `json:"suffixes"`
}

func CloudConfigFromMetadataHost(ctx context.Context, metadataHost string) (cloud.Configuration, error) {
	// Obtaining cloud config from the metadata host
	client := httpclient.New(ctx)

	// If you change the API version here, verify the JSON response format is accurate to that version
	// You can check with this URL:
	// https://management.azure.com/metadata/endpoints?api-version=2023-11-01
	resp, err := client.Get(fmt.Sprintf("https://%s/metadata/endpoints?api-version=2023-11-01", metadataHost))
	if err != nil {
		return cloud.Configuration{}, fmt.Errorf("retrieving environments from Azure MetaData service: %w", err)
	}
	defer resp.Body.Close()

	var environment Environment
	if err := json.NewDecoder(resp.Body).Decode(&environment); err != nil {
		return cloud.Configuration{}, fmt.Errorf("decoding json in metadata response: %w", err)
	}

	storageSuffix, ok := environment.Suffixes["storage"]
	if !ok {
		return cloud.Configuration{}, errors.New("could not find storage endpoint in given metadata host")
	}
	if len(environment.Authentication.Audiences) == 0 {
		return cloud.Configuration{}, errors.New("could not find token audience in given metadata host")
	}
	audience := environment.Authentication.Audiences[0]

	return cloud.Configuration{
		ActiveDirectoryAuthorityHost: environment.Authentication.LoginEndpoint,
		Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
			cloud.ResourceManager: {
				Endpoint: environment.ResourceManager,
				Audience: audience,
			},
			STORAGE: {
				Endpoint: storageSuffix,
				Audience: audience,
			},
		},
	}, nil
}

// NewContainerClientFromStorageAccessKey gets a container client authenticated with
// the provided Storage Account Access Key.
func NewContainerClientFromStorageAccessKey(ctx context.Context, names StorageAddresses, storageAccessKey string) (*container.Client, error) {
	client := httpclient.New(ctx)
	containerClient, _, err := newContainerClientFromStorageAccessKey(client, names, storageAccessKey)
	return containerClient, err
}

// containerURL must only be called once it is verified that the StorageAccount and StorageContainer
// names are valid in Azure.
func containerURL(names StorageAddresses) string {
	return fmt.Sprintf("https://%s.blob.%s/%s", names.StorageAccount, names.StorageSuffix, names.StorageContainer)
}

func newContainerClientFromStorageAccessKey(client *http.Client, names StorageAddresses, storageAccessKey string) (*container.Client, string, error) {
	sharedKeyCredential, err := container.NewSharedKeyCredential(names.StorageAccount, storageAccessKey)
	if err != nil {
		return nil, "", fmt.Errorf("error creating credential from shared access key: %w", err)
	}
	containerURL := containerURL(names)

	containerClient, err := container.NewClientWithSharedKeyCredential(containerURL, sharedKeyCredential, &container.ClientOptions{
		ClientOptions: clientOptions(client, names.CloudConfig),
	})
	if err != nil {
		return nil, "", fmt.Errorf("error obtaining container client from access key: %w", err)
	}
	return containerClient, storageAccessKey, nil
}

// NewContainerClientFromSAS gets a client authenticated with a Shared Access Signature
func NewContainerClientFromSAS(ctx context.Context, names StorageAddresses, sasToken string) (*container.Client, error) {
	client := httpclient.New(ctx)
	url := containerURL(names)

	containerURL := fmt.Sprintf("%s?%s", url, sasToken)

	return container.NewClientWithNoCredential(containerURL, &container.ClientOptions{
		ClientOptions: clientOptions(client, names.CloudConfig),
	})
}

// NewContainerClient gets a client authenticated with the given auth credentials.
func NewContainerClient(ctx context.Context, names StorageAddresses, authCred azcore.TokenCredential) (*container.Client, error) {
	client := httpclient.New(ctx)
	return container.NewClient(containerURL(names), authCred, &container.ClientOptions{
		ClientOptions: clientOptions(client, names.CloudConfig),
	})
}
