// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cos

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	tag "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tag/v20180813"
	"github.com/tencentyun/cos-go-sdk-v5"
)

const (
	lockTagKey = "tencentcloud-terraform-lock"
)

// RemoteClient implements the client of remote state
type remoteClient struct {
	cosClient *cos.Client
	tagClient *tag.Client

	bucket    string
	stateFile string
	lockFile  string
	encrypt   bool
	acl       string
}

// Get returns remote state file
func (c *remoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	log.Printf("[DEBUG] get remote state file %s", c.stateFile)

	exists, data, checksum, err := c.getObject(ctx, c.stateFile)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	payload := &remote.Payload{
		Data: data,
		MD5:  []byte(checksum),
	}

	return payload, nil
}

// Put put state file to remote
func (c *remoteClient) Put(ctx context.Context, data []byte) error {
	log.Printf("[DEBUG] put remote state file %s", c.stateFile)

	return c.putObject(ctx, c.stateFile, data)
}

// Delete delete remote state file
func (c *remoteClient) Delete(ctx context.Context) error {
	log.Printf("[DEBUG] delete remote state file %s", c.stateFile)

	return c.deleteObject(ctx, c.stateFile)
}

// Lock lock remote state file for writing
func (c *remoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	log.Printf("[DEBUG] lock remote state file %s", c.lockFile)

	err := c.cosLock(c.bucket, c.lockFile)
	if err != nil {
		return "", c.lockError(ctx, err)
	}
	// Local helper function so we can call it multiple places
	lockUnlock := func(parent error) error {
		if err := c.cosUnlock(c.bucket, c.lockFile); err != nil {
			return errors.Join(
				fmt.Errorf("error unlocking cos state: %w", c.lockError(ctx, err)),
				parent,
			)
		}
		return parent
	}

	exists, _, _, err := c.getObject(ctx, c.lockFile)
	if err != nil {
		return "", lockUnlock(c.lockError(ctx, err))
	}

	if exists {
		return "", lockUnlock(c.lockError(ctx, fmt.Errorf("lock file %s exists", c.lockFile)))
	}

	info.Path = c.lockFile
	data, err := json.Marshal(info)
	if err != nil {
		return "", lockUnlock(c.lockError(ctx, err))
	}

	check := fmt.Sprintf("%x", md5.Sum(data))
	err = c.putObject(ctx, c.lockFile, data)
	if err != nil {
		return "", lockUnlock(c.lockError(ctx, err))
	}

	return check, lockUnlock(nil)
}

// Unlock unlock remote state file
func (c *remoteClient) Unlock(ctx context.Context, check string) error {
	log.Printf("[DEBUG] unlock remote state file %s", c.lockFile)

	info, err := c.lockInfo(ctx)
	if err != nil {
		return c.lockError(ctx, err)
	}

	if info.ID != check {
		return c.lockError(ctx, fmt.Errorf("lock id mismatch, %v != %v", info.ID, check))
	}

	err = c.deleteObject(ctx, c.lockFile)
	if err != nil {
		return c.lockError(ctx, err)
	}

	err = c.cosUnlock(c.bucket, c.lockFile)
	if err != nil {
		return c.lockError(ctx, err)
	}

	return nil
}

// lockError returns statemgr.LockError
func (c *remoteClient) lockError(ctx context.Context, err error) *statemgr.LockError {
	log.Printf("[DEBUG] failed to lock or unlock %s: %v", c.lockFile, err)

	lockErr := &statemgr.LockError{
		Err: err,
	}

	info, infoErr := c.lockInfo(ctx)
	if infoErr != nil {
		lockErr.Err = multierror.Append(lockErr.Err, infoErr)
	} else {
		lockErr.Info = info
	}

	return lockErr
}

// lockInfo returns LockInfo from lock file
func (c *remoteClient) lockInfo(ctx context.Context) (*statemgr.LockInfo, error) {
	exists, data, checksum, err := c.getObject(ctx, c.lockFile)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, fmt.Errorf("lock file %s not exists", c.lockFile)
	}

	info := &statemgr.LockInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, err
	}

	info.ID = checksum

	return info, nil
}

// getObject get remote object
func (c *remoteClient) getObject(ctx context.Context, cosFile string) (exists bool, data []byte, checksum string, err error) {
	rsp, err := c.cosClient.Object.Get(ctx, cosFile, nil)
	if rsp == nil {
		log.Printf("[DEBUG] getObject %s: error: %v", cosFile, err)
		err = fmt.Errorf("failed to open file at %v: %w", cosFile, err)
		return
	}
	defer rsp.Body.Close()

	log.Printf("[DEBUG] getObject %s: code: %d, error: %v", cosFile, rsp.StatusCode, err)
	if err != nil {
		if rsp.StatusCode == 404 {
			err = nil
		} else {
			err = fmt.Errorf("failed to open file at %v: %w", cosFile, err)
		}
		return
	}

	checksum = rsp.Header.Get("X-Cos-Meta-Md5")
	log.Printf("[DEBUG] getObject %s: checksum: %s", cosFile, checksum)
	if len(checksum) != 32 {
		err = fmt.Errorf("failed to open file at %v: checksum %s invalid", cosFile, checksum)
		return
	}

	exists = true
	data, err = io.ReadAll(rsp.Body)
	log.Printf("[DEBUG] getObject %s: data length: %d", cosFile, len(data))
	if err != nil {
		err = fmt.Errorf("failed to open file at %v: %w", cosFile, err)
		return
	}

	check := fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[DEBUG] getObject %s: check: %s", cosFile, check)
	if check != checksum {
		err = fmt.Errorf("failed to open file at %v: checksum mismatch, %s != %s", cosFile, check, checksum)
		return
	}

	return
}

// putObject put object to remote
func (c *remoteClient) putObject(ctx context.Context, cosFile string, data []byte) error {
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			XCosMetaXXX: &http.Header{
				"X-Cos-Meta-Md5": []string{fmt.Sprintf("%x", md5.Sum(data))},
			},
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{
			XCosACL: c.acl,
		},
	}

	if c.encrypt {
		opt.ObjectPutHeaderOptions.XCosServerSideEncryption = "AES256"
	}

	r := bytes.NewReader(data)
	rsp, err := c.cosClient.Object.Put(ctx, cosFile, r, opt)
	if rsp == nil {
		log.Printf("[DEBUG] putObject %s: error: %v", cosFile, err)
		return fmt.Errorf("failed to save file to %v: %w", cosFile, err)
	}
	defer rsp.Body.Close()

	log.Printf("[DEBUG] putObject %s: code: %d, error: %v", cosFile, rsp.StatusCode, err)
	if err != nil {
		return fmt.Errorf("failed to save file to %v: %w", cosFile, err)
	}

	return nil
}

// deleteObject delete remote object
func (c *remoteClient) deleteObject(ctx context.Context, cosFile string) error {
	rsp, err := c.cosClient.Object.Delete(ctx, cosFile)
	if rsp == nil {
		log.Printf("[DEBUG] deleteObject %s: error: %v", cosFile, err)
		return fmt.Errorf("failed to delete file %v: %w", cosFile, err)
	}
	defer rsp.Body.Close()

	log.Printf("[DEBUG] deleteObject %s: code: %d, error: %v", cosFile, rsp.StatusCode, err)
	if rsp.StatusCode == 404 {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to delete file %v: %w", cosFile, err)
	}

	return nil
}

// getBucket list bucket by prefix
func (c *remoteClient) getBucket(ctx context.Context, prefix string) (obs []cos.Object, err error) {
	fs, rsp, err := c.cosClient.Bucket.Get(ctx, &cos.BucketGetOptions{Prefix: prefix})
	if rsp == nil {
		log.Printf("[DEBUG] getBucket %s/%s: error: %v", c.bucket, prefix, err)
		err = fmt.Errorf("bucket %s not exists", c.bucket)
		return
	}
	defer rsp.Body.Close()

	log.Printf("[DEBUG] getBucket %s/%s: code: %d, error: %v", c.bucket, prefix, rsp.StatusCode, err)
	if rsp.StatusCode == 404 {
		err = fmt.Errorf("bucket %s not exists", c.bucket)
		return
	}

	if err != nil {
		return
	}

	return fs.Contents, nil
}

// putBucket create cos bucket
func (c *remoteClient) putBucket(ctx context.Context) error {
	rsp, err := c.cosClient.Bucket.Put(ctx, nil)
	if rsp == nil {
		log.Printf("[DEBUG] putBucket %s: error: %v", c.bucket, err)
		return fmt.Errorf("failed to create bucket %v: %w", c.bucket, err)
	}
	defer rsp.Body.Close()

	log.Printf("[DEBUG] putBucket %s: code: %d, error: %v", c.bucket, rsp.StatusCode, err)
	if rsp.StatusCode == 409 {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create bucket %v: %w", c.bucket, err)
	}

	return nil
}

// deleteBucket delete cos bucket
func (c *remoteClient) deleteBucket(ctx context.Context, recursive bool) error {
	if recursive {
		obs, err := c.getBucket(ctx, "")
		if err != nil {
			if strings.Contains(err.Error(), "not exists") {
				return nil
			}
			log.Printf("[DEBUG] deleteBucket %s: empty bucket error: %v", c.bucket, err)
			return fmt.Errorf("failed to empty bucket %v: %w", c.bucket, err)
		}
		for _, v := range obs {
			err := c.deleteObject(ctx, v.Key)
			if err != nil {
				return fmt.Errorf("failed to delete object with key %s: %w", v.Key, err)
			}
		}
	}

	rsp, err := c.cosClient.Bucket.Delete(ctx)
	if rsp == nil {
		log.Printf("[DEBUG] deleteBucket %s: error: %v", c.bucket, err)
		return fmt.Errorf("failed to delete bucket %v: %w", c.bucket, err)
	}
	defer rsp.Body.Close()

	log.Printf("[DEBUG] deleteBucket %s: code: %d, error: %v", c.bucket, rsp.StatusCode, err)
	if rsp.StatusCode == 404 {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to delete bucket %v: %w", c.bucket, err)
	}

	return nil
}

// cosLock lock cos for writing
func (c *remoteClient) cosLock(bucket, cosFile string) error {
	log.Printf("[DEBUG] lock cos file %s:%s", bucket, cosFile)

	cosPath := fmt.Sprintf("%s:%s", bucket, cosFile)
	lockTagValue := fmt.Sprintf("%x", md5.Sum([]byte(cosPath)))

	return c.CreateTag(lockTagKey, lockTagValue)
}

// cosUnlock unlock cos writing
func (c *remoteClient) cosUnlock(bucket, cosFile string) error {
	log.Printf("[DEBUG] unlock cos file %s:%s", bucket, cosFile)

	cosPath := fmt.Sprintf("%s:%s", bucket, cosFile)
	lockTagValue := fmt.Sprintf("%x", md5.Sum([]byte(cosPath)))

	var err error
	for i := 0; i < 30; i++ {
		tagExists, err := c.CheckTag(lockTagKey, lockTagValue)

		if err != nil {
			return err
		}

		if !tagExists {
			return nil
		}

		err = c.DeleteTag(lockTagKey, lockTagValue)
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return err
}

// CheckTag checks if tag key:value exists
func (c *remoteClient) CheckTag(key, value string) (exists bool, err error) {
	request := tag.NewDescribeTagsRequest()
	request.TagKey = &key
	request.TagValue = &value

	response, err := c.tagClient.DescribeTags(request)
	log.Printf("[DEBUG] create tag %s:%s: error: %v", key, value, err)
	if err != nil {
		return
	}

	if len(response.Response.Tags) == 0 {
		return
	}

	tagKey := response.Response.Tags[0].TagKey
	tagValue := response.Response.Tags[0].TagValue

	exists = key == *tagKey && value == *tagValue

	return
}

// CreateTag create tag by key and value
func (c *remoteClient) CreateTag(key, value string) error {
	request := tag.NewCreateTagRequest()
	request.TagKey = &key
	request.TagValue = &value

	_, err := c.tagClient.CreateTag(request)
	log.Printf("[DEBUG] create tag %s:%s: error: %v", key, value, err)
	if err != nil {
		return fmt.Errorf("failed to create tag: %s -> %s: %w", key, value, err)
	}

	return nil
}

// DeleteTag create tag by key and value
func (c *remoteClient) DeleteTag(key, value string) error {
	request := tag.NewDeleteTagRequest()
	request.TagKey = &key
	request.TagValue = &value

	_, err := c.tagClient.DeleteTag(request)
	log.Printf("[DEBUG] delete tag %s:%s: error: %v", key, value, err)
	if err != nil {
		return fmt.Errorf("failed to delete tag: %s -> %s: %w", key, value, err)
	}

	return nil
}
