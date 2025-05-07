// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/tombuildsstuff/giovanni/storage/2018-11-09/blob/blobs"
	"github.com/tombuildsstuff/giovanni/storage/2018-11-09/blob/containers"
)

const (
	// This will be used as directory name, the odd looking colon is simply to
	// reduce the chance of name conflicts with existing objects.
	keyEnvPrefix = "env:"
)

// getContextWithTimeout returns a context with timeout based on the timeoutSeconds
func (b *Backend) getContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(b.armClient.timeoutSeconds)*time.Second)
}

func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	ctx, cancel := b.getContextWithTimeout(ctx)
	defer cancel()

	client, err := b.armClient.getContainersClient(ctx)
	if err != nil {
		return nil, err
	}

	prefix := b.keyName + keyEnvPrefix
	result, err := getPaginatedResults(ctx, client, prefix, b.armClient.storageAccountName, b.containerName)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	ctx, cancel := b.getContextWithTimeout(ctx)
	defer cancel()
	client, err := b.armClient.getBlobClient(ctx)
	if err != nil {
		return err
	}

	if resp, err := client.Delete(ctx, b.armClient.storageAccountName, b.containerName, b.path(name), blobs.DeleteInput{}); err != nil {
		if resp.Response.StatusCode != 404 {
			return err
		}
	}

	return nil
}

func (b *Backend) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	ctx, cancel := b.getContextWithTimeout(ctx)
	defer cancel()

	blobClient, err := b.armClient.getBlobClient(ctx)
	if err != nil {
		return nil, err
	}

	client := &RemoteClient{
		giovanniBlobClient: *blobClient,
		containerName:      b.containerName,
		keyName:            b.path(name),
		accountName:        b.accountName,
		snapshot:           b.snapshot,
		timeoutSeconds:     b.armClient.timeoutSeconds,
	}

	stateMgr := remote.NewState(client, b.encryption)

	// Grab the value
	if err := stateMgr.RefreshState(); err != nil {
		return nil, err
	}
	//if this isn't the default state name, we need to create the object so
	//it's listed by States.
	if v := stateMgr.State(); v == nil {
		// take a lock on this state while we write it
		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockId, err := client.Lock(lockInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to lock azure state: %w", err)
		}

		// Local helper function so we can call it multiple places
		lockUnlock := func(parent error) error {
			if err := stateMgr.Unlock(lockId); err != nil {
				return fmt.Errorf(strings.TrimSpace(errStateUnlock), lockId, err)
			}
			return parent
		}

		// Grab the value
		if err := stateMgr.RefreshState(); err != nil {
			err = lockUnlock(err)
			return nil, err
		}
		//if this isn't the default state name, we need to create the object so
		//it's listed by States.
		if v := stateMgr.State(); v == nil {
			// If we have no state, we have to create an empty state
			if err := stateMgr.WriteState(states.NewState()); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
			if err := stateMgr.PersistState(nil); err != nil {
				err = lockUnlock(err)
				return nil, err
			}

			// Unlock, the state should now be initialized
			if err := lockUnlock(nil); err != nil {
				return nil, err
			}
		}
	}

	return stateMgr, nil
}

func (b *Backend) client() *RemoteClient {
	return &RemoteClient{}
}

func (b *Backend) path(name string) string {
	if name == backend.DefaultStateName {
		return b.keyName
	}

	return b.keyName + keyEnvPrefix + name
}

const errStateUnlock = `
Error unlocking Azure state. Lock ID: %s

Error: %w

You may have to force-unlock this state in order to use it again.
`

type azureClient interface {
	ListBlobs(ctx context.Context, accountName, containerName string, input containers.ListBlobsInput) (result containers.ListBlobsResult, err error)
}

func getPaginatedResults(ctx context.Context, client azureClient, prefix, accName, containerName string) ([]string, error) {
	count := 1
	initialMarker := ""

	params := containers.ListBlobsInput{
		Prefix: &prefix,
		Marker: &initialMarker,
	}
	result := []string{backend.DefaultStateName}

	for {
		log.Printf("[TRACE] Getting page %d of blob results", count)
		resp, err := client.ListBlobs(ctx, accName, containerName, params)
		if err != nil {
			return nil, err
		}

		// Used to paginate blobs, saving the NextMarker result from ListBlobs
		params.Marker = resp.NextMarker
		for _, obj := range resp.Blobs.Blobs {
			key := obj.Name
			if !strings.HasPrefix(key, prefix) {
				continue
			}

			name := strings.TrimPrefix(key, prefix)
			// we store the state in a key, not a directory
			if strings.Contains(name, "/") {
				continue
			}
			result = append(result, name)
		}

		count++
		if params.Marker == nil || *params.Marker == "" {
			break
		}
	}

	sort.Strings(result[1:])
	return result, nil
}
