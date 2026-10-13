// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"context"
	"os"
	"testing"
	"time"

	uuid "github.com/hashicorp/go-uuid"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// This is the only live test. It deliberately requires explicit opt-in and an
// existing bucket. Authentication is selected independently through OCI/TF_VAR
// environment variables, so the same test can be run on each identity runtime.
// It creates and deletes objects under a fresh UUID prefix, never the bucket.
func TestAccOCIObjectStorage(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("live OCI validation requires TF_ACC=1")
	}
	namespace, bucket := os.Getenv("TF_OCI_NAMESPACE"), os.Getenv("TF_OCI_BUCKET")
	if namespace == "" || bucket == "" {
		t.Fatal("set TF_OCI_NAMESPACE and TF_OCI_BUCKET to an existing test bucket")
	}
	id, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	prefix := "opentofu-backend-acceptance/" + id
	b := New(encryption.StateEncryptionDisabled()).(*Backend)
	config, diags := b.PrepareConfig(backendConfig(t, b, map[string]string{
		"namespace": namespace, "bucket": bucket, "key": prefix + "/state",
		"workspace_key_prefix": prefix + "/workspaces",
	}))
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if diags := b.Configure(t.Context(), config); diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	c := newRemoteClient(b.storageClient, namespace, bucket, b.key, true, "")
	other := newRemoteClient(b.storageClient, namespace, bucket, b.key, true, "")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, client := range []*remoteClient{c, other} {
			client.mu.Lock()
			lockID := client.heldLockID
			client.mu.Unlock()
			if lockID != "" {
				if err := client.Unlock(ctx, lockID); err != nil {
					t.Errorf("acceptance lock cleanup: %s", err)
					return
				}
			}
		}
		// Preserve conditional state cleanup, including on a fatal lock-test
		// failure. Never delete beneath an actor that acquired a new lock.
		lockID, err := c.Lock(ctx, statemgr.NewLockInfo())
		if err != nil {
			t.Errorf("acceptance cleanup lock: %s", err)
			return
		}
		if err := c.Delete(ctx); err != nil {
			t.Errorf("acceptance state cleanup: %s", err)
		}
		if err := c.Unlock(ctx, lockID); err != nil {
			t.Errorf("acceptance cleanup unlock: %s", err)
		}
	})
	remote.TestClient(t, c)
	remote.TestRemoteLocks(t, c, other)

	manager, err := b.StateMgr(t.Context(), "acceptance")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		lockID, err := manager.Lock(ctx, statemgr.NewLockInfo())
		if err != nil {
			t.Errorf("workspace cleanup lock: %s", err)
			return
		}
		if err := b.DeleteWorkspace(ctx, "acceptance", true); err != nil {
			t.Errorf("workspace cleanup: %s", err)
		}
		if err := manager.Unlock(ctx, lockID); err != nil {
			t.Errorf("workspace cleanup unlock: %s", err)
		}
	})
	names, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "default" || names[1] != "acceptance" {
		t.Fatalf("unexpected live workspace listing: %v", names)
	}
}
