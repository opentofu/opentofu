package gitlab

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states/remote"
)

func TestRemoteClientContract(_ *testing.T) {
	var _ remote.Client = new(RemoteClient)
	var _ remote.ClientLocker = new(RemoteClient)
}

func stateKeyFromRequest(r *http.Request) (string, bool, error) {
	// Parse the raw URL path to extract path segments.
	segments := strings.SplitN(r.URL.RawPath, "/", 9)[1:]

	// /api/v4/projects/{state}/terraform/state/{default}[/lock]
	lengthMatches := len(segments) >= 7 && len(segments) <= 8
	isProjectPath := slices.Equal(segments[0:3], []string{"api", "v4", "projects"})
	isStatePath := slices.Equal(segments[4:6], []string{"terraform", "state"})
	isLockRequest := segments[len(segments)-1] == "lock"

	// Only ever match Gitlab Terraform state paths.
	if !(lengthMatches && isProjectPath && isStatePath) {
		return "", false, fmt.Errorf("not a valid gitlab terraform state path")
	}

	projectName, _ := url.PathUnescape(segments[3])
	stateName, _ := url.PathUnescape(segments[6])

	// Generate a unique key to represent this project/state.
	key := fmt.Sprintf("%s:%s", projectName, stateName)

	return key, isLockRequest, nil
}

func gitlabStateValueHandler() http.HandlerFunc {
	valueStorage := map[string][]byte{}
	return func(w http.ResponseWriter, r *http.Request) {
		// Always close request body.
		defer r.Body.Close()

		stateKey, lockRequest, err := stateKeyFromRequest(r)

		if lockRequest || err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch r.Method {
		// Retrieve state.
		case http.MethodGet:
			currentValue, exists := valueStorage[stateKey]
			if exists {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(currentValue)
			} else {
				w.WriteHeader(http.StatusNoContent)
				_, _ = w.Write([]byte{})
			}
		// Update state.
		case http.MethodPost:
			currentValue, exists := valueStorage[stateKey]
			stateValue, _ := io.ReadAll(r.Body)

			if !exists {
				valueStorage[stateKey] = stateValue
				w.WriteHeader(http.StatusCreated)
			} else {
				// Check if the existing state matches what was sent.
				if bytes.Equal(currentValue, stateValue) {
					w.WriteHeader(http.StatusNoContent)
				} else {
					valueStorage[stateKey] = stateValue
					w.WriteHeader(http.StatusOK)
				}
			}
		// Delete state.
		case http.MethodDelete:
			delete(valueStorage, stateKey)
			w.WriteHeader(http.StatusOK)
		}
	}
}

func TestRemoteClientOperations(t *testing.T) {
	testServer := httptest.NewServer(gitlabStateValueHandler())
	defer testServer.Close()

	url, err := url.Parse(testServer.URL)

	if err != nil {
		t.Fatalf("Parse: %s", err)
	}

	client := &RemoteClient{
		HTTPClient: retryablehttp.NewClient(),
		StateName:  backend.DefaultStateName,
		BaseURL:    url,
		Project:    "test/with/slashes",
	}

	remote.TestClient(t, client)
}

func gitlabLockStateHandler() http.HandlerFunc {
	lockStorage := map[string][]byte{}
	return func(w http.ResponseWriter, r *http.Request) {
		// Always close request body.
		defer r.Body.Close()

		stateKey, lockRequest, err := stateKeyFromRequest(r)

		if !lockRequest || err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		lockValue, _ := io.ReadAll(r.Body)

		switch r.Method {
		// Acquire lock.
		case http.MethodPost:
			currentLock, locked := lockStorage[stateKey]
			if locked {
				w.WriteHeader(http.StatusLocked)
				_, _ = w.Write(currentLock)
			} else {
				lockStorage[stateKey] = lockValue
				w.WriteHeader(http.StatusOK)
			}
		// Relinquish lock.
		case http.MethodDelete:
			currentLock := lockStorage[stateKey]
			if !bytes.Equal(lockValue, currentLock) {
				w.WriteHeader(http.StatusConflict)
			} else {
				delete(lockStorage, stateKey)
				w.WriteHeader(http.StatusOK)
			}
		}
	}
}

func TestRemoteClientLocks(t *testing.T) {
	testServer := httptest.NewServer(gitlabLockStateHandler())
	defer testServer.Close()

	url, err := url.Parse(testServer.URL)

	if err != nil {
		t.Fatalf("Parse: %s", err)
	}

	firstClient := &RemoteClient{
		HTTPClient: retryablehttp.NewClient(),
		BaseURL:    url,
		Project:    "test/with/slashes",
		StateName:  backend.DefaultStateName,
	}

	secondClient := &RemoteClient{
		HTTPClient: retryablehttp.NewClient(),
		BaseURL:    url,
		Project:    "test/with/slashes",
		StateName:  backend.DefaultStateName,
	}

	remote.TestRemoteLocks(t, firstClient, secondClient)
}
