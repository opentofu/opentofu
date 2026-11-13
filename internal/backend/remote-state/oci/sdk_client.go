// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"context"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

// All workspace clients share one SDK client and refreshable signing identity.
// Serializing SDK calls keeps each request's key identifier and private key
// snapshot together while the SDK invokes the signer's separate getters.
type serializedObjectStorageClient struct {
	client objectStorageClient
	// The official SDK can return on cancellation before its request goroutine
	// has stopped. A factory gives that goroutine its own client and signer
	// snapshot, so a following operation cannot replace its credential pair.
	newClient func() objectStorageClient
	once      sync.Once
	gate      chan struct{}
}

var _ objectStorageClient = (*serializedObjectStorageClient)(nil)

func (c *serializedObjectStorageClient) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.once.Do(func() { c.gate = make(chan struct{}, 1) })
	select {
	case c.gate <- struct{}{}:
		// Cancellation can coincide with an available gate. Check again so a
		// canceled waiter never begins another credential or network operation.
		if err := ctx.Err(); err != nil {
			<-c.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *serializedObjectStorageClient) release() { <-c.gate }

func (c *serializedObjectStorageClient) operationClient() objectStorageClient {
	if c.newClient != nil {
		return c.newClient()
	}
	return c.client
}

func (c *serializedObjectStorageClient) GetObject(ctx context.Context, request objectstorage.GetObjectRequest) (objectstorage.GetObjectResponse, error) {
	if err := c.acquire(ctx); err != nil {
		return objectstorage.GetObjectResponse{}, err
	}
	defer c.release()
	// Signing is finished when headers are returned. The response body need
	// not retain the gate, allowing callers to read it independently.
	return c.operationClient().GetObject(ctx, request)
}

func (c *serializedObjectStorageClient) PutObject(ctx context.Context, request objectstorage.PutObjectRequest) (objectstorage.PutObjectResponse, error) {
	if err := c.acquire(ctx); err != nil {
		return objectstorage.PutObjectResponse{}, err
	}
	defer c.release()
	return c.operationClient().PutObject(ctx, request)
}

func (c *serializedObjectStorageClient) DeleteObject(ctx context.Context, request objectstorage.DeleteObjectRequest) (objectstorage.DeleteObjectResponse, error) {
	if err := c.acquire(ctx); err != nil {
		return objectstorage.DeleteObjectResponse{}, err
	}
	defer c.release()
	return c.operationClient().DeleteObject(ctx, request)
}

func (c *serializedObjectStorageClient) ListObjects(ctx context.Context, request objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	if err := c.acquire(ctx); err != nil {
		return objectstorage.ListObjectsResponse{}, err
	}
	defer c.release()
	return c.operationClient().ListObjects(ctx, request)
}
