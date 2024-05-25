// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vault

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-uuid"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

const (
	lockSuffix = "/.lock"

	secretNotFoundError = "secret not found"
)

// RemoteClient is a remote client that stores data in Consul.
type RemoteClient struct {
	Mount string
	Name  string
	GZip  bool

	mu     sync.Mutex
	Client *vaultapi.Client
	// lockState is true if we're using locks
	lockState bool

	// The index of the last state we wrote.
	// If this is > 0, Put will perform a CAS to ensure that the state wasn't
	// changed during the operation. This is important even with locks, because
	// if the client loses the lock for some reason, then reacquires it, we
	// need to make sure that the state was not modified.
	modifyIndex uint64
}

func (c *RemoteClient) Get() (*remote.Payload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	kv := c.Client.KVv2(c.Mount)

	chunked, hash, chunks, secret, err := c.chunkedMode()

	// If vault error contains no secret, return empty state
	if err != nil {
		if strings.Contains(err.Error(), secretNotFoundError) {
			return nil, nil
		}
		return nil, err
	}

	c.modifyIndex = uint64(secret.VersionMetadata.Version)

	var payload string
	if chunked {
		for _, chunkName := range chunks {
			ctx := context.TODO()
			secret, err := kv.Get(ctx, chunkName)
			if err != nil {
				return nil, err
			}
			if secret == nil {
				return nil, fmt.Errorf("Key %q could not be found", chunkName)
			}
			if val, ok := secret.Data["data"]; ok {
				if f, ok := val.(string); ok {
					payload = payload + f
				} else {
					return nil, fmt.Errorf("Invalid chunk type, not string: %v", chunkName)
				}
			} else {
				return nil, fmt.Errorf("Could not find 'data' key: %v", chunkName)
			}
		}
	} else {
		if val, ok := secret.Data["data"]; ok {
			if f, ok := val.(string); ok {
				payload = f
			} else {
				return nil, fmt.Errorf("Invalid chunk type, not string: %v", c.Name)
			}
		} else {
			return nil, fmt.Errorf("Could not find 'data' key: %v", c.Name)
		}
	}

	// If the payload starts with 0x1f, it's gzip, not json
	if len(payload) >= 1 && payload[0] == '\x1f' {
		payload, err = uncompressState(payload)
		if err != nil {
			return nil, err
		}
	}

	md5 := md5.Sum([]byte(payload))

	if hash != "" && fmt.Sprintf("%x", md5) != hash {
		return nil, fmt.Errorf("The remote state does not match the expected hash")
	}

	return &remote.Payload{
		Data: []byte(payload),
		MD5:  md5[:],
	}, nil
}

func (c *RemoteClient) Put(data []byte) error {
	// The state can be stored in 4 different ways, based on the payload size
	// and whether the user enabled gzip:
	//  - single entry mode with plain JSON: a single JSON is stored at
	//	  "tfstate/my_project"
	//  - single entry mode gzip: the JSON payload is first gziped and stored at
	//    "tfstate/my_project"
	//  - chunked mode with plain JSON: the JSON payload is split in pieces and
	//    stored like so:
	//       - "tfstate/my_project" -> a JSON payload that contains the path of
	//         the chunks and an MD5 sum like so:
	//              {
	//              	"current-hash": "abcdef1234",
	//              	"chunks": [
	//              		"tfstate/my_project/tfstate.abcdef1234/0",
	//              		"tfstate/my_project/tfstate.abcdef1234/1",
	//              		"tfstate/my_project/tfstate.abcdef1234/2",
	//              	]
	//              }
	//       - "tfstate/my_project/tfstate.abcdef1234/0" -> The first chunk
	//       - "tfstate/my_project/tfstate.abcdef1234/1" -> The next one
	//       - ...
	//  - chunked mode with gzip: the same system but we gziped the JSON payload
	//    before splitting it in chunks
	//
	// When overwritting the current state, we need to clean the old chunks if
	// we were in chunked mode (no matter whether we need to use chunks for the
	// new one). To do so based on the 4 possibilities above we look at the
	// value at "tfstate/my_project" and if it is:
	//  - absent then it's a new state and there will be nothing to cleanup,
	//  - not a JSON payload we were in single entry mode with gzip so there will
	// 	  be nothing to cleanup
	//  - a JSON payload, then we were either single entry mode with plain JSON
	//    or in chunked mode. To differentiate between the two we look whether a
	//    "current-hash" key is present in the payload. If we find one we were
	//    in chunked mode and we will need to remove the old chunks (whether or
	//    not we were using gzip does not matter in that case).

	c.mu.Lock()
	defer c.mu.Unlock()

	kv := c.Client.KVv2(c.Mount)
	ctx := context.TODO()

	// First we determine what mode we were using and to prepare the cleanup
	chunked, _, oldChunks, _, err := c.chunkedMode()
	if err != nil {
		if !strings.Contains(err.Error(), secretNotFoundError) {
			return err
		}
	}
	cleanupOldChunks := func() {}
	if chunked {
		cleanupOldChunks = func() {
			for _, chunkName := range oldChunks {
				ctx := context.TODO()
				kv.Delete(ctx, chunkName)
			}
		}
	}

	payload := string(data)
	if c.GZip {
		if compressedState, err := compressState(payload); err == nil {
			payload = compressedState
		} else {
			return err
		}
	}

	// The payload may be too large to store in a single KV entry in Consul. We
	// could try to determine whether it will fit or not before sending the
	// request but since we are using the Transaction API and not the KV API,
	// it grows by about a 1/3 when it is base64 encoded plus the overhead of
	// the fields specific to the Transaction API.
	// Rather than trying to calculate the overhead (which could change from
	// one version of Consul to another, and between Consul Community Edition
	// and Consul Enterprise), we try to send the whole state in one request, if
	// it fails because it is too big we then split it in chunks and send each
	// chunk separately.
	// When splitting in chunks, we make each chunk 524288 bits, which is the
	// default max size for raft. If the user changed it, we still may send
	// chunks too big and fail but this is not a setting that should be fiddled
	// with anyway.

	store := func(payload map[string]interface{}) error {
		var secret *vaultapi.KVSecret = nil
		var err error = nil
		if c.modifyIndex == 0 {
			secret, err = kv.Put(ctx, c.Name, payload)
		} else {
			secret, err = kv.Patch(ctx, c.Name, payload)
		}
		if err != nil {
			return err
		}
		c.modifyIndex = uint64(secret.VersionMetadata.Version)

		// We remove all the old chunks
		cleanupOldChunks()

		return nil
	}

	if err = store(map[string]interface{}{"data": payload}); err == nil {
		// The payload was small enough to be stored
		return nil
	} else if !strings.Contains(err.Error(), "too large") {
		// We failed for some other reason, report this to the user
		return err
	}

	// The payload was too large so we split it in multiple chunks

	md5 := md5.Sum(data)
	chunks := split(payload, 524288)
	chunkPaths := make([]string, 0)

	// First we write the new chunks
	for i, p := range chunks {
		path := strings.TrimRight(c.Mount, "/") + fmt.Sprintf("/tfstate.%x/%d", md5, i)
		chunkPaths = append(chunkPaths, path)
		_, err := kv.Put(ctx, path, map[string]interface{}{
			"data": p,
		})

		if err != nil {
			return err
		}
	}

	// Then we update the link to point to the new chunks
	return store(map[string]interface{}{
		"current-hash": fmt.Sprintf("%x", md5),
		"chunks":       chunkPaths,
	})
}

func (c *RemoteClient) Delete() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	kv := c.Client.KVv2(c.Mount)
	ctx := context.TODO()

	chunked, _, chunks, _, err := c.chunkedMode()
	if err != nil {
		return err
	}

	err = kv.Delete(ctx, c.Name)

	// If there were chunks we need to remove them
	if chunked {
		for _, chunk := range chunks {
			kv.Delete(ctx, chunk)
		}
	}

	return err
}

func (c *RemoteClient) lockPath() string {
	// we sanitize the path for the lock as Consul does not like having
	// two consecutive slashes for the lock path
	return c.Name + ".lock"
}

func (c *RemoteClient) getLockInfo() (*statemgr.LockInfo, error) {
	path := c.lockPath()

	kv := c.Client.KVv2(c.Mount)
	ctx := context.TODO()
	secret, err := kv.Get(ctx, path)

	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, nil
	}

	li := &statemgr.LockInfo{}
	byteData, err := json.Marshal(secret.Data)
	if err != nil {
		return nil, fmt.Errorf("Error re-marshaling lock info: %w", err)
	}
	err = json.Unmarshal(byteData, li)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling lock info: %w", err)
	}

	return li, nil
}

func hasSecretNotFound(err error) bool {
	if err != nil && strings.Contains(err.Error(), secretNotFoundError) {
		return true
	}
	return false
}

func (c *RemoteClient) getCurrentLockMetadata() (*vaultapi.KVMetadata, error) {
	metadata, err := c.Client.KVv2(c.Mount).GetMetadata(context.TODO(), c.lockPath())
	if hasSecretNotFound(err) {
		return nil, nil
	}
	return metadata, err
}

func (c *RemoteClient) Lock(info *statemgr.LockInfo) (string, error) {
	info.Path = c.lockPath()
	kv := c.Client.KVv2(c.Mount)
	ctx := context.TODO()

	// Lock perform lock by performing the following, making use of CAS:
	// 1. Obtain latest version of lock secret
	// 2. Ensure latest version is either non-existent or deleted
	// 3. Increment the CAS
	// 4. Create new version with inremented CAS
	// 5. If the write is allowed, then we have the lock
	// 6. If Vault rejects the secret, then someone else has locked

	if info.ID == "" {
		lockID, err := uuid.GenerateUUID()
		if err != nil {
			return "", err
		}

		info.ID = lockID
	}

	// Implement 1.
	currentLockMetadata, err := c.getCurrentLockMetadata()
	if err != nil {
		return "", err
	}

	var lockCas int = 0
	if currentLockMetadata != nil {
		// Implement 2.
		currentLockData, err := kv.GetVersion(ctx, c.lockPath(), currentLockMetadata.CurrentVersion)
		if err != nil {
			return "", err
		}
		if currentLockId, ok := currentLockData.Data["ID"]; ok {
			return "", fmt.Errorf("Lock already exists: %v", currentLockId)
		}

		// Implement 3.
		lockCas = currentLockMetadata.CurrentVersion
	}

	val := info.Marshal()
	var vaultData map[string]interface{}
	err = json.Unmarshal(val, &vaultData)
	if err != nil {
		return "", err
	}

	// Implement 4.
	_, err = kv.Put(ctx, c.lockPath(), vaultData, vaultapi.WithCheckAndSet(lockCas))

	// Implement 6.
	if err != nil {
		lockInfo, infoErr := c.getLockInfo()
		if infoErr != nil {
			err = multierror.Append(err, infoErr)
		}

		lockErr := &statemgr.LockError{
			Err:  err,
			Info: lockInfo,
		}
		return "", lockErr
	}

	// Implement 5.
	return info.ID, nil
}

// called after a lock is acquired
var testLockHook func()

func (c *RemoteClient) Unlock(id string) error {
	kv := c.Client.KVv2(c.Mount)
	ctx := context.TODO()

	currentLockData, err := kv.Get(ctx, c.lockPath())
	if err != nil {
		return err
	}
	if currentLockData == nil {
		return fmt.Errorf("Unable to obtain current lock secret")
	}
	if existingLockId, ok := currentLockData.Data["ID"]; ok {
		if existingLockId != id {
			return fmt.Errorf("Existing lock ID (%s) does not match ID to unlock: %s", existingLockId, id)
		}
	} else {
		return fmt.Errorf("Cannot obtain ID from existing lock")
	}

	var versionsToDelete = []int{currentLockData.VersionMetadata.Version}
	err = kv.DeleteVersions(ctx, c.lockPath(), versionsToDelete)
	if err != nil {
		return err
	}

	return nil
}

func compressState(data string) (string, error) {
	b := new(bytes.Buffer)
	gz := gzip.NewWriter(b)
	if _, err := gz.Write([]byte(data)); err != nil {
		return "", err
	}
	if err := gz.Flush(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func uncompressState(data string) (string, error) {
	b := new(bytes.Buffer)
	reader := strings.NewReader(data)
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return "", err
	}
	b.ReadFrom(gz)
	if err := gz.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func split(payload string, limit int) []string {
	var chunk string
	chunks := make([]string, 0, len(payload)/limit+1)
	for len(payload) >= limit {
		chunk, payload = payload[:limit], payload[limit:]
		chunks = append(chunks, chunk)
	}
	if len(payload) > 0 {
		chunks = append(chunks, payload[:])
	}
	return chunks
}

func (c *RemoteClient) chunkedMode() (bool, string, []string, *vaultapi.KVSecret, error) {
	kv := c.Client.KVv2(c.Mount)
	ctx := context.TODO()
	secret, err := kv.Get(ctx, c.Name)
	if err != nil {
		if strings.Contains(err.Error(), secretNotFoundError) {
			return false, "", nil, secret, err
		}
		return false, "", nil, secret, err
	}
	d := secret.Data
	// If we find the "current-hash" key we were in chunked mode
	hash, ok := d["current-hash"]
	if ok {
		chunks := make([]string, 0)
		for _, c := range d["chunks"].([]interface{}) {
			chunks = append(chunks, c.(string))
		}
		return true, hash.(string), chunks, secret, nil
	}
	return false, "", nil, secret, nil
}
