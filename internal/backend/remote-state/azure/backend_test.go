// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/remote-state/azure/auth"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/legacy/helper/acctest"
)

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = new(Backend)
}

func TestBackendConfig(t *testing.T) {
	// This test just instantiates the client. Shouldn't make any actual
	// requests nor incur any costs.

	config := map[string]interface{}{
		"storage_account_name": "tfaccount",
		"container_name":       "tfcontainer",
		"key":                  "state",
		"snapshot":             false,
		// Access Key must be Base64
		"access_key": "QUNDRVNTX0tFWQ0K",
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config)).(*Backend)

	if b.containerName != "tfcontainer" {
		t.Fatalf("Incorrect bucketName was populated")
	}
	if b.keyName != "state" {
		t.Fatalf("Incorrect keyName was populated")
	}
	if b.snapshot != false {
		t.Fatalf("Incorrect snapshot was populated")
	}
}

func TestBackendConfig_Timeout(t *testing.T) {
	config := map[string]any{
		"storage_account_name": "tfaccount",
		"container_name":       "tfcontainer",
		"key":                  "state",
		"snapshot":             false,
		// Access Key must be Base64
		"access_key": "QUNDRVNTX0tFWQ0K",
	}
	testCases := []struct {
		name           string
		timeoutSeconds any
		expectError    bool
	}{
		{
			name:           "string timeout",
			timeoutSeconds: "Nonsense",
			expectError:    true,
		},
		{
			name:           "negative timeout",
			timeoutSeconds: -10,
			expectError:    true,
		},
		{
			// 0 is a valid timeout value, it disables the timeout
			name:           "zero timeout",
			timeoutSeconds: 0,
		},
		{
			name:           "positive timeout",
			timeoutSeconds: 10,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config["timeout_seconds"] = tc.timeoutSeconds
			b, _, errors := backend.TestBackendConfigWarningsAndErrors(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config))
			if tc.expectError {
				if len(errors) == 0 {
					t.Fatalf("Expected an error")
				}
				return
			}
			if !tc.expectError && len(errors) > 0 {
				t.Fatalf("Expected no errors, got: %v", errors)
			}
			be, ok := b.(*Backend)
			if !ok || be == nil {
				t.Fatalf("Expected initialized Backend, got %T", b)
			}
			if int(be.timeout.Seconds()) != tc.timeoutSeconds {
				t.Fatalf("Expected timeoutSeconds to be %d, got %d", tc.timeoutSeconds, int(be.timeout.Seconds()))
			}
		})
	}
}

type mockClient struct {
	marker string
}

func (p mockClient) NewListBlobsFlatPager(params *container.ListBlobsFlatOptions) *runtime.Pager[container.ListBlobsFlatResponse] {
	env_name := "env-name"
	blobDetails := make([]*container.BlobItem, 5000)
	for i := range blobDetails {
		blobDetails[i] = &container.BlobItem{}
		blobDetails[i].Name = &env_name
	}

	returnMarker := "next-token"

	return runtime.NewPager(runtime.PagingHandler[container.ListBlobsFlatResponse]{
		More: func(resp container.ListBlobsFlatResponse) bool {
			return *resp.Marker != returnMarker
		},
		Fetcher: func(context.Context, *container.ListBlobsFlatResponse) (container.ListBlobsFlatResponse, error) {
			prevMarker := p.marker
			p.marker = returnMarker
			return container.ListBlobsFlatResponse{
				ListBlobsFlatSegmentResponse: container.ListBlobsFlatSegmentResponse{
					Segment: &container.BlobFlatListSegment{
						BlobItems: blobDetails,
					},
					Marker:     &prevMarker,
					NextMarker: &returnMarker,
				},
			}, nil
		},
	})
}

func TestBackendPagination(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	result, err := getPaginatedResults(ctx, client, "env")
	if err != nil {
		t.Fatalf("error getting paginated results %q", err)
	}

	// default is always on the list + 10k generated blobs from the mocked ListBlobs
	if len(result) != 10001 {
		t.Fatalf("expected len 10001, got %d instead", len(result))
	}
}

func TestStorageNames(t *testing.T) {
	err := checkAccountAndContainerNames("goodaccountname1000", "good-container-name-2")
	if err != nil {
		t.Fatalf("encountered an error on good storage name: %v", err)
	}
	err = checkAccountAndContainerNames("bad-accountname1000", "good-container-name-2")
	if err == nil {
		t.Fatalf("encountered no error on a bad storage name: account names cannot have hyphens")
	}
	err = checkAccountAndContainerNames("", "good-container-name-2")
	if err == nil {
		t.Fatalf("encountered no error on a bad storage name: account names must have 24 characters or fewer")
	}
	err = checkAccountAndContainerNames("aa", "good-container-name-2")
	if err == nil {
		t.Fatalf("encountered no error on a bad storage name: account names must have 3 characters or more")
	}
	err = checkAccountAndContainerNames("goodaccountname1000", "-bad-container-2")
	if err == nil {
		t.Fatalf("encountered no error on a bad container name: container cannot start with a hyphen")
	}
	err = checkAccountAndContainerNames("goodaccountname1000", "bad-container-2-")
	if err == nil {
		t.Fatalf("encountered no error on a bad container name: container cannot end with a hyphen")
	}
	err = checkAccountAndContainerNames("goodaccountname1000", "bad--container-2")
	if err == nil {
		t.Fatalf("encountered no error on a bad container name: container cannot have consecutive hyphens")
	}
	err = checkAccountAndContainerNames("goodaccountname1000", "myveryeducatedmotherjustservedusnachoswaitwhathappenedtothe9pies")
	if err == nil {
		t.Fatalf("encountered no error on a bad container name: containers must have 63 characters or fewer")
	}
	err = checkAccountAndContainerNames("goodaccountname1000", "x")
	if err == nil {
		t.Fatalf("encountered no error on a bad container name: containers must have 3 characters or more")
	}
}

// TestAccBackendAccessKeyBasic tests if the backend functions when using basic access key.
func TestAccBackendAccessKeyBasic(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	authMethod, err := auth.GetAuthMethod(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}

	resourceGroupClient, _, err := createTestResources(t, &res, authCred)

	t.Cleanup(func() {
		destroyTestResources(t, resourceGroupClient, res)
	})
	if err != nil {
		t.Fatal(err)
	}

	// The call to backend.TestBackendStates tests workspace creation, list and deletion.
	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"use_cli":              false,
	})).(*Backend)

	backend.TestBackendStates(t, b1)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"use_cli":              false,
	})).(*Backend)

	// TestBackendStateForceUnlock runs the both the TestBackendStateLocks test and the --force-unlock tests
	backend.TestBackendStateForceUnlock(t, b1, b2)
}

// TestAccBackendSASToken tests if the backend functions when using a SAS token.
func TestAccBackendSASToken(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	authMethod, err := auth.GetAuthMethod(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}

	resourceGroupClient, _, err := createTestResources(t, &res, authCred)

	t.Cleanup(func() {
		destroyTestResources(t, resourceGroupClient, res)
	})
	if err != nil {
		t.Fatal(err)
	}

	keycred, err := azblob.NewSharedKeyCredential(res.storageAccountName, res.storageAccountAccessKey)
	if err != nil {
		t.Fatal(err)
	}

	sasToken, err := getSASToken(keycred)
	if err != nil {
		t.Fatal(err)
	}

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"sas_token":            sasToken,
		"use_cli":              false,
	})).(*Backend)

	backend.TestBackendStates(t, b1)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"sas_token":            sasToken,
		"use_cli":              false,
	})).(*Backend)

	backend.TestBackendStateForceUnlock(t, b1, b2)
}

// TestAccBackendServicePrincipalClientSecret tests if the backend functions when using a client ID and secret.
func TestAccBackendServicePrincipalClientSecret(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	client_id := os.Getenv("TF_AZURE_TEST_CLIENT_ID")
	client_secret := os.Getenv("TF_AZURE_TEST_CLIENT_SECRET")
	if client_id == "" || client_secret == "" {
		t.Skip(`
A client ID or client secret was not provided.
Please set TF_AZURE_TEST_CLIENT_ID and TF_AZURE_TEST_CLIENT_SECRET, either manually or using the terraform plan in the meta-test folder.`)
	}
	if res.tenantID == "" {
		t.Fatal(errors.New("A tenant ID must be provided through ARM_TENANT_ID in order to run this test."))
	}

	authMethod, err := auth.GetAuthMethod(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}

	resourceGroupClient, _, err := createTestResources(t, &res, authCred)

	t.Cleanup(func() {
		destroyTestResources(t, resourceGroupClient, res)
	})
	if err != nil {
		t.Fatal(err)
	}

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"resource_group_name":  res.resourceGroup,
		"client_id":            client_id,
		"client_secret":        client_secret,
		"use_cli":              false,
	})).(*Backend)

	backend.TestBackendStates(t, b1)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"resource_group_name":  res.resourceGroup,
		"client_id":            client_id,
		"client_secret":        client_secret,
		"use_cli":              false,
	})).(*Backend)

	// TestBackendStateForceUnlock runs the both the TestBackendStateLocks test and the --force-unlock tests
	backend.TestBackendStateForceUnlock(t, b1, b2)
}

// TestAccBackendServicePrincipalClientCertificate tests if the backend functions when using a PFX certificate file.
func TestAccBackendServicePrincipalClientCertificate(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	client_id := os.Getenv("TF_AZURE_TEST_CLIENT_ID")
	cert_path := os.Getenv("TF_AZURE_TEST_CERT_PATH")
	// cert_password may be empty
	cert_password := os.Getenv("TF_AZURE_TEST_CERT_PASSWORD")
	if client_id == "" || cert_path == "" {
		t.Skip("A certificate must be provided through TF_AZURE_TEST_CERT_PATH, and a client_id must be provided through TF_AZURE_TEST_CLIENT_ID")
	}
	// Make sure we can open and read the file
	cert_file, err := os.Open(cert_path)
	if err != nil {
		t.Fatalf("error opening cert file: %s", err.Error())
	}
	_, err = io.ReadAll(cert_file)
	if err != nil {
		t.Fatalf("error reading cert file: %s", err.Error())
	}
	cert_file.Close()

	authMethod, err := auth.GetAuthMethod(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}

	resourceGroupClient, _, err := createTestResources(t, &res, authCred)

	t.Cleanup(func() {
		destroyTestResources(t, resourceGroupClient, res)
	})
	if err != nil {
		t.Fatal(err)
	}

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name":        res.storageAccountName,
		"container_name":              res.storageContainerName,
		"key":                         res.storageKeyName,
		"resource_group_name":         res.resourceGroup,
		"client_id":                   client_id,
		"client_certificate_path":     cert_path,
		"client_certificate_password": cert_password,
		"use_cli":                     false,
	})).(*Backend)

	backend.TestBackendStates(t, b1)
}

// TestAccBackendManagedServiceIdentity tests if the backend functions when using a managed service identity, like on an Azure VM.
// Note: this test does NOT create its own resource group, storage account, or storage container. You must set up that infrastructure
// manually, as well as the underlying managed service identity which this test depends upon.
func TestAccBackendManagedServiceIdentity(t *testing.T) {
	testAccAzureBackend(t)

	storageAccountName := os.Getenv("TF_AZURE_TEST_STORAGE_ACCOUNT_NAME")
	resourceGroupName := os.Getenv("TF_AZURE_TEST_RESOURCE_GROUP_NAME")
	containerName := os.Getenv("TF_AZURE_TEST_CONTAINER_NAME")

	if storageAccountName == "" || resourceGroupName == "" || containerName == "" {
		t.Skip("For MSI tests, all infrastructure must be set up ahead of time and passed through environment variables.")
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"key":                  "testState",
		"resource_group_name":  resourceGroupName,
		"use_msi":              true,
		"use_cli":              false,
	})).(*Backend)

	backend.TestBackendStates(t, b)

	// Manually delete all blobs in the container
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

	deleteBlobsManually(t, authCred, storageAccountName, resourceGroupName, containerName)
}

// TestAccBackendAKSWorkloadIdentity tests if the backend functions when using workload identity, on Azure AKS (Kubernetes).
// Note: this test does NOT create its own resource group, storage account, or storage container. You must set up that infrastructure
// manually, as well as the kubernetes cluster, workload identity, and managed identity which this test depends upon.
func TestAccBackendAKSWorkloadIdentity(t *testing.T) {
	testAccAzureBackend(t)

	storageAccountName := os.Getenv("TF_AZURE_TEST_STORAGE_ACCOUNT_NAME")
	resourceGroupName := os.Getenv("TF_AZURE_TEST_RESOURCE_GROUP_NAME")
	containerName := os.Getenv("TF_AZURE_TEST_CONTAINER_NAME")

	if storageAccountName == "" || resourceGroupName == "" || containerName == "" {
		t.Skip("For MSI tests, all infrastructure must be set up ahead of time and passed through environment variables.")
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name":      storageAccountName,
		"container_name":            containerName,
		"key":                       "testState",
		"resource_group_name":       resourceGroupName,
		"use_aks_workload_identity": true,
		"use_cli":                   false,
	})).(*Backend)

	backend.TestBackendStates(t, b)
	client := httpclient.New(t.Context())

	authCred, err := azidentity.NewWorkloadIdentityCredential(
		&azidentity.WorkloadIdentityCredentialOptions{ClientOptions: azcore.ClientOptions{
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

	// Manually delete all blobs in the container
	deleteBlobsManually(t, authCred, storageAccountName, resourceGroupName, containerName)
}
