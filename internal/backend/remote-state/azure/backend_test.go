// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"os"
	"testing"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/acctest"
	"github.com/tombuildsstuff/giovanni/storage/2018-11-09/blob/containers"
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
			if be.armClient.timeoutSeconds != tc.timeoutSeconds {
				t.Fatalf("Expected timeoutSeconds to be %d, got %d", tc.timeoutSeconds, be.armClient.timeoutSeconds)
			}
		})
	}
}

// TestAccBackendAccessKeyBasic tests if resources are created using basic access key.
// The call to backend.TestBackendStates tests workspace creation, list and deletion.
func TestAccBackendAccessKeyBasic(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendSASTokenBasic(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	sasToken, err := buildSasToken(res.storageAccountName, res.storageAccountAccessKey)
	if err != nil {
		t.Fatalf("Error building SAS Token: %+v", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"sas_token":            *sasToken,
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendOIDCBasic(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"resource_group_name":  res.resourceGroup,
		"use_oidc":             true,
		"subscription_id":      os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":            os.Getenv("ARM_TENANT_ID"),
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendManagedServiceIdentityBasic(t *testing.T) {
	testAccAzureBackendRunningInAzure(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"resource_group_name":  res.resourceGroup,
		"use_msi":              true,
		"subscription_id":      os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":            os.Getenv("ARM_TENANT_ID"),
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendServicePrincipalClientCertificateBasic(t *testing.T) {
	testAccAzureBackend(t)

	clientCertPassword := os.Getenv("ARM_CLIENT_CERTIFICATE_PASSWORD")
	clientCertPath := os.Getenv("ARM_CLIENT_CERTIFICATE_PATH")
	if clientCertPath == "" {
		t.Skip("Skipping since `ARM_CLIENT_CERTIFICATE_PATH` is not specified!")
	}

	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name":        res.storageAccountName,
		"container_name":              res.storageContainerName,
		"key":                         res.storageKeyName,
		"resource_group_name":         res.resourceGroup,
		"subscription_id":             os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":                   os.Getenv("ARM_TENANT_ID"),
		"client_id":                   os.Getenv("ARM_CLIENT_ID"),
		"client_certificate_password": clientCertPassword,
		"client_certificate_path":     clientCertPath,
		"environment":                 os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":                    os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendServicePrincipalClientSecretBasic(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"resource_group_name":  res.resourceGroup,
		"subscription_id":      os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":            os.Getenv("ARM_TENANT_ID"),
		"client_id":            os.Getenv("ARM_CLIENT_ID"),
		"client_secret":        os.Getenv("ARM_CLIENT_SECRET"),
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendServicePrincipalClientSecretCustomEndpoint(t *testing.T) {
	testAccAzureBackend(t)

	// this is only applicable for Azure Stack.
	endpoint := os.Getenv("ARM_ENDPOINT")
	if endpoint == "" {
		t.Skip("Skipping as ARM_ENDPOINT isn't configured")
	}

	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"resource_group_name":  res.resourceGroup,
		"subscription_id":      os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":            os.Getenv("ARM_TENANT_ID"),
		"client_id":            os.Getenv("ARM_CLIENT_ID"),
		"client_secret":        os.Getenv("ARM_CLIENT_SECRET"),
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             endpoint,
	})).(*Backend)

	backend.TestBackendStates(t, b)
}

func TestAccBackendAccessKeyLocked(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStateLocks(t, b1, b2)
	backend.TestBackendStateForceUnlock(t, b1, b2)

	backend.TestBackendStateLocksInWS(t, b1, b2, "foo")
	backend.TestBackendStateForceUnlockInWS(t, b1, b2, "foo")
}

func TestAccBackendServicePrincipalLocked(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")
	armClient := buildTestClient(t, res)

	err := armClient.buildTestResources(t, t.Context(), &res)
	t.Cleanup(func() {
		if err := armClient.destroyTestResources(t, t.Context(), res); err != nil {
			t.Fatalf("error when destroying resources: %q", err)
		}
	})
	if err != nil {
		t.Fatalf("Error creating Test Resources: %q", err)
	}

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"subscription_id":      os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":            os.Getenv("ARM_TENANT_ID"),
		"client_id":            os.Getenv("ARM_CLIENT_ID"),
		"client_secret":        os.Getenv("ARM_CLIENT_SECRET"),
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"subscription_id":      os.Getenv("ARM_SUBSCRIPTION_ID"),
		"tenant_id":            os.Getenv("ARM_TENANT_ID"),
		"client_id":            os.Getenv("ARM_CLIENT_ID"),
		"client_secret":        os.Getenv("ARM_CLIENT_SECRET"),
		"environment":          os.Getenv("ARM_ENVIRONMENT"),
		"endpoint":             os.Getenv("ARM_ENDPOINT"),
	})).(*Backend)

	backend.TestBackendStateLocks(t, b1, b2)
	backend.TestBackendStateForceUnlock(t, b1, b2)

	backend.TestBackendStateLocksInWS(t, b1, b2, "foo")
	backend.TestBackendStateForceUnlockInWS(t, b1, b2, "foo")
}

type mockClient struct {
}

func (p mockClient) ListBlobs(ctx context.Context, accountName, containerName string, params containers.ListBlobsInput) (containers.ListBlobsResult, error) {
	blobDetails := make([]containers.BlobDetails, 5000)
	for i := range blobDetails {
		blobDetails[i].Name = "env-name"
	}

	returnMarker := "next-token"

	// This function will be called first with an empty parameter, putting the returnMarker as "next-token".
	// On the second call, the returnMarker won't be empty, then finishing the pagination function;
	if *params.Marker != "" {
		returnMarker = ""
	}

	listBlobsResult := containers.ListBlobsResult{
		Blobs: containers.Blobs{
			Blobs: blobDetails,
		},
		NextMarker: &returnMarker,
	}
	return listBlobsResult, nil
}

func TestBackendPagination(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	result, err := getPaginatedResults(ctx, client, "env", "acc-name", "storage-name")
	if err != nil {
		t.Fatalf("error getting paginated results %q", err)
	}

	// default is always on the list + 10k generated blobs from the mocked ListBlobs
	if len(result) != 10001 {
		t.Fatalf("expected len 10001, got %d instead", len(result))
	}
}
