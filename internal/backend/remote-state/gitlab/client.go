package gitlab

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

type RemoteClient struct {
	HTTPClient *retryablehttp.Client
	BaseURL    *url.URL
	Project    string
	StateName  string

	lockID       string
	jsonLockInfo []byte
}

func (client *RemoteClient) buildStateURL(extra ...string) *url.URL {
	// Escape the project and state names as URL path components.
	projectName := url.PathEscape(client.Project)
	stateName := url.PathEscape(client.StateName)

	// /api/v4/projects/{projectName}/terraform/state/{stateName}
	pathComponents := []string{
		"api", "v4",
		"projects", projectName,
		"terraform", "state", stateName,
	}

	// Append additional path components (ie: lock)
	pathComponents = append(pathComponents, extra...)

	return client.BaseURL.JoinPath(pathComponents...)
}

func (client *RemoteClient) httpRequest(method string, url *url.URL, data []byte, what string) (*http.Response, error) {
	req, err := retryablehttp.NewRequest(method, url.String(), data)

	if err != nil {
		return nil, fmt.Errorf("%s failed: create request failed: %w", what, err)
	}

	if len(data) > 0 {
		req.Header.Set("Content-Type", "application/json")

		hash := md5.Sum(data)
		digest := base64.StdEncoding.EncodeToString(hash[:])

		req.Header.Set("Content-MD5", digest)
	}

	resp, err := client.HTTPClient.Do(req)

	if err != nil {
		return nil, fmt.Errorf("%s failed: could not create request: %w", what, err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("%s failed: cannot read body: %w", what, err)
	}

	// Backfill a Content-MD5 header so we don't have to do it later.
	if raw := resp.Header.Get("Content-MD5"); raw == "" {
		hash := md5.Sum(body)
		digest := base64.StdEncoding.EncodeToString(hash[:])

		resp.Header.Set("Content-MD5", digest)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("%s failed: requires auth", what)
	case http.StatusForbidden:
		return nil, fmt.Errorf("%s failed: not authorized", what)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil, fmt.Errorf("%s failed: request not understood", what)
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("%s failed: internal server error", what)
	}

	// Reassign the response body with a new reader.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return resp, nil
}

func (client *RemoteClient) Get() (*remote.Payload, error) {
	stateURL := client.buildStateURL()

	resp, err := client.httpRequest(http.MethodGet, stateURL, nil, "get state")

	if err != nil {
		return nil, fmt.Errorf("gitlab remote state get failed: %w", err)
	}

	defer resp.Body.Close()

	// If there was no state or the state was empty then we're starting fresh.
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab remote state get failed: http response code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("failed to read remote state: %w", err)
	}

	// Empty data means fresh state.
	if len(data) == 0 {
		return nil, nil
	}

	raw := resp.Header.Get("Content-MD5")
	hash, err := base64.StdEncoding.DecodeString(raw)

	if err != nil {
		return nil, fmt.Errorf("could not decode Content-MD5 '%s': %w", raw, err)
	}

	payload := &remote.Payload{
		Data: data,
		MD5:  hash,
	}

	return payload, nil
}

func (client *RemoteClient) Put(data []byte) error {
	stateURL := client.buildStateURL()

	if client.lockID != "" {
		query := stateURL.Query()
		query.Set("ID", client.lockID)
		stateURL.RawQuery = query.Encode()
	}

	resp, err := client.httpRequest(http.MethodPost, stateURL, data, "put state")

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	// One of: created, updated, or nothing has changed.
	case http.StatusCreated, http.StatusOK, http.StatusNoContent:
		return nil
	}

	return fmt.Errorf("gitlab remote state delete failed: http response code %d", resp.StatusCode)
}

func (client *RemoteClient) Delete() error {
	resp, err := client.httpRequest(http.MethodDelete, client.buildStateURL(), nil, "delete state")

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf("gitlab remote state delete failed: http response code %d", resp.StatusCode)
}

func (client *RemoteClient) Lock(info *statemgr.LockInfo) (string, error) {
	client.lockID = ""
	jsonLockInfo := info.Marshal()
	lockURL := client.buildStateURL("lock")

	resp, err := client.httpRequest(http.MethodPost, lockURL, jsonLockInfo, "lock state")

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		client.lockID = info.ID
		client.jsonLockInfo = jsonLockInfo
		return info.ID, nil
	}

	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusLocked {
		body, err := io.ReadAll(resp.Body)

		if err != nil {
			return "", &statemgr.LockError{
				Info: info,
				Err:  fmt.Errorf("gitlab remote state already locked; failed to read body"),
			}
		}

		existing := statemgr.LockInfo{}
		err = json.Unmarshal(body, &existing)

		if err != nil {
			return "", &statemgr.LockError{
				Info: info,
				Err:  fmt.Errorf("gitlab remote state already locked; failed to unmarshal body"),
			}
		}

		return "", &statemgr.LockError{
			Info: info,
			Err:  fmt.Errorf("gitlab remote state already locked: id=%s", existing.ID),
		}
	}

	return "", fmt.Errorf("gitlab remote state lock failed: http response code %d", resp.StatusCode)
}

func (client *RemoteClient) Unlock(_ string) error {
	lockURL := client.buildStateURL("lock")

	resp, err := client.httpRequest(http.MethodDelete, lockURL, client.jsonLockInfo, "unlock state")

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf("gitlab remote state unlock failed: http response code %d", resp.StatusCode)
}
