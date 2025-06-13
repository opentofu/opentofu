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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

const (
	// This will be used as directory name, the odd looking colon is simply to
	// reduce the chance of name conflicts with existing objects.
	keyEnvPrefix = "env:"
)

// getContextWithTimeout returns a context with timeout based on the timeoutSeconds
func (b *Backend) getContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, b.timeout)
}

func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	ctx, cancel := b.getContextWithTimeout(ctx)
	defer cancel()

	prefix := b.keyName + keyEnvPrefix
	result, err := getPaginatedResults(ctx, b.containerClient, prefix)
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
	blobClient := b.containerClient.NewBlockBlobClient(b.path(name))

	if _, err := blobClient.Delete(ctx, nil); err != nil {
		if !notFoundError(err) {
			return fmt.Errorf("error deleting blob: %w", err)
		}
	}

	return nil
}

func (b *Backend) StateMgr(_ context.Context, name string) (statemgr.Full, error) {
	blobClient := b.containerClient.NewBlockBlobClient(b.path(name))

	client := &RemoteClient{
		blobClient: blobClient,
		snapshot:   b.snapshot,
		timeout:    b.timeout,
	}

	stateMgr := remote.NewState(client, b.encryption)

	// Grab the value
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		return nil, err
	}
	//if this isn't the default state name, we need to create the object so
	//it's listed by States.
	if v := stateMgr.State(); v == nil {
		// take a lock on this state while we write it
		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockId, err := client.Lock(context.TODO(), lockInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to lock azure state: %w", err)
		}

		// Local helper function so we can call it multiple places
		lockUnlock := func(parent error) error {
			if err := stateMgr.Unlock(context.TODO(), lockId); err != nil {
				return fmt.Errorf(strings.TrimSpace(errStateUnlock), lockId, err)
			}
			return parent
		}

		if err := stateMgr.WriteState(states.NewState()); err != nil {
			err = lockUnlock(err)
			return nil, err
		}
		if err := stateMgr.PersistState(context.TODO(), nil); err != nil {
			err = lockUnlock(err)
			return nil, err
		}

		// Unlock, the state should now be initialized
		if err := lockUnlock(nil); err != nil {
			return nil, err
		}
	}

	return stateMgr, nil
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
	NewListBlobsFlatPager(o *container.ListBlobsFlatOptions) *runtime.Pager[container.ListBlobsFlatResponse]
}

func getPaginatedResults(ctx context.Context, client azureClient, prefix string) ([]string, error) {
	count := 1
	initialMarker := ""

	params := container.ListBlobsFlatOptions{
		Prefix: &prefix,
		Marker: &initialMarker,
	}
	result := []string{backend.DefaultStateName}
	pager := client.NewListBlobsFlatPager(&params)

	for pager.More() {
		log.Printf("[TRACE] Getting page %d of blob results", count)

		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("error listing blobs: %w", err)
		}

		for _, obj := range resp.Segment.BlobItems {
			key := obj.Name
			if !strings.HasPrefix(*key, prefix) {
				continue
			}

			name := strings.TrimPrefix(*key, prefix)
			// we store the state in a key, not a directory
			if strings.Contains(name, "/") {
				continue
			}
			result = append(result, name)
		}

		count++
	}

	sort.Strings(result[1:])
	return result, nil
}
