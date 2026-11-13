// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	uuid "github.com/hashicorp/go-uuid"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"

	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// The SDK request and response types are kept at this boundary so that tests
// can exercise conditional requests without requiring an OCI account.
type objectStorageClient interface {
	GetObject(context.Context, objectstorage.GetObjectRequest) (objectstorage.GetObjectResponse, error)
	PutObject(context.Context, objectstorage.PutObjectRequest) (objectstorage.PutObjectResponse, error)
	DeleteObject(context.Context, objectstorage.DeleteObjectRequest) (objectstorage.DeleteObjectResponse, error)
	ListObjects(context.Context, objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error)
}

const (
	objectOperationTimeout  = 60 * time.Second
	objectOperationAttempts = 4
	writeTokenMetadata      = "opentofu-write-id"
	maxLockSize             = 1024 * 1024
)

var (
	errObjectChanged  = errors.New("the remote object changed concurrently; refresh the state before retrying")
	errObjectNoBody   = errors.New("OCI returned an object without a body")
	errObjectNoETag   = errors.New("OCI returned an object without an ETag; mutation was refused to protect state")
	errObjectChecksum = errors.New("OCI object checksum verification failed; check the remote object and connectivity before retrying")
	errOversizedLock  = errors.New("OCI state lock exceeds 1 MiB; inspect the lock object before attempting recovery")
)

// remoteClient keeps the ETag observed by Get, including an explicit observation
// that the object is absent. Every subsequent mutation uses that observation as
// a precondition, even when CLI state locking is disabled.
type remoteClient struct {
	client           objectStorageClient
	namespace        string
	bucketName       string
	stateFilePath    string
	lockFilePath     string
	kmsKeyID         string
	locking          bool
	operationTimeout time.Duration

	mu          sync.Mutex
	stateSeen   bool
	stateETag   string
	heldLockID  string
	heldLockTag string
	retryWait   func(context.Context, int) error
}

var _ remote.OptionalClientLocker = (*remoteClient)(nil)

func newRemoteClient(client objectStorageClient, namespace, bucket, statePath string, locking bool, kmsKeyID string) *remoteClient {
	return &remoteClient{
		client: client, namespace: namespace, bucketName: bucket,
		stateFilePath: statePath, lockFilePath: statePath + ".lock",
		locking: locking, kmsKeyID: kmsKeyID, retryWait: waitForObjectRetry, operationTimeout: objectOperationTimeout,
	}
}

func (c *remoteClient) IsLockingEnabled() bool { return c.locking }

func (c *remoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	obj, err := c.readObject(ctx, c.stateFilePath)
	if err != nil {
		return nil, err
	}
	c.stateSeen, c.stateETag = true, obj.etag
	if !obj.exists {
		return nil, nil
	}
	digest := md5.Sum(obj.data)
	return &remote.Payload{Data: obj.data, MD5: digest[:]}, nil
}

func (c *remoteClient) Put(ctx context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	if err := c.observeState(ctx); err != nil {
		return err
	}
	token, err := uuid.GenerateUUID()
	if err != nil {
		return errors.New("OCI state upload could not create an operation identifier")
	}
	etag, err := c.writeObject(ctx, c.stateFilePath, data, c.stateETag, token, true)
	if err != nil {
		return err
	}
	c.stateSeen, c.stateETag = true, etag
	return nil
}

func (c *remoteClient) Delete(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	if err := c.observeState(ctx); err != nil {
		return err
	}
	if err := c.verifyHeldLock(ctx); err != nil {
		return err
	}
	if c.stateETag == "" {
		return nil
	}
	if err := c.removeObject(ctx, c.stateFilePath, c.stateETag, true); err != nil {
		return err
	}
	c.stateSeen, c.stateETag = true, ""
	return nil
}

func (c *remoteClient) observeState(ctx context.Context) error {
	if c.stateSeen {
		return nil
	}
	obj, err := c.readObject(ctx, c.stateFilePath)
	if err != nil {
		return err
	}
	c.stateSeen, c.stateETag = true, obj.etag
	return nil
}

func (c *remoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.locking {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()
	if info == nil {
		return "", errors.New("OCI state locking requires lock information")
	}
	if c.heldLockID != "" {
		return "", c.lockError(ctx, errors.New("this state client already holds a lock"))
	}
	if info.ID == "" {
		id, err := uuid.GenerateUUID()
		if err != nil {
			return "", errors.New("OCI state locking could not create a lock identifier")
		}
		info.ID = id
	}
	info.Path = c.stateFilePath
	data, err := json.Marshal(info)
	if err != nil || len(data) > maxLockSize {
		return "", errors.New("OCI lock information cannot be encoded or exceeds 1 MiB")
	}
	token, err := uuid.GenerateUUID()
	if err != nil {
		return "", errors.New("OCI state locking could not create an operation identifier")
	}
	etag, err := c.writeObject(ctx, c.lockFilePath, data, "", token, false)
	if err != nil {
		return "", c.lockError(ctx, err)
	}
	c.heldLockID, c.heldLockTag = info.ID, etag
	return info.ID, nil
}

func (c *remoteClient) Unlock(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.locking {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	obj, info, err := c.readLock(ctx)
	if err != nil {
		return &statemgr.LockError{Err: err}
	}
	if !obj.exists {
		return &statemgr.LockError{Err: errors.New("OCI state lock no longer exists")}
	}
	if id == "" || info.ID != id {
		return &statemgr.LockError{Err: errors.New("the supplied lock ID does not match the OCI state lock"), Info: info}
	}
	if c.heldLockID != "" && (c.heldLockID != id || c.heldLockTag != obj.etag) {
		return &statemgr.LockError{Err: errors.New("the OCI state lock was replaced; this client cannot release its replacement"), Info: info}
	}
	if err := c.removeObject(ctx, c.lockFilePath, obj.etag, false); err != nil {
		return c.lockError(ctx, err)
	}
	c.heldLockID, c.heldLockTag = "", ""
	return nil
}

// Object Storage does not offer a transaction covering the lock and state
// objects. This check detects a removed/replaced lock before each mutation;
// the state ETag additionally fences concurrent changes to the state itself.
func (c *remoteClient) verifyHeldLock(ctx context.Context) error {
	if c.heldLockID == "" {
		return nil
	}
	obj, info, err := c.readLock(ctx)
	if err != nil {
		return &statemgr.LockError{Err: err}
	}
	if !obj.exists || info.ID != c.heldLockID || obj.etag != c.heldLockTag {
		return &statemgr.LockError{Err: errors.New("the OCI state lock was removed or replaced; stop this operation and refresh state"), Info: info}
	}
	return nil
}

type storedObject struct {
	exists bool
	etag   string
	data   []byte
	token  string
}

// Preserve seeking so the SDK can rewind an upload if a refreshable identity
// requires another signed request. io.NopCloser would hide bytes.Reader.Seek.
type objectReadSeekCloser struct{ *bytes.Reader }

func (objectReadSeekCloser) Close() error { return nil }

func (c *remoteClient) readObject(ctx context.Context, name string) (storedObject, error) {
	for attempt := 0; attempt < objectOperationAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return storedObject{}, sanitizedOCIError("read object", err)
		}
		policy := common.NoRetryPolicy()
		response, err := c.client.GetObject(ctx, objectstorage.GetObjectRequest{
			NamespaceName: common.String(c.namespace), BucketName: common.String(c.bucketName), ObjectName: common.String(name),
			RequestMetadata: common.RequestMetadata{RetryPolicy: &policy},
		})
		if err == nil {
			obj, readErr := decodeObject(response, name == c.lockFilePath)
			if readErr == nil {
				return obj, nil
			}
			err = readErr
		} else if response.Content != nil {
			response.Content.Close()
		}
		if isObjectMissing(err) {
			return storedObject{}, nil
		}
		if !retryableOCIError(err) || attempt == objectOperationAttempts-1 {
			return storedObject{}, sanitizedOCIError("read object", err)
		}
		if err := c.retryWait(ctx, attempt); err != nil {
			return storedObject{}, sanitizedOCIError("read object", err)
		}
	}
	panic("unreachable")
}

func decodeObject(response objectstorage.GetObjectResponse, isLock bool) (storedObject, error) {
	if response.Content == nil {
		return storedObject{}, errObjectNoBody
	}
	defer response.Content.Close()
	var reader io.Reader = response.Content
	if isLock {
		reader = io.LimitReader(reader, maxLockSize+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return storedObject{}, err
	}
	if isLock && len(data) > maxLockSize {
		return storedObject{}, errOversizedLock
	}
	if response.ETag == nil || *response.ETag == "" {
		return storedObject{}, errObjectNoETag
	}
	if response.ContentMd5 != nil && *response.ContentMd5 != "" {
		digest := md5.Sum(data)
		if base64.StdEncoding.EncodeToString(digest[:]) != *response.ContentMd5 {
			return storedObject{}, errObjectChecksum
		}
	}
	return storedObject{exists: true, etag: *response.ETag, data: data, token: response.OpcMeta[writeTokenMetadata]}, nil
}

// writeObject never sends an unconditional PUT. A unique metadata value lets
// us prove whether an upload committed when its response was lost. A matching
// body alone cannot establish ownership of a state or lock written elsewhere.
func (c *remoteClient) writeObject(ctx context.Context, name string, data []byte, expectedETag, token string, checkLock bool) (string, error) {
	digest := md5.Sum(data)
	for attempt := 0; attempt < objectOperationAttempts; attempt++ {
		if checkLock {
			if err := c.verifyHeldLock(ctx); err != nil {
				return "", err
			}
		}
		if err := ctx.Err(); err != nil {
			return "", sanitizedOCIError("upload object", err)
		}
		policy := common.NoRetryPolicy()
		request := objectstorage.PutObjectRequest{
			NamespaceName: common.String(c.namespace), BucketName: common.String(c.bucketName), ObjectName: common.String(name),
			PutObjectBody: objectReadSeekCloser{bytes.NewReader(data)}, ContentLength: common.Int64(int64(len(data))),
			ContentType: common.String("application/json"), ContentMD5: common.String(base64.StdEncoding.EncodeToString(digest[:])),
			OpcMeta: map[string]string{writeTokenMetadata: token}, RequestMetadata: common.RequestMetadata{RetryPolicy: &policy},
		}
		if expectedETag == "" {
			request.IfNoneMatch = common.String("*")
		} else {
			request.IfMatch = common.String(expectedETag)
		}
		if c.kmsKeyID != "" {
			request.OpcSseKmsKeyId = common.String(c.kmsKeyID)
		}
		response, err := c.client.PutObject(ctx, request)
		request.PutObjectBody.Close()
		if err == nil && response.ETag != nil && *response.ETag != "" {
			return *response.ETag, nil
		}
		if isOCIStatus(err, 412) {
			return "", errObjectChanged
		}
		if err != nil && !retryableOCIError(err) {
			return "", sanitizedOCIError("upload object", err)
		}

		current, readErr := c.readObject(ctx, name)
		if readErr != nil {
			return "", errors.New("OCI upload outcome could not be confirmed; preserve local state and refresh the remote state before retrying")
		}
		if current.exists && current.token == token && bytes.Equal(current.data, data) {
			return current.etag, nil
		}
		if current.etag != expectedETag {
			return "", errObjectChanged
		}
		if attempt == objectOperationAttempts-1 {
			return "", sanitizedOCIError("upload object after bounded retries", err)
		}
		if err := c.retryWait(ctx, attempt); err != nil {
			return "", sanitizedOCIError("upload object", err)
		}
	}
	panic("unreachable")
}

func (c *remoteClient) removeObject(ctx context.Context, name, expectedETag string, checkLock bool) error {
	if expectedETag == "" {
		return errors.New("OCI object deletion requires an observed ETag")
	}
	for attempt := 0; attempt < objectOperationAttempts; attempt++ {
		if checkLock {
			if err := c.verifyHeldLock(ctx); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return sanitizedOCIError("delete object", err)
		}
		policy := common.NoRetryPolicy()
		_, err := c.client.DeleteObject(ctx, objectstorage.DeleteObjectRequest{
			NamespaceName: common.String(c.namespace), BucketName: common.String(c.bucketName), ObjectName: common.String(name), IfMatch: common.String(expectedETag),
			RequestMetadata: common.RequestMetadata{RetryPolicy: &policy},
		})
		if err == nil || isObjectMissing(err) {
			return nil
		}
		if isOCIStatus(err, 412) {
			return errObjectChanged
		}
		if !retryableOCIError(err) {
			return sanitizedOCIError("delete object", err)
		}
		current, readErr := c.readObject(ctx, name)
		if readErr != nil {
			return errors.New("OCI deletion outcome could not be confirmed; inspect the remote state or lock before retrying")
		}
		if !current.exists {
			return nil
		}
		if current.etag != expectedETag {
			return errObjectChanged
		}
		if attempt == objectOperationAttempts-1 {
			return sanitizedOCIError("delete object after bounded retries", err)
		}
		if err := c.retryWait(ctx, attempt); err != nil {
			return sanitizedOCIError("delete object", err)
		}
	}
	panic("unreachable")
}

func (c *remoteClient) readLock(ctx context.Context) (storedObject, *statemgr.LockInfo, error) {
	obj, err := c.readObject(ctx, c.lockFilePath)
	if err != nil || !obj.exists {
		return obj, nil, err
	}
	info := new(statemgr.LockInfo)
	if err := json.Unmarshal(obj.data, info); err != nil || info.ID == "" {
		return obj, nil, errors.New("OCI state lock has invalid metadata; inspect the lock object before attempting recovery")
	}
	return obj, info, nil
}

func (c *remoteClient) lockError(ctx context.Context, cause error) *statemgr.LockError {
	obj, info, err := c.readLock(ctx)
	lockErr := &statemgr.LockError{Err: cause, Info: info}
	if err != nil {
		lockErr.Err = fmt.Errorf("%s; unable to inspect the current OCI lock", cause)
	} else if !obj.exists && errors.Is(cause, errObjectChanged) {
		lockErr.InconsistentRead = true
	}
	return lockErr
}

func waitForObjectRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(100 * time.Millisecond << attempt)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isOCIStatus(err error, status int) bool {
	var serviceErr common.ServiceError
	return errors.As(err, &serviceErr) && serviceErr.GetHTTPStatusCode() == status
}

func isObjectMissing(err error) bool {
	var serviceErr common.ServiceError
	return errors.As(err, &serviceErr) && serviceErr.GetHTTPStatusCode() == 404 && serviceErr.GetCode() == "ObjectNotFound"
}

func retryableOCIError(err error) bool {
	if err == nil {
		return true // A successful mutation without an ETag requires reconciliation.
	}
	var serviceErr common.ServiceError
	if errors.As(err, &serviceErr) {
		status := serviceErr.GetHTTPStatusCode()
		return status == 429 || status == 500 || status == 502 || status == 503 || status == 504
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// SDK and service messages may contain request bodies, file paths, tokens,
// signed URLs, or credential-provider errors. Never interpolate those messages.
func sanitizedOCIError(operation string, err error) error {
	for _, safe := range []error{errObjectNoBody, errObjectNoETag, errObjectChecksum, errOversizedLock} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("OCI %s was canceled", operation)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("OCI %s timed out; check connectivity and retry", operation)
	}
	var serviceErr common.ServiceError
	if errors.As(err, &serviceErr) {
		status := serviceErr.GetHTTPStatusCode()
		switch status {
		case 401:
			return fmt.Errorf("OCI %s was not authenticated (HTTP 401); check the selected credentials and refresh expired security tokens", operation)
		case 403:
			return fmt.Errorf("OCI %s was denied (HTTP 403); check IAM permissions for the configured bucket", operation)
		case 404:
			return fmt.Errorf("OCI %s could not find an accessible resource (HTTP 404); check namespace, bucket, region, and IAM permissions", operation)
		case 412:
			return errObjectChanged
		case 429:
			return fmt.Errorf("OCI %s was throttled (HTTP 429); retry after reducing request concurrency", operation)
		default:
			return fmt.Errorf("OCI %s failed (HTTP %d); check Object Storage availability and configuration", operation, status)
		}
	}
	return fmt.Errorf("OCI %s failed; check connectivity, credentials, and Object Storage configuration", operation)
}
