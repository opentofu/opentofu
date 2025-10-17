// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"cloud.google.com/go/storage"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// remoteClient is used by "state/remote".State to read and write
// blobs representing state.
// Implements "state/remote".ClientLocker
type remoteClient struct {
	storageClient *storage.Client
	bucketName    string
	stateFilePath string
	lockFilePath  string
	encryptionKey []byte
	kmsKeyName    string
}

func (c *remoteClient) Get(ctx context.Context) (payload *remote.Payload, err error) {
	stateFileReader, err := c.stateFile().NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, nil
		} else {
			return nil, fmt.Errorf("Failed to open state file at %v: %w", c.stateFileURL(), err)
		}
	}
	defer stateFileReader.Close()

	stateFileContents, err := io.ReadAll(stateFileReader)
	if err != nil {
		return nil, fmt.Errorf("Failed to read state file from %v: %w", c.stateFileURL(), err)
	}

	stateFileAttrs, err := c.stateFile().Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to read state file attrs from %v: %w", c.stateFileURL(), err)
	}

	result := &remote.Payload{
		Data: stateFileContents,
		MD5:  stateFileAttrs.MD5,
	}

	return result, nil
}

func (c *remoteClient) Put(ctx context.Context, data []byte) error {
	err := func() error {
		stateFileWriter := c.stateFile().NewWriter(ctx)
		if len(c.kmsKeyName) > 0 {
			stateFileWriter.KMSKeyName = c.kmsKeyName
		}
		if _, err := stateFileWriter.Write(data); err != nil {
			return err
		}
		return stateFileWriter.Close()
	}()
	if err != nil {
		return fmt.Errorf("Failed to upload state to %v: %w", c.stateFileURL(), err)
	}

	return nil
}

func (c *remoteClient) Delete(ctx context.Context) error {
	if err := c.stateFile().Delete(ctx); err != nil {
		return fmt.Errorf("Failed to delete state file %v: %w", c.stateFileURL(), err)
	}

	return nil
}

// Lock writes to a lock file, ensuring file creation. Returns the generation
// number, which must be passed to Unlock().
func (c *remoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	// update the path we're using
	// we can't set the ID until the info is written
	info.Path = c.lockFileURL()

	infoJson, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	lockFile := c.lockFile()
	w := lockFile.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	err = func() error {
		if _, err := w.Write(infoJson); err != nil {
			return err
		}
		return w.Close()
	}()

	if err != nil {
		return "", c.lockError(ctx, fmt.Errorf("writing %q failed: %w", c.lockFileURL(), err))
	}

	info.ID = strconv.FormatInt(w.Attrs().Generation, 10)

	return info.ID, nil
}

func (c *remoteClient) Unlock(ctx context.Context, id string) error {
	gen, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("Lock ID should be numerical value, got '%s'", id)
	}

	if err := c.lockFile().If(storage.Conditions{GenerationMatch: gen}).Delete(ctx); err != nil {
		return c.lockError(ctx, err)
	}

	return nil
}

func (c *remoteClient) lockError(ctx context.Context, err error) *statemgr.LockError {
	lockErr := &statemgr.LockError{
		Err: err,
	}

	info, infoErr := c.lockInfo(ctx)
	switch {
	case errors.Is(infoErr, storage.ErrObjectNotExist):
		// Race condition - file exists initially but then has been deleted by other process
		lockErr.InconsistentRead = true
	case infoErr != nil:
		lockErr.Err = multierror.Append(lockErr.Err, infoErr)
	default:
		lockErr.Info = info
	}
	return lockErr
}

// lockInfo reads the lock file, parses its contents and returns the parsed
// LockInfo struct.
func (c *remoteClient) lockInfo(ctx context.Context) (*statemgr.LockInfo, error) {
	r, err := c.lockFile().NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	rawData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	info := &statemgr.LockInfo{}
	if err := json.Unmarshal(rawData, info); err != nil {
		return nil, err
	}

	// We use the Generation as the ID, so overwrite the ID in the json.
	// This can't be written into the Info, since the generation isn't known
	// until it's written.
	attrs, err := c.lockFile().Attrs(ctx)
	if err != nil {
		return nil, err
	}
	info.ID = strconv.FormatInt(attrs.Generation, 10)

	return info, nil
}

func (c *remoteClient) stateFile() *storage.ObjectHandle {
	h := c.storageClient.Bucket(c.bucketName).Object(c.stateFilePath)
	if len(c.encryptionKey) > 0 {
		return h.Key(c.encryptionKey)
	}
	return h
}

func (c *remoteClient) stateFileURL() string {
	return fmt.Sprintf("gs://%v/%v", c.bucketName, c.stateFilePath)
}

func (c *remoteClient) lockFile() *storage.ObjectHandle {
	return c.storageClient.Bucket(c.bucketName).Object(c.lockFilePath)
}

func (c *remoteClient) lockFileURL() string {
	return fmt.Sprintf("gs://%v/%v", c.bucketName, c.lockFilePath)
}
