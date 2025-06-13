// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/lease"
	"github.com/hashicorp/go-uuid"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

const (
	lockInfoMetaKey = "terraformlockid"
)

type RemoteClient struct {
	blobClient *blockblob.Client
	leaseID    *string
	snapshot   bool
	timeout    time.Duration
}

func (c *RemoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	// Get should time out after the timeoutSeconds
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	resp, err := c.blobClient.DownloadStream(ctx, &blob.DownloadStreamOptions{
		AccessConditions: c.leaseAccessCondition(),
	})
	if err != nil {
		if notFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error downloading azure blob: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading azure blob: %w", err)
	}

	payload := &remote.Payload{
		Data: data,
	}

	// If there was no data, then return nil
	if len(data) == 0 {
		return nil, nil
	}

	return payload, nil
}

func (c *RemoteClient) Put(ctx context.Context, data []byte) error {
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	if c.snapshot {
		snapshotInput := &blob.CreateSnapshotOptions{AccessConditions: c.leaseAccessCondition()}
		log.Printf("[DEBUG] Snapshotting existing Blob %s", c.blobClient.URL())
		if _, err := c.blobClient.CreateSnapshot(ctx, snapshotInput); err != nil {
			return fmt.Errorf("error snapshotting Blob %s: %w", c.blobClient.URL(), err)
		}

		log.Print("[DEBUG] Created blob snapshot")
	}

	properties, err := c.getBlobProperties(ctx)
	if err != nil && !notFoundError(err) {
		return fmt.Errorf("error getting blob properties while doing Put: %w", err)
	}

	putOptions := &blockblob.UploadBufferOptions{
		Metadata:         properties.Metadata,
		AccessConditions: c.leaseAccessCondition(),
		HTTPHeaders:      httpHeaders(),
	}
	_, err = c.blobClient.UploadBuffer(ctx, data, putOptions)
	if err != nil {
		err = fmt.Errorf("error uploading blob: %w", err)
	}

	return err
}

func (c *RemoteClient) Delete(ctx context.Context) error {
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	_, err := c.blobClient.Delete(ctx, &blob.DeleteOptions{AccessConditions: c.leaseAccessCondition()})
	if err != nil && !notFoundError(err) {
		return fmt.Errorf("error deleting blob: %w", err)
	}
	return nil
}

func (c *RemoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	info.Path = c.blobClient.URL()

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
			err = errors.Join(err, infoErr)
		}

		return &statemgr.LockError{
			Err:  err,
			Info: lockInfo,
		}
	}

	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()

	// obtain properties to see if the blob lease is already in use. If the blob doesn't exist, create it
	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		// error if we had issues getting the blob
		if !notFoundError(err) {
			return "", fmt.Errorf("error getting blob properties while doing Lock: %w", err)
		}
		// if we don't find the blob, we need to build it
		_, err = c.blobClient.UploadBuffer(ctx, []byte{}, &blockblob.UploadBufferOptions{
			HTTPHeaders: httpHeaders(),
		})

		if err != nil {
			return "", getLockInfoErr(err)
		}
	}

	// if the blob is already locked then error
	if properties.LeaseStatus != nil && *properties.LeaseStatus == lease.StatusTypeLocked {
		return "", getLockInfoErr(fmt.Errorf("state blob is already locked"))
	}

	leaseOptions := &lease.BlobClientOptions{
		LeaseID: &info.ID,
	}

	leaseClient, err := lease.NewBlobClient(c.blobClient, leaseOptions)
	if err != nil {
		return "", fmt.Errorf("error getting blob lease client: %w", err)
	}
	leaseResp, err := leaseClient.AcquireLease(ctx, -1, nil)

	if err != nil {
		return "", getLockInfoErr(err)
	}

	info.ID = *leaseResp.LeaseID
	c.setLeaseID(leaseResp.LeaseID)

	if err := c.writeLockInfo(ctx, info); err != nil {
		return "", err
	}

	return info.ID, nil
}

func (c *RemoteClient) getLockInfo(ctx context.Context) (*statemgr.LockInfo, error) {
	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting lock info: %w", err)
	}

	raw := properties.Metadata[lockInfoMetaKey]
	if raw == nil || *raw == "" {
		return nil, fmt.Errorf("blob metadata %q was empty", lockInfoMetaKey)
	}

	data, err := base64.StdEncoding.DecodeString(*raw)
	if err != nil {
		return nil, fmt.Errorf("error in base64 decoding lock string: %w", err)
	}

	lockInfo := &statemgr.LockInfo{}
	err = json.Unmarshal(data, lockInfo)
	if err != nil {
		return nil, fmt.Errorf("error decoding json data from lock: %w", err)
	}

	return lockInfo, nil
}

// writes info to blob meta data, deletes metadata entry if info is nil
func (c *RemoteClient) writeLockInfo(ctx context.Context, info *statemgr.LockInfo) error {
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	properties, err := c.getBlobProperties(ctx)
	if err != nil {
		return fmt.Errorf("error getting blob properties while writing lock: %w", err)
	}

	if info == nil {
		delete(properties.Metadata, lockInfoMetaKey)
	} else {
		value := base64.StdEncoding.EncodeToString(info.Marshal())
		properties.Metadata[lockInfoMetaKey] = &value
	}

	_, err = c.blobClient.SetMetadata(ctx, properties.Metadata, &blob.SetMetadataOptions{
		AccessConditions: c.leaseAccessCondition(),
	})

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

	c.setLeaseID(&lockInfo.ID)
	if err := c.writeLockInfo(ctx, nil); err != nil {
		lockErr.Err = fmt.Errorf("failed to delete lock info from metadata: %w", err)
		return lockErr
	}

	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()

	leaseOptions := &lease.BlobClientOptions{
		LeaseID: c.leaseID,
	}
	leaseClient, err := lease.NewBlobClient(c.blobClient, leaseOptions)
	if err != nil {
		lockErr.Err = fmt.Errorf("error getting blob lease client: %w", err)
		return lockErr
	}

	_, err = leaseClient.ReleaseLease(ctx, nil)
	if err != nil {
		lockErr.Err = fmt.Errorf("error when releasing lease for azure lock: %w", err)
		return lockErr
	}

	c.leaseID = nil

	return nil
}

// getBlobProperties wraps the GetProperties method of the blobClient with timeout.
// This method ensures the Metadata property of the response is set to a non-nil map.
func (c *RemoteClient) getBlobProperties(ctx context.Context) (blob.GetPropertiesResponse, error) {
	ctx, ctxCancel := c.getContextWithTimeout(ctx)
	defer ctxCancel()
	resp, err := c.blobClient.GetProperties(ctx, &blob.GetPropertiesOptions{AccessConditions: c.leaseAccessCondition()})
	if err == nil {
		resp.Metadata = fixMetadata(resp.Metadata)
	}
	return resp, err
}

// fixMetadata ensures the Metadata property of the response is set to a non-nil map.
// It also lower-cases all existing metadata headers to keep it backwards-compatible with the metadata stored by the Giovanni client
// which was used in the previous version of the azurerm backend.
func fixMetadata(metadata map[string]*string) map[string]*string {
	output := make(map[string]*string)
	if metadata == nil {
		return output
	}
	for k, v := range metadata {
		output[strings.ToLower(k)] = v
	}
	return output
}

// getContextWithTimeout returns a context with timeout based on the timeoutSeconds
func (c *RemoteClient) getContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// setLeaseID takes a string leaseID and sets the leaseID field of the RemoteClient
// if passed leaseID is empty, it sets the leaseID field to nil
func (c *RemoteClient) setLeaseID(leaseID *string) {
	if leaseID == nil || *leaseID == "" {
		c.leaseID = nil
	} else {
		c.leaseID = leaseID
	}
}

func (c *RemoteClient) leaseAccessCondition() *blob.AccessConditions {
	return &blob.AccessConditions{
		LeaseAccessConditions: &blob.LeaseAccessConditions{
			LeaseID: c.leaseID,
		},
	}
}

func notFoundError(err error) bool {
	respErr, ok := err.(*azcore.ResponseError)
	return ok && respErr.StatusCode == 404
}

func httpHeaders() *blob.HTTPHeaders {
	contentType := "application/json"
	return &blob.HTTPHeaders{
		BlobContentType: &contentType,
	}
}
