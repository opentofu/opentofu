// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package r2

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/version"
)

// RemoteClient implements the remote.Client interface for R2
type RemoteClient struct {
	backend    *Backend
	bucketName string
	key        string
}

// Get retrieves the state from R2
func (c *RemoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	// Use the R2 S3-compatible endpoint for object operations
	url := fmt.Sprintf("%s/%s/%s", c.backend.getR2Endpoint(), c.bucketName, c.key)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	// Add R2 authentication headers
	c.addAuthHeaders(req, "GET", c.key, nil)
	
	resp, err := c.backend.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	// Handle not found
	if resp.StatusCode == 404 {
		return nil, nil
	}
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get state: %s (status: %d)", string(body), resp.StatusCode)
	}
	
	// Read the state data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read state data: %w", err)
	}
	
	// Calculate MD5 for integrity
	md5Sum := md5.Sum(data)
	
	payload := &remote.Payload{
		Data: data,
		MD5:  md5Sum[:],
	}
	
	return payload, nil
}

// Put stores the state in R2
func (c *RemoteClient) Put(ctx context.Context, data []byte) error {
	url := fmt.Sprintf("%s/%s/%s", c.backend.getR2Endpoint(), c.bucketName, c.key)
	
	// Calculate MD5 for content verification
	md5Sum := md5.Sum(data)
	md5Base64 := base64.StdEncoding.EncodeToString(md5Sum[:])
	
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	
	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-MD5", md5Base64)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	
	// Add custom metadata for state management
	req.Header.Set("x-amz-meta-opentofu-version", "1.0")
	req.Header.Set("x-amz-meta-last-modified", time.Now().UTC().Format(time.RFC3339))
	
	// Add R2 authentication headers
	c.addAuthHeaders(req, "PUT", c.key, data)
	
	resp, err := c.backend.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to put state: %s (status: %d)", string(body), resp.StatusCode)
	}
	
	return nil
}

// Delete removes the state from R2
func (c *RemoteClient) Delete(ctx context.Context) error {
	url := fmt.Sprintf("%s/%s/%s", c.backend.getR2Endpoint(), c.bucketName, c.key)
	
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	
	// Add R2 authentication headers
	c.addAuthHeaders(req, "DELETE", c.key, nil)
	
	resp, err := c.backend.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	// 204 No Content or 200 OK are both acceptable
	if resp.StatusCode != 204 && resp.StatusCode != 200 && resp.StatusCode != 404 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete state: %s (status: %d)", string(body), resp.StatusCode)
	}
	
	return nil
}

// Lock is not implemented for R2 backend
// R2 doesn't support object locking like DynamoDB
func (c *RemoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	// Return a simple lock ID to satisfy the interface
	// In practice, users should use external locking mechanisms
	return "r2-no-lock", nil
}

// Unlock is not implemented for R2 backend
func (c *RemoteClient) Unlock(ctx context.Context, id string) error {
	// No-op since we don't support locking
	return nil
}

// addAuthHeaders adds authentication headers for R2 S3-compatible API
func (c *RemoteClient) addAuthHeaders(req *http.Request, method, key string, body []byte) {
	// R2 uses API tokens for authentication
	// The token can be used directly as a bearer token or converted to S3-style credentials
	
	// For simplicity and native R2 support, use the API token directly
	req.Header.Set("Authorization", "Bearer "+c.backend.apiToken)
	
	// Add required headers for R2
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("x-amz-date", time.Now().UTC().Format("20060102T150405Z"))
	req.Header.Set("User-Agent", httpclient.OpenTofuUserAgent(version.String()))
	
	// Add content hash for integrity
	if body != nil {
		contentHash := hex.EncodeToString(hashSHA256(body))
		req.Header.Set("x-amz-content-sha256", contentHash)
	} else {
		req.Header.Set("x-amz-content-sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") // Empty hash
	}
}

// hashSHA256 computes SHA256 hash of data
func hashSHA256(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}
