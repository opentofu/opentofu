// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/remote-state/azure/auth"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/acctest"
	"github.com/opentofu/opentofu/internal/states/remote"
)

func TestRemoteClient_impl(t *testing.T) {
	var _ remote.Client = new(RemoteClient)
	var _ remote.ClientLocker = new(RemoteClient)
}

func TestPutMaintainsMetadata(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	authMethod, err := auth.GetAuthMethod(t.Context(), emptyAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), emptyAuthConfig())
	if err != nil {
		t.Fatal(err)
	}

	resourceGroupClient, containerClient, err := createTestResources(t, &res, authCred)

	t.Cleanup(func() {
		destroyTestResources(t, resourceGroupClient, res)
	})
	if err != nil {
		t.Fatal(err)
	}

	headerName := "acceptancetest"
	expectedValue := "f3b56bad-33ad-4b93-a600-7a66e9cbd1eb"

	blobClient := containerClient.NewBlockBlobClient(res.storageKeyName)

	// PUT
	_, err = blobClient.UploadBuffer(t.Context(), []byte{}, nil)
	if err != nil {
		t.Fatalf("Error Creating Block Blob: %+v", err)
	}

	remoteClient := RemoteClient{
		blobClient: blobClient,
		timeout:    time.Duration(180) * time.Second,
	}

	// GET PROPERTIES
	blobReference, err := remoteClient.getBlobProperties(t.Context())
	if err != nil {
		t.Fatalf("Error loading Metadata: %+v", err)
	}
	// CHANGE + SET METADATA
	// Metadata should be empty; this is a new blob.
	blobReference.Metadata = make(map[string]*string)
	blobReference.Metadata[headerName] = &expectedValue
	_, err = blobClient.SetMetadata(t.Context(), blobReference.Metadata, nil)
	if err != nil {
		t.Fatalf("Error setting Metadata: %+v", err)
	}

	// UPDATE WITH PUT
	bytes := []byte(acctest.RandString(20))
	err = remoteClient.Put(t.Context(), bytes)
	if err != nil {
		t.Fatalf("Error putting data: %+v", err)
	}
	// CHECK METADATA AGAIN, SEE THAT IT IS NOT SQUOOSHED
	blobReference, err = remoteClient.getBlobProperties(t.Context())
	if err != nil {
		t.Fatalf("Error loading Metadata: %+v", err)
	}

	if metaval, ok := blobReference.Metadata[headerName]; !ok || *metaval != expectedValue {
		t.Fatalf("%q was not set to %q in the Metadata: %+v", headerName, expectedValue, blobReference.Metadata)
	}
}

func TestAccRemoteClientAccessKeyBasic(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	authMethod, err := auth.GetAuthMethod(t.Context(), emptyAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), emptyAuthConfig())
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
		"access_key":           res.storageAccountAccessKey,
		"use_cli":              false,
	})).(*Backend)

	s1, err := b1.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, s1.(*remote.State).Client)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"use_cli":              false,
	})).(*Backend)

	s2, err := b2.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
}

func TestAccRemoteClientSASToken(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	authMethod, err := auth.GetAuthMethod(t.Context(), emptyAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), emptyAuthConfig())
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

	s1, err := b1.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, s1.(*remote.State).Client)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"sas_token":            sasToken,
		"use_cli":              false,
	})).(*Backend)

	s2, err := b2.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
}

func TestAccRemoteClientServicePrincipalClientSecret(t *testing.T) {
	testAccAzureBackend(t)
	rs := acctest.RandString(4)
	res := testResourceNames(rs, "testState")

	client_id := os.Getenv("TF_AZURE_TEST_CLIENT_ID")
	client_secret := os.Getenv("TF_AZURE_TEST_SECRET")
	if client_id == "" || client_secret == "" {
		t.Skip(`
A client ID or client secret was not provided.
Please set TF_AZURE_TEST_CLIENT_ID and TF_AZURE_TEST_SECRET, either manually or using the terraform plan in the meta-test folder.`)
	}
	if res.tenantID == "" {
		t.Fatal(errors.New("A tenant ID must be provided through ARM_TENANT_ID in order to run this test."))
	}

	authMethod, err := auth.GetAuthMethod(t.Context(), emptyAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	authCred, err := authMethod.Construct(t.Context(), emptyAuthConfig())
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

	s1, err := b1.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, s1.(*remote.State).Client)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"storage_account_name": res.storageAccountName,
		"container_name":       res.storageContainerName,
		"key":                  res.storageKeyName,
		"access_key":           res.storageAccountAccessKey,
		"resource_group_name":  res.resourceGroup,
		"client_id":            client_id,
		"client_secret":        client_secret,
		"use_cli":              false,
	})).(*Backend)

	s2, err := b2.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
}
