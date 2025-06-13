// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/opentofu/opentofu/internal/backend/remote-state/azure/auth"
	"github.com/opentofu/opentofu/internal/httpclient"
)

// verify that we are doing ACC tests or the Azure tests specifically
func testAccAzureBackend(t *testing.T) {
	skip := os.Getenv("TF_ACC") == "" && os.Getenv("TF_AZURE_TEST") == ""
	if skip {
		t.Log("azure backend tests require setting TF_ACC or TF_AZURE_TEST")
		t.Skip()
	}
}

type resourceNames struct {
	subscriptionID          string
	tenantID                string
	resourceGroup           string
	location                string
	storageAccountName      string
	storageContainerName    string
	storageKeyName          string
	storageAccountAccessKey string
}

func testResourceNames(rString string, keyName string) resourceNames {
	return resourceNames{
		subscriptionID:       os.Getenv("ARM_SUBSCRIPTION_ID"),
		tenantID:             os.Getenv("ARM_TENANT_ID"),
		resourceGroup:        fmt.Sprintf("acctestRG-backend-%s-%s", strings.Replace(time.Now().Local().Format("060102150405.00"), ".", "", 1), rString),
		location:             os.Getenv("ARM_LOCATION"),
		storageAccountName:   fmt.Sprintf("acctestsa%s", rString),
		storageContainerName: "acctestcont",
		storageKeyName:       keyName,
	}
}

// createTestResources creates a resource group, a storage account, and a storage container.
// Additionally, it sets the storageAccountAccessKey to a valid key on that storage account,
// and returns a client for both the storage container and the resource group (the latter for
// cleanup purposes).
func createTestResources(t *testing.T, res *resourceNames, authCred azcore.TokenCredential) (*armresources.ResourceGroupsClient, *container.Client, error) {
	client := httpclient.New(t.Context())
	resourceGroupClient, err := auth.NewResourceClient(client, authCred, res.subscriptionID)
	if err != nil {
		return nil, nil, err
	}

	// Create Resource Group
	_, err = resourceGroupClient.CreateOrUpdate(t.Context(), res.resourceGroup, armresources.ResourceGroup{Location: &res.location}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating resource group: %w", err)
	}

	accountsClient, err := auth.NewStorageAccountsClient(client, authCred, cloud.AzurePublic, res.subscriptionID)
	if err != nil {
		return nil, nil, err
	}

	// Create Storage Account
	future, err := accountsClient.BeginCreate(t.Context(), res.resourceGroup, res.storageAccountName, armstorage.AccountCreateParameters{
		Kind:     to.Ptr(armstorage.KindStorageV2),
		Location: &res.location,
		SKU: &armstorage.SKU{
			Name: to.Ptr(armstorage.SKUNameStandardLRS),
			Tier: to.Ptr(armstorage.SKUTierStandard),
		},
	}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test storage account: %w", err)
	}
	// Wait until the Storage Account is fully created
	_, err = future.PollUntilDone(t.Context(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed waiting for the creation of storage account: %w", err)
	}

	containerClient, key, err := auth.NewContainerClientWithSharedKeyCredentialAndKey(
		t.Context(),
		auth.StorageAddresses{
			CloudConfig:      cloud.AzurePublic,
			SubscriptionID:   res.subscriptionID,
			StorageAccount:   res.storageAccountName,
			StorageSuffix:    "core.windows.net",
			ResourceGroup:    res.resourceGroup,
			StorageContainer: res.storageContainerName,
		},
		authCred,
	)
	if err != nil {
		return nil, nil, err
	}

	// The storage account access key is used in some tests: we "return" it through the resource name pointer
	res.storageAccountAccessKey = key

	// Create a Storage Container
	_, err = containerClient.Create(t.Context(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating storage container: %w", err)
	}
	return resourceGroupClient, containerClient, nil
}

var PERMISSIONS sas.AccountPermissions = sas.AccountPermissions{
	Read:    true,
	Write:   true,
	Delete:  true,
	List:    true,
	Add:     true,
	Create:  true,
	Update:  true,
	Process: true,
}

func getSASToken(sharedKey *sas.SharedKeyCredential) (string, error) {
	utcNow := time.Now().UTC()

	// account for servers being up to 5 minutes out
	startDate := utcNow.Add(time.Minute * -5)
	endDate := utcNow.Add(time.Hour * 24)

	qps, err := sas.AccountSignatureValues{
		Version:       sas.Version,
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     startDate,
		ExpiryTime:    endDate,
		Permissions:   PERMISSIONS.String(),
		ResourceTypes: "sco",
	}.SignWithSharedKey(sharedKey)
	if err != nil {
		return "", err
	}
	return qps.Encode(), nil
}

func deleteBlobsInMSI(t *testing.T, storageAccountName, resourceGroupName, containerName string) {
	client := httpclient.New(t.Context())

	authCred, err := azidentity.NewManagedIdentityCredential(
		&azidentity.ManagedIdentityCredentialOptions{ClientOptions: azcore.ClientOptions{
			Telemetry: policy.TelemetryOptions{
				Disabled: true,
			},
			Transport: client,
			Cloud:     cloud.AzurePublic,
		}},
	)
	if err != nil {
		t.Logf("Skipping deleting blobs in container %s due to error obtaining credentials: %v", containerName, err)
		return
	}
	names := auth.StorageAddresses{
		CloudConfig:      cloud.AzurePublic,
		ResourceGroup:    resourceGroupName,
		StorageAccount:   storageAccountName,
		StorageContainer: containerName,
		StorageSuffix:    "core.windows.net",
	}

	containerClient, err := auth.NewContainerClient(t.Context(), names, authCred)
	if err != nil {
		t.Logf("Skipping deleting blobs in container %s due to error obtaining container client: %v", containerName, err)
		return
	}

	pager := containerClient.NewListBlobsFlatPager(nil)
	for pager.More() {
		resp, err := pager.NextPage(t.Context())
		if err != nil || resp.Segment == nil || resp.Segment.BlobItems == nil {
			t.Logf("error getting blob page, ignoring and continuing: %v", err)
			continue
		}
		for _, obj := range resp.Segment.BlobItems {
			_, err := containerClient.NewBlobClient(*obj.Name).Delete(t.Context(), nil)
			if err != nil {
				t.Logf("error deleting blob, ignoring and continuing: %v", err)
				continue
			}
		}
	}
}

func destroyTestResources(t *testing.T, resourceGroupClient *armresources.ResourceGroupsClient, res resourceNames) {
	_, err := resourceGroupClient.BeginDelete(context.Background(), res.resourceGroup, nil)
	if err != nil {
		t.Fatalf("Error deleting Resource Group: %v", err)
	}
}

func emptyAuthConfig() *auth.Config {
	return &auth.Config{
		AzureCLIAuthConfig: auth.AzureCLIAuthConfig{
			CLIAuthEnabled: true,
		},
		ClientSecretCredentialAuthConfig: auth.ClientSecretCredentialAuthConfig{},
		ClientCertificateAuthConfig:      auth.ClientCertificateAuthConfig{},
		OIDCAuthConfig:                   auth.OIDCAuthConfig{},
		MSIAuthConfig:                    auth.MSIAuthConfig{},
		StorageAddresses:                 auth.StorageAddresses{},
	}
}
