// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-uuid"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/tombuildsstuff/giovanni/storage/2018-11-09/blob/blobs"
)

const (
	// Must be lower case
	lockInfoMetaKey = "terraformlockid"
)

type RemoteClient struct {
	giovanniBlobClient blobs.Client
	accountName        string
	containerName      string
	keyName            string
	leaseID            *string
	snapshot           bool
	timeoutSeconds     int
}

func (c *RemoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	// Get should time out after the timeoutSeconds
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	blob, err := c.giovanniBlobClient.Get(ctx, c.accountName, c.containerName, c.keyName, blobs.GetInput{LeaseID: c.leaseID})
	if err != nil {
		if blob.Response.IsHTTPStatus(http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}

	payload := &remote.Payload{
		Data: blob.Contents,
	}

	// If there was no data, then return nil
	if len(payload.Data) == 0 {
		return nil, nil
	}

	return payload, nil
}

func (c *RemoteClient) Put(ctx context.Context, data []byte) error {
	if c.snapshot {
		snapshotInput := blobs.SnapshotInput{LeaseID: c.leaseID}
		log.Printf("[DEBUG] Snapshotting existing Blob %q (Container %q / Account %q)", c.keyName, c.containerName, c.accountName)
		if _, err := c.giovanniBlobClient.Snapshot(ctx, c.accountName, c.containerName, c.keyName, snapshotInput); err != nil {
			return fmt.Errorf("error snapshotting Blob %q (Container %q / Account %q): %w", c.keyName, c.containerName, c.accountName, err)
		}

		log.Print("[DEBUG] Created blob snapshot")
	}

	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		if properties.StatusCode != http.StatusNotFound {
			return err
		}
	}

	contentType := "application/json"
	putOptions := blobs.PutBlockBlobInput{
		LeaseID:     c.leaseID,
		Content:     &data,
		ContentType: &contentType,
		MetaData:    properties.MetaData,
	}
	_, err = c.giovanniBlobClient.PutBlockBlob(ctx, c.accountName, c.containerName, c.keyName, putOptions)

	return err
}

func (c *RemoteClient) Delete(ctx context.Context) error {
	resp, err := c.giovanniBlobClient.Delete(ctx, c.accountName, c.containerName, c.keyName, blobs.DeleteInput{LeaseID: c.leaseID})
	if err != nil {
		if !resp.IsHTTPStatus(http.StatusNotFound) {
			return err
		}
	}
	return nil
}

func (c *RemoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	stateName := fmt.Sprintf("%s/%s", c.containerName, c.keyName)
	info.Path = stateName

	if info.ID == "" {
		lockID, err := uuid.GenerateUUID()
		if err != nil {
			return "", err
		}

		info.ID = lockID
	}

	getLockInfoErr := func(err error) error {
		lockInfo, infoErr := c.getLockInfo(ctx)
		if infoErr != nil {
			err = multierror.Append(err, infoErr)
		}

		return &statemgr.LockError{
			Err:  err,
			Info: lockInfo,
		}
	}

	leaseOptions := blobs.AcquireLeaseInput{
		ProposedLeaseID: &info.ID,
		LeaseDuration:   -1,
	}

	// obtain properties to see if the blob lease is already in use. If the blob doesn't exist, create it
	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		// error if we had issues getting the blob
		if !properties.Response.IsHTTPStatus(http.StatusNotFound) {
			return "", getLockInfoErr(err)
		}
		// if we don't find the blob, we need to build it
		contentType := "application/json"
		putGOptions := blobs.PutBlockBlobInput{
			ContentType: &contentType,
		}

		_, err = c.giovanniBlobClient.PutBlockBlob(ctx, c.accountName, c.containerName, c.keyName, putGOptions)
		if err != nil {
			return "", getLockInfoErr(err)
		}
	}

	// if the blob is already locked then error
	if properties.LeaseStatus == blobs.Locked {
		return "", getLockInfoErr(fmt.Errorf("state blob is already locked"))
	}

	leaseID, err := c.giovanniBlobClient.AcquireLease(ctx, c.accountName, c.containerName, c.keyName, leaseOptions)
	if err != nil {
		return "", getLockInfoErr(err)
	}

	info.ID = leaseID.LeaseID
	c.setLeaseID(leaseID.LeaseID)

	if err := c.writeLockInfo(ctx, info); err != nil {
		return "", err
	}

	return info.ID, nil
}

func (c *RemoteClient) getLockInfo(ctx context.Context) (*statemgr.LockInfo, error) {
	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		return nil, err
	}

	raw := properties.MetaData[lockInfoMetaKey]
	if raw == "" {
		return nil, fmt.Errorf("blob metadata %q was empty", lockInfoMetaKey)
	}

	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	lockInfo := &statemgr.LockInfo{}
	err = json.Unmarshal(data, lockInfo)
	if err != nil {
		return nil, err
	}

	return lockInfo, nil
}

// writes info to blob meta data, deletes metadata entry if info is nil
func (c *RemoteClient) writeLockInfo(ctx context.Context, info *statemgr.LockInfo) error {
	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		return err
	}

	if info == nil {
		delete(properties.MetaData, lockInfoMetaKey)
	} else {
		value := base64.StdEncoding.EncodeToString(info.Marshal())
		properties.MetaData[lockInfoMetaKey] = value
	}

	opts := blobs.SetMetaDataInput{
		LeaseID:  c.leaseID,
		MetaData: properties.MetaData,
	}

	_, err = c.giovanniBlobClient.SetMetaData(ctx, c.accountName, c.containerName, c.keyName, opts)
	return err
}

func (c *RemoteClient) Unlock(ctx context.Context, id string) error {
	lockErr := &statemgr.LockError{}

	lockInfo, err := c.getLockInfo(ctx)
	if err != nil {
		lockErr.Err = fmt.Errorf("failed to retrieve lock info: %w", err)
		return lockErr
	}
	lockErr.Info = lockInfo

	if lockInfo.ID != id {
		lockErr.Err = fmt.Errorf("lock id %q does not match existing lock", id)
		return lockErr
	}

	c.setLeaseID(lockInfo.ID)
	if err := c.writeLockInfo(ctx, nil); err != nil {
		lockErr.Err = fmt.Errorf("failed to delete lock info from metadata: %w", err)
		return lockErr
	}

	_, err = c.giovanniBlobClient.ReleaseLease(ctx, c.accountName, c.containerName, c.keyName, id)
	if err != nil {
		lockErr.Err = err
		return lockErr
	}

	c.leaseID = nil

	return nil
}

// getBlobProperties wraps the GetProperties method of the giovanniBlobClient with timeout
func (c *RemoteClient) getBlobProperties(ctx context.Context) (blobs.GetPropertiesResult, error) {
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	return c.giovanniBlobClient.GetProperties(ctx, c.accountName, c.containerName, c.keyName, blobs.GetPropertiesInput{LeaseID: c.leaseID})
}

// getContextWithTimeout returns a context with timeout based on the timeoutSeconds
func (c *RemoteClient) getContextWithTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(c.timeoutSeconds)*time.Second)
}

// setLeaseID takes a string leaseID and sets the leaseID field of the RemoteClient
// if passed leaseID is empty, it sets the leaseID field to nil
func (c *RemoteClient) setLeaseID(leaseID string) {
	if leaseID == "" {
		c.leaseID = nil
	} else {
		c.leaseID = &leaseID
	}
}
