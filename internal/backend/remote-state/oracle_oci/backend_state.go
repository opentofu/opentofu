// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oracle_oci

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

const errStateUnlock = `
Error unlocking oci state. Lock ID: %s

Error: %s

You may have to force-unlock this state in order to use it again.
`

func (b *Backend) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	b.client.path = b.path(name)
	b.client.lockFilePath = b.getLockFilePath(name)
	stateMgr := remote.NewState(&RemoteClient{
		objectStorageClient: b.client.objectStorageClient,
		bucketName:          b.bucket,
		path:                b.path(name),
		lockFilePath:        b.getLockFilePath(name),
		namespace:           b.namespace,
		kmsKeyID:            b.kmsKeyID,

		SSECustomerKey:       b.SSECustomerKey,
		SSECustomerKeySHA256: b.SSECustomerKeySHA256,
		SSECustomerAlgorithm: b.SSECustomerAlgorithm,
		ctx:                  ctx,
	}, b.encryption)

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
		lockId, err := b.client.Lock(ctx, lockInfo)
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

		// Grab the value
		if err := stateMgr.RefreshState(context.TODO()); err != nil {
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
			if err := stateMgr.PersistState(context.TODO(), nil); err != nil {
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
func (b *Backend) configureRemoteClient(ctx context.Context) error {

	configProvider, err := b.configProvider.getSdkConfigProvider()
	if err != nil {
		return err
	}

	client, err := buildConfigureClient(configProvider, buildHttpClient())
	if err != nil {
		return err
	}

	b.client = &RemoteClient{
		objectStorageClient: client,
		bucketName:          b.bucket,
		namespace:           b.namespace,
		kmsKeyID:            b.kmsKeyID,

		SSECustomerKey:       b.SSECustomerKey,
		SSECustomerKeySHA256: b.SSECustomerKeySHA256,
		SSECustomerAlgorithm: b.SSECustomerAlgorithm,
		ctx:                  ctx,
	}
	return nil
}

func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	logger := logWithOperation("listWorkspaces")
	const maxKeys = 1000

	wss := []string{backend.DefaultStateName}
	start := common.String("")
	if b.client == nil {
		err := b.configureRemoteClient(ctx)
		if err != nil {
			return nil, err
		}
	}
	for {
		listObjectReq := objectstorage.ListObjectsRequest{
			BucketName:    common.String(b.bucket),
			NamespaceName: common.String(b.namespace),
			Prefix:        common.String(b.workspaceKeyPrefix),
			Start:         start,
			Limit:         common.Int(maxKeys),
		}
		listObjectResponse, err := b.client.objectStorageClient.ListObjects(ctx, listObjectReq)
		if err != nil {
			logger.Error("Failed to list workspaces in Object Storage backend: %v", err)
			return nil, err
		}

		for _, object := range listObjectResponse.Objects {
			key := *object.Name
			if strings.HasPrefix(key, b.workspaceKeyPrefix) && strings.HasSuffix(key, b.key) {
				name := strings.TrimPrefix(key, b.workspaceKeyPrefix+"/")
				name = strings.TrimSuffix(name, b.key)
				name = strings.TrimSuffix(name, "/")

				if name != "" {
					wss = append(wss, name)
				}
			}
		}
		if len(listObjectResponse.Objects) < maxKeys {
			break
		}
		start = listObjectResponse.NextStartWith

	}

	return uniqueStrings(wss), nil
}

func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {

	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}
	if b.client == nil {
		err := b.configureRemoteClient(ctx)
		if err != nil {
			return err
		}
	}

	b.client.path = b.path(name)
	b.client.lockFilePath = b.getLockFilePath(name)
	return b.client.Delete(ctx)

}
