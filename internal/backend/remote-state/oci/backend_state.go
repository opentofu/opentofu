// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func validWorkspace(name string) bool {
	return name != "" && name != "." && name != ".." && name == url.PathEscape(name)
}

func (b *Backend) stateFile(name string) string {
	if name == backend.DefaultStateName {
		return b.key
	}
	if b.workspaceKeyPrefix == "" {
		return name + "/" + b.key
	}
	return b.workspaceKeyPrefix + "/" + name + "/" + b.key
}

func (b *Backend) workspaceForObject(object string) string {
	prefix := b.workspaceKeyPrefix
	if prefix != "" {
		prefix += "/"
	}
	if !strings.HasPrefix(object, prefix) {
		return ""
	}
	name, suffix, ok := strings.Cut(strings.TrimPrefix(object, prefix), "/")
	if !ok || suffix != b.key || !validWorkspace(name) || name == backend.DefaultStateName {
		return ""
	}
	return name
}

func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	if b.storageClient == nil {
		return nil, fmt.Errorf("oracle-oci backend has not been configured")
	}
	timeout := b.requestTimeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	prefix := b.workspaceKeyPrefix
	if prefix != "" {
		prefix += "/"
	}
	policy := common.NewRetryPolicyWithOptions(common.WithMaximumNumberAttempts(4), common.WithExponentialBackoff(5*time.Second, 2))
	request := objectstorage.ListObjectsRequest{
		NamespaceName: common.String(b.namespace), BucketName: common.String(b.bucketName),
		Prefix: common.String(prefix), Limit: common.Int(1000), Fields: common.String("name"),
		RequestMetadata: common.RequestMetadata{RetryPolicy: &policy},
	}
	seen := map[string]bool{backend.DefaultStateName: true}
	pages := map[string]bool{}
	for {
		response, err := b.storageClient.ListObjects(ctx, request)
		if err != nil {
			return nil, sanitizedOCIError("list workspaces", err)
		}
		for _, object := range response.Objects {
			if object.Name != nil {
				if name := b.workspaceForObject(*object.Name); name != "" {
					seen[name] = true
				}
			}
		}
		if response.NextStartWith == nil || *response.NextStartWith == "" {
			break
		}
		if pages[*response.NextStartWith] {
			return nil, fmt.Errorf("OCI workspace listing returned a repeated pagination marker")
		}
		pages[*response.NextStartWith] = true
		request.Start = response.NextStartWith
	}
	names := []string{backend.DefaultStateName}
	for name := range seen {
		if name != backend.DefaultStateName {
			names = append(names, name)
		}
	}
	sort.Strings(names[1:])
	return names, nil
}

// A cached manager keeps DeleteWorkspace on the same conditional state client
// that the caller has already refreshed and locked.
func (b *Backend) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	if !validWorkspace(name) {
		return nil, fmt.Errorf("invalid OCI workspace name")
	}
	if b.storageClient == nil {
		return nil, fmt.Errorf("oracle-oci backend has not been configured")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if manager := b.managers[name]; manager != nil {
		return manager, nil
	}
	client := newRemoteClient(b.storageClient, b.namespace, b.bucketName, b.stateFile(name), true, b.kmsKeyID)
	if b.requestTimeout > 0 {
		client.operationTimeout = b.requestTimeout
	}
	manager := remote.NewState(client, b.encryption)
	if err := manager.RefreshState(ctx); err != nil {
		return nil, err
	}
	if manager.State() == nil {
		info := statemgr.NewLockInfo()
		info.Operation = "init"
		id, err := manager.Lock(ctx, info)
		if err != nil {
			return nil, err
		}
		initializeErr := func() error {
			// Another initializer could have created state between our read
			// and acquiring the lock. Preserve its lineage and contents.
			if err := manager.RefreshState(ctx); err != nil {
				return err
			}
			if manager.State() != nil {
				return nil
			}
			if err := manager.WriteState(states.NewState()); err != nil {
				return err
			}
			return manager.PersistState(ctx, nil)
		}()
		cleanupTimeout := b.requestTimeout
		if cleanupTimeout <= 0 {
			cleanupTimeout = time.Minute
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		unlockErr := manager.Unlock(cleanupCtx, id)
		cancel()
		if unlockErr != nil {
			unlockErr = fmt.Errorf("OCI initialization lock could not be released; inspect it and use tofu force-unlock with lock ID %s if its owner has stopped: %w", id, unlockErr)
		}
		if err := errors.Join(initializeErr, unlockErr); err != nil {
			return nil, err
		}
	}
	if b.managers == nil {
		b.managers = make(map[string]*remote.State)
	}
	b.managers[name] = manager
	return manager, nil
}

func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if !validWorkspace(name) || name == backend.DefaultStateName {
		return fmt.Errorf("cannot delete the default or an invalid OCI workspace")
	}
	if b.storageClient == nil {
		return fmt.Errorf("oracle-oci backend has not been configured")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var client *remoteClient
	if manager := b.managers[name]; manager != nil {
		client = manager.Client.(*remoteClient)
	} else {
		client = newRemoteClient(b.storageClient, b.namespace, b.bucketName, b.stateFile(name), true, b.kmsKeyID)
		if b.requestTimeout > 0 {
			client.operationTimeout = b.requestTimeout
		}
	}
	// The workspace-delete command releases its inspection lock before this
	// call. Reacquire it without refreshing the inspected state baseline, so
	// neither another lock holder nor an intervening state update is ignored.
	// A caller that already holds this client's lock retains responsibility
	// for releasing it.
	client.mu.Lock()
	callerHoldsLock := client.heldLockID != ""
	client.mu.Unlock()
	var deletionLockID string
	if !callerHoldsLock {
		info := statemgr.NewLockInfo()
		info.Operation = "workspace-delete"
		var err error
		deletionLockID, err = client.Lock(ctx, info)
		if err != nil {
			return err
		}
	}
	deleteErr := client.Delete(ctx)
	if deleteErr == nil {
		delete(b.managers, name)
	}
	if deletionLockID == "" {
		return deleteErr
	}
	timeout := b.requestTimeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	unlockErr := client.Unlock(cleanupCtx, deletionLockID)
	cancel()
	if unlockErr != nil {
		unlockErr = fmt.Errorf("OCI workspace deletion lock could not be released; inspect lock ID %s before recovery: %w", deletionLockID, unlockErr)
	}
	return errors.Join(deleteErr, unlockErr)
}
