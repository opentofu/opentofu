// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func backendConfig(t *testing.T, b *Backend, overrides map[string]string) cty.Value {
	t.Helper()
	values := map[string]cty.Value{"namespace": cty.StringVal("test-namespace"), "bucket": cty.StringVal("test-bucket")}
	for key, value := range overrides {
		values[key] = cty.StringVal(value)
	}
	config, err := b.ConfigSchema().CoerceValue(cty.ObjectVal(values))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestPrepareConfigDoesNotReadCredentials(t *testing.T) {
	t.Setenv("OCI_PRIVATE_KEY", "SECRET")
	t.Setenv("OCI_AUTH", "ResourcePrincipal")
	b := New(encryption.StateEncryptionDisabled()).(*Backend)
	config, diags := b.PrepareConfig(backendConfig(t, b, nil))
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	for _, name := range []string{"auth", "private_key", "region", "config_file_profile"} {
		if !config.GetAttr(name).IsNull() {
			t.Fatalf("%s must remain unset before Configure", name)
		}
	}
	if configString(config, "key") != "terraform.tfstate" || configString(config, "workspace_key_prefix") != "env:" || configString(config, "request_timeout") != "60s" {
		t.Fatal("incorrect storage defaults")
	}
	for _, name := range []string{"private_key", "private_key_password", "token_exchange_client_secret"} {
		if !b.ConfigSchema().Attributes[name].Sensitive {
			t.Fatalf("%s should be marked sensitive", name)
		}
	}
}

func TestPrepareConfigRejectsUnsafeStorageInputs(t *testing.T) {
	b := New(encryption.StateEncryptionDisabled()).(*Backend)
	for _, test := range []struct{ name, value string }{
		{"namespace", ""}, {"bucket", "bucket/other"}, {"key", "../state"}, {"key", "/state"},
		{"key", "state/"}, {"key", "a//state"}, {"workspace_key_prefix", "a/../b"},
		{"request_timeout", "0s"}, {"request_timeout", "invalid"}, {"auth", "SECRET-unsupported"},
	} {
		t.Run(test.name+"/"+test.value, func(t *testing.T) {
			_, diags := b.PrepareConfig(backendConfig(t, b, map[string]string{test.name: test.value}))
			if !diags.HasErrors() {
				t.Fatal("expected validation error")
			}
			if strings.Contains(diags.Err().Error(), "SECRET") {
				t.Fatal("validation disclosed the value")
			}
		})
	}
	if _, diags := b.PrepareConfig(backendConfig(t, b, map[string]string{"workspace_key_prefix": ""})); diags.HasErrors() {
		t.Fatal(diags.Err())
	}
}

func TestConfigureInitializesIndependentlyAndKeepsContext(t *testing.T) {
	b := New(encryption.StateEncryptionDisabled()).(*Backend)
	config, diags := b.PrepareConfig(backendConfig(t, b, map[string]string{"auth": "ApiKey", "private_key": "backend-only-key", "region": "us-ashburn-1", "request_timeout": "5s"}))
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "backend-context")
	calls := 0
	b.newClient = func(got context.Context, auth authConfig, timeout time.Duration) (objectStorageClient, error) {
		calls++
		if got.Value(contextKey{}) != "backend-context" || auth.PrivateKey != "backend-only-key" || auth.Region != "us-ashburn-1" || timeout != 5*time.Second {
			t.Fatal("configuration/context was not passed independently")
		}
		return newMockObjectStore(), nil
	}
	if diags := b.Configure(ctx, config); diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if calls != 1 || b.bucketName != "test-bucket" {
		t.Fatal("backend was not configured")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if diags := b.Configure(canceled, config); !diags.HasErrors() || calls != 1 {
		t.Fatal("canceled Configure should not initialize authentication")
	}
}

func TestStorageClientAppliesTimeoutAndDelegationAtInitialization(t *testing.T) {
	authTestEnvironment(t)
	clearDelegationEnvironment(t)
	rsaKey, key := authTestPrivateKey(t)
	cfg := authConfig{PrivateKey: key, TenancyOCID: "tenancy", UserOCID: "user", Fingerprint: "fp", Region: "us-ashburn-1"}
	t.Setenv("use_obo_token", "true")
	if _, err := newStorageClient(t.Context(), cfg, 7*time.Second); err == nil {
		t.Fatal("backend ignored missing enabled delegation")
	}
	t.Setenv("obo_token", "opaque-delegation")
	storage, err := newStorageClient(t.Context(), cfg, 7*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := storage.(*serializedObjectStorageClient).client.(*objectstorage.ObjectStorageClient)
	if client.HTTPClient.(*http.Client).Timeout != 7*time.Second {
		t.Fatal("SDK HTTP timeout was not applied")
	}
	request, err := http.NewRequest("GET", "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Interceptor(request); err != nil || request.Header.Get(delegationHeader) != "opaque-delegation" {
		t.Fatal("backend did not apply delegated identity")
	}
	// Exercise the assembled production client: the serialized SDK call uses
	// the snapshot signer and keeps the delegated header in its RSA signature.
	requests := 0
	client.HTTPClient = &http.Client{Transport: mockSDKTransport{http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		verifyDelegationSignature(t, request, rsaKey)
		writeMockOCIError(w, mockOCIError{404, "ObjectNotFound"})
	})}}
	stateClient := newRemoteClient(storage, "n", "b", "state", true, "")
	if payload, err := stateClient.Get(t.Context()); err != nil || payload != nil || requests != 1 {
		t.Fatalf("assembled backend SDK client failed: %v", err)
	}
}

type pagedObjectStore struct {
	objectStorageClient
	requests []objectstorage.ListObjectsRequest
	pages    []objectstorage.ListObjectsResponse
	err      error
}

func (s *pagedObjectStore) ListObjects(_ context.Context, request objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	s.requests = append(s.requests, request)
	if s.err != nil {
		return objectstorage.ListObjectsResponse{}, s.err
	}
	if len(s.pages) == 0 {
		return objectstorage.ListObjectsResponse{}, errors.New("unexpected listing")
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}
func objectNames(names ...string) []objectstorage.ObjectSummary {
	objects := make([]objectstorage.ObjectSummary, len(names))
	for i, name := range names {
		objects[i].Name = common.String(name)
	}
	return objects
}

func TestWorkspacesPaginationAndFiltering(t *testing.T) {
	store := &pagedObjectStore{pages: []objectstorage.ListObjectsResponse{
		{ListObjects: objectstorage.ListObjects{Objects: objectNames("env:/z/dir/state", "env:/a/dir/state.lock", "unrelated/a/dir/state"), NextStartWith: common.String("env:/z/dir/state")}},
		{ListObjects: objectstorage.ListObjects{Objects: objectNames("env:/a/dir/state", "env:/default/dir/state", "env:/z/dir/state", "env:/../dir/state", "env:/b/other")}},
	}}
	b := &Backend{storageClient: store, namespace: "n", bucketName: "b", key: "dir/state", workspaceKeyPrefix: "env:"}
	names, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"default", "a", "z"}) {
		t.Fatalf("workspaces: %v", names)
	}
	if *store.requests[0].Prefix != "env:/" || store.requests[1].Start == nil || *store.requests[1].Start != "env:/z/dir/state" {
		t.Fatal("incorrect pagination")
	}
}

func TestWorkspacesFailsOnPermissionAndRepeatedMarker(t *testing.T) {
	store := &pagedObjectStore{err: mockOCIError{403, "NotAuthorized"}}
	b := &Backend{storageClient: store, key: "state", workspaceKeyPrefix: "env:"}
	if _, err := b.Workspaces(t.Context()); err == nil || strings.Contains(err.Error(), "TOKEN") {
		t.Fatalf("unsafe listing error: %v", err)
	}
	store.err = nil
	page := objectstorage.ListObjectsResponse{ListObjects: objectstorage.ListObjects{NextStartWith: common.String("same")}}
	store.pages = []objectstorage.ListObjectsResponse{page, page}
	if _, err := b.Workspaces(t.Context()); err == nil {
		t.Fatal("repeated marker must fail")
	}
}

func TestWorkspaceObjectPaths(t *testing.T) {
	for _, prefix := range []string{"env:", "custom/path", ""} {
		b := &Backend{key: "nested/state", workspaceKeyPrefix: prefix}
		if b.stateFile("default") != "nested/state" {
			t.Fatal("default workspace moved")
		}
		if b.workspaceForObject(b.stateFile("dev")) != "dev" {
			t.Fatal("workspace path did not round trip")
		}
		if b.workspaceForObject(b.stateFile("dev")+".lock") != "" {
			t.Fatal("lock was treated as state")
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", "a\\b", "a%2fb", "a b"} {
		if validWorkspace(name) {
			t.Fatalf("unsafe workspace %q accepted", name)
		}
	}
}

func TestStateManagerInitializesLocksAndPreservesLineage(t *testing.T) {
	store := newMockObjectStore()
	b := &Backend{storageClient: store, namespace: "n", bucketName: "b", key: "state", workspaceKeyPrefix: "env:", encryption: encryption.StateEncryptionDisabled(), requestTimeout: 5 * time.Second}
	manager, err := b.StateMgr(t.Context(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if manager.State() == nil {
		t.Fatal("workspace state was not initialized")
	}
	if _, exists := store.objects["env:/dev/state.lock"]; exists {
		t.Fatal("initialization lock leaked")
	}
	meta := manager.(statemgr.PersistentMeta).StateSnapshotMeta()
	if meta.Lineage == "" {
		t.Fatal("initial state lacks lineage")
	}
	second, err := b.StateMgr(t.Context(), "dev")
	if err != nil || second != manager {
		t.Fatal("state manager was not retained for workspace deletion")
	}
	other := &Backend{storageClient: store, namespace: "n", bucketName: "b", key: "state", workspaceKeyPrefix: "env:", encryption: encryption.StateEncryptionDisabled()}
	third, err := other.StateMgr(t.Context(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if third.(statemgr.PersistentMeta).StateSnapshotMeta().Lineage != meta.Lineage {
		t.Fatal("existing lineage was replaced")
	}
	if manager.(*remote.State).Client.(*remoteClient).operationTimeout != 5*time.Second {
		t.Fatal("configured operation timeout was lost")
	}
}

func TestStateInitializationRechecksAfterAcquiringLock(t *testing.T) {
	store := newMockObjectStore()
	otherClient := testRemoteClient(store)
	otherClient.stateFilePath = "state"
	otherManager := remote.NewState(otherClient, encryption.StateEncryptionDisabled())
	if err := otherManager.WriteState(states.NewState()); err != nil {
		t.Fatal(err)
	}
	if err := otherManager.PersistState(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	existing := store.objects["state"]
	delete(store.objects, "state")
	store.afterPut = func(store *mockObjectStore, name string) {
		if name == "state.lock" {
			store.objects["state"] = existing
		}
	}
	b := &Backend{storageClient: store, key: "state", encryption: encryption.StateEncryptionDisabled()}
	manager, err := b.StateMgr(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if manager.(statemgr.PersistentMeta).StateSnapshotMeta().Lineage != otherManager.StateSnapshotMeta().Lineage {
		t.Fatal("racing initialized state was replaced")
	}
}

func TestDeleteWorkspaceUsesTheLockedClientAndLeavesUnlockToCaller(t *testing.T) {
	store := newMockObjectStore()
	b := &Backend{storageClient: store, key: "state", workspaceKeyPrefix: "env:", encryption: encryption.StateEncryptionDisabled()}
	manager, err := b.StateMgr(t.Context(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Lock(t.Context(), statemgr.NewLockInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteWorkspace(t.Context(), "dev", true); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.objects["env:/dev/state"]; exists {
		t.Fatal("workspace not deleted")
	}
	if _, exists := store.objects["env:/dev/state.lock"]; !exists {
		t.Fatal("delete removed caller's lock")
	}
	if err := manager.Unlock(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteWorkspace(t.Context(), backend.DefaultStateName, true); err == nil {
		t.Fatal("default workspace deleted")
	}
}

func TestWorkspaceDeletionReacquiresLockAfterCLIInspection(t *testing.T) {
	store := newMockObjectStore()
	b := &Backend{storageClient: store, key: "state", workspaceKeyPrefix: "env:", encryption: encryption.StateEncryptionDisabled()}
	manager, err := b.StateMgr(t.Context(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Lock(t.Context(), statemgr.NewLockInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RefreshState(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Unlock(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	other := newRemoteClient(store, "", "", "env:/dev/state", true, "")
	id, err = other.Lock(t.Context(), statemgr.NewLockInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteWorkspace(t.Context(), "dev", true); err == nil {
		t.Fatal("deleted another actor's locked workspace")
	}
	if _, exists := store.objects["env:/dev/state"]; !exists {
		t.Fatal("conflict removed state")
	}
	if err := other.Unlock(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteWorkspace(t.Context(), "dev", true); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatal("deletion leaked lock or state")
	}
}

func TestWorkspaceDeletionDoesNotRefreshInspectedBaseline(t *testing.T) {
	store := newMockObjectStore()
	b := &Backend{storageClient: store, key: "state", workspaceKeyPrefix: "env:", encryption: encryption.StateEncryptionDisabled()}
	if _, err := b.StateMgr(t.Context(), "dev"); err != nil {
		t.Fatal(err)
	}
	store.set("env:/dev/state", []byte("new resources after inspection"), "another-writer")
	if err := b.DeleteWorkspace(t.Context(), "dev", false); !errors.Is(err, errObjectChanged) {
		t.Fatalf("stale deletion did not fail: %v", err)
	}
	if _, exists := store.objects["env:/dev/state.lock"]; exists {
		t.Fatal("failed deletion leaked lock")
	}
	if string(store.objects["env:/dev/state"].data) != "new resources after inspection" {
		t.Fatal("new state deleted")
	}
}

type workspaceObjectStore struct{ *mockObjectStore }

func (s workspaceObjectStore) ListObjects(ctx context.Context, request objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	if err := ctx.Err(); err != nil {
		return objectstorage.ListObjectsResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for name := range s.objects {
		if request.Prefix == nil || strings.HasPrefix(name, *request.Prefix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return objectstorage.ListObjectsResponse{ListObjects: objectstorage.ListObjects{Objects: objectNames(names...)}}, nil
}

// Use OpenTofu's existing interface conformance helpers directly rather than
// reproducing their state and force-unlock contract tests.
func TestBackendInterfaceConformance(t *testing.T) {
	store := workspaceObjectStore{newMockObjectStore()}
	newBackend := func() *Backend {
		return &Backend{storageClient: store, namespace: "n", bucketName: "b", key: "state", workspaceKeyPrefix: "env:", encryption: encryption.StateEncryptionDisabled()}
	}
	backend.TestBackendStates(t, newBackend())
	backend.TestBackendStateLocks(t, newBackend(), newBackend())
	backend.TestBackendStateForceUnlockInWS(t, newBackend(), newBackend(), "force-unlock")
}
