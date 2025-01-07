// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package http

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// httpClient is a remote client that stores data in Consul or HTTP REST.
type httpClient struct {
	// Update & Retrieve
	URL          *url.URL
	UpdateMethod string

	// Locking
	LockURL      *url.URL
	LockMethod   string
	UnlockURL    *url.URL
	UnlockMethod string

	// HTTP
	Client   *retryablehttp.Client
	Headers  map[string]string
	Username string
	Password string

	lockID       string
	jsonLockInfo []byte
}

func (c *httpClient) httpRequest(method string, url *url.URL, data []byte, what string) (*http.Response, error) {
	var body interface{}
	if len(data) > 0 {
		body = data
	}

	log.Printf("[DEBUG] Executing HTTP remote state request for: %q", what)

	// Create the request
	req, err := retryablehttp.NewRequest(method, url.String(), body)
	if err != nil {
		return nil, fmt.Errorf("Failed to make %s HTTP request: %w", what, err)
	}

	// Add user-defined headers
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	// Work with data/body
	if len(data) > 0 {
		req.Header.Set("Content-Type", "application/json")

		// Generate the MD5
		hash := md5.Sum(data)
		b64 := base64.StdEncoding.EncodeToString(hash[:])
		req.Header.Set("Content-MD5", b64)
	}

	// Make the request
	resp, err := c.Client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("Failed to %s: %w", what, err)
	}

	log.Printf("[DEBUG] HTTP remote state request for %q returned status code: %d", what, resp.StatusCode)
	log.Printf("[DEBUG] HTTP response headers: %s", parseHeadersForLog(resp))

	return resp, nil
}

func (c *httpClient) Lock(info *statemgr.LockInfo) (string, error) {
	if c.LockURL == nil {
		return "", nil
	}
	c.lockID = ""

	jsonLockInfo := info.Marshal()
	resp, err := c.httpRequest(c.LockMethod, c.LockURL, jsonLockInfo, "lock")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		c.lockID = info.ID
		c.jsonLockInfo = jsonLockInfo
		return info.ID, nil
	case http.StatusUnauthorized:
		log.Printf("[DEBUG] LOCK, Unauthorized: %s", parseResponseBodyForLog(resp))
		return "", fmt.Errorf("HTTP remote state endpoint requires auth")
	case http.StatusForbidden:
		log.Printf("[DEBUG] LOCK, Forbidden: %s", parseResponseBodyForLog(resp))
		return "", fmt.Errorf("HTTP remote state endpoint invalid auth")
	case http.StatusConflict, http.StatusLocked:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", &statemgr.LockError{
				Info: info,
				Err:  fmt.Errorf("HTTP remote state already locked, failed to read body"),
			}
		}
		existing := statemgr.LockInfo{}
		err = json.Unmarshal(body, &existing)
		if err != nil {
			return "", &statemgr.LockError{
				Info: info,
				Err:  fmt.Errorf("HTTP remote state already locked, failed to unmarshal body"),
			}
		}
		return "", &statemgr.LockError{
			Info: &existing,
			Err:  fmt.Errorf("HTTP remote state already locked: ID=%s", existing.ID),
		}
	default:
		log.Printf("[DEBUG] LOCK, %d: %s", resp.StatusCode, parseResponseBodyForLog(resp))
		return "", fmt.Errorf("Unexpected HTTP response code %d", resp.StatusCode)
	}
}

func (c *httpClient) Unlock(id string) error {
	if c.UnlockURL == nil {
		return nil
	}

	resp, err := c.httpRequest(c.UnlockMethod, c.UnlockURL, c.jsonLockInfo, "unlock")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	default:
		log.Printf("[DEBUG] UNLOCK, %d: %s", resp.StatusCode, parseResponseBodyForLog(resp))
		return fmt.Errorf("Unexpected HTTP response code %d", resp.StatusCode)
	}
}

func (c *httpClient) Get() (*remote.Payload, error) {
	resp, err := c.httpRequest(http.MethodGet, c.URL, nil, "get state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Handle the common status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Handled after
	case http.StatusNoContent:
		return nil, nil
	case http.StatusNotFound:
		return nil, nil
	case http.StatusUnauthorized:
		log.Printf("[DEBUG] GET STATE, Unauthorized: %s", parseResponseBodyForLog(resp))
		return nil, fmt.Errorf("HTTP remote state endpoint requires auth")
	case http.StatusForbidden:
		log.Printf("[DEBUG] GET STATE, Forbidden: %s", parseResponseBodyForLog(resp))
		return nil, fmt.Errorf("HTTP remote state endpoint invalid auth")
	case http.StatusInternalServerError:
		log.Printf("[DEBUG] GET STATE, Internal Server Error: %s", parseResponseBodyForLog(resp))
		return nil, fmt.Errorf("HTTP remote state internal server error")
	default:
		log.Printf("[DEBUG] GET STATE, %d: %s", resp.StatusCode, parseResponseBodyForLog(resp))
		return nil, fmt.Errorf("Unexpected HTTP response code %d", resp.StatusCode)
	}

	// Read in the body
	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, fmt.Errorf("Failed to read remote state: %w", err)
	}

	// Create the payload
	payload := &remote.Payload{
		Data: buf.Bytes(),
	}

	// If there was no data, then return nil
	if len(payload.Data) == 0 {
		return nil, nil
	}

	// Check for the MD5
	if raw := resp.Header.Get("Content-MD5"); raw != "" {
		md5, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf(
				"Failed to decode Content-MD5 '%s': %w", raw, err)
		}

		payload.MD5 = md5
	} else {
		// Generate the MD5
		hash := md5.Sum(payload.Data)
		payload.MD5 = hash[:]
	}

	return payload, nil
}

func (c *httpClient) Put(data []byte) error {
	// Copy the target URL
	base := *c.URL

	if c.lockID != "" {
		query := base.Query()
		query.Set("ID", c.lockID)
		base.RawQuery = query.Encode()
	}

	/*
		// Set the force query parameter if needed
		if force {
			values := base.Query()
			values.Set("force", "true")
			base.RawQuery = values.Encode()
		}
	*/

	var method string = "POST"
	if c.UpdateMethod != "" {
		method = c.UpdateMethod
	}
	resp, err := c.httpRequest(method, &base, data, "upload state")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle the error codes
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		log.Printf("[DEBUG] UPLOAD STATE, %d: %s", resp.StatusCode, parseResponseBodyForLog(resp))
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}
}

func (c *httpClient) Delete() error {
	// Make the request
	resp, err := c.httpRequest(http.MethodDelete, c.URL, nil, "delete state")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle the error codes
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	default:
		log.Printf("[DEBUG] DELETE STATE, %d: %s", resp.StatusCode, parseResponseBodyForLog(resp))
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}
}

func (c *httpClient) IsLockingEnabled() bool {
	return c.UnlockURL != nil
}
