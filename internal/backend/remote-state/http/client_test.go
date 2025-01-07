// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func TestHTTPClient_impl(t *testing.T) {
	var _ remote.Client = new(httpClient)
	var _ remote.ClientLocker = new(httpClient)
}

func TestHTTPClient(t *testing.T) {
	handler := new(testHTTPHandler)
	ts := httptest.NewServer(http.HandlerFunc(handler.Handle))
	defer ts.Close()

	url, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Parse: %s", err)
	}

	// Test basic get/update
	client := &httpClient{URL: url, Client: retryablehttp.NewClient()}
	remote.TestClient(t, client)

	// test just a single PUT
	p := &httpClient{
		URL:          url,
		UpdateMethod: "PUT",
		Client:       retryablehttp.NewClient(),
	}
	remote.TestClient(t, p)

	// Test headers
	c := retryablehttp.NewClient()
	c.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, _ int) {
		// Test user defined header is part of the request
		v := req.Header.Get("user-defined")
		if v != "test" {
			t.Fatalf("Expected header \"user-defined\" with value \"test\", got \"%s\"", v)
		}

		// Test the content-type header was not overridden
		v = req.Header.Get("content-type")
		if req.Method == "PUT" && v != "application/json" {
			t.Fatalf("Expected header \"content-type\" with value \"application/json\", got \"%s\"", v)
		}
	}

	p = &httpClient{
		URL:          url,
		UpdateMethod: "PUT",
		Headers: map[string]string{
			"user-defined": "test",
			"content-type": "application/xml",
		},
		Client: c,
	}

	remote.TestClient(t, p)

	// Test locking and alternative UpdateMethod
	a := &httpClient{
		URL:          url,
		UpdateMethod: "PUT",
		LockURL:      url,
		LockMethod:   "LOCK",
		UnlockURL:    url,
		UnlockMethod: "UNLOCK",
		Client:       retryablehttp.NewClient(),
	}
	b := &httpClient{
		URL:          url,
		UpdateMethod: "PUT",
		LockURL:      url,
		LockMethod:   "LOCK",
		UnlockURL:    url,
		UnlockMethod: "UNLOCK",
		Client:       retryablehttp.NewClient(),
	}
	remote.TestRemoteLocks(t, a, b)

	// test a WebDAV-ish backend
	davhandler := new(testHTTPHandler)
	ts = httptest.NewServer(http.HandlerFunc(davhandler.HandleWebDAV))
	defer ts.Close()

	url, err = url.Parse(ts.URL)
	client = &httpClient{
		URL:          url,
		UpdateMethod: "PUT",
		Client:       retryablehttp.NewClient(),
	}
	if err != nil {
		t.Fatalf("Parse: %s", err)
	}

	remote.TestClient(t, client) // first time through: 201
	remote.TestClient(t, client) // second time, with identical data: 204

	// test a broken backend
	brokenHandler := new(testBrokenHTTPHandler)
	brokenHandler.handler = new(testHTTPHandler)
	ts = httptest.NewServer(http.HandlerFunc(brokenHandler.Handle))
	defer ts.Close()

	url, err = url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Parse: %s", err)
	}
	client = &httpClient{URL: url, Client: retryablehttp.NewClient()}
	remote.TestClient(t, client)
}

type testHTTPHandler struct {
	Data   []byte
	Locked bool
}

func (h *testHTTPHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Write(h.Data)
	case "PUT":
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, r.Body); err != nil {
			w.WriteHeader(500)
		}
		w.WriteHeader(201)
		h.Data = buf.Bytes()
	case "POST":
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, r.Body); err != nil {
			w.WriteHeader(500)
		}
		h.Data = buf.Bytes()
	case "LOCK":
		if h.Locked {
			w.WriteHeader(423)
		} else {
			h.Locked = true
		}
	case "UNLOCK":
		h.Locked = false
	case "DELETE":
		h.Data = nil
		w.WriteHeader(200)
	default:
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("Unknown method: %s", r.Method)))
	}
}

// mod_dav-ish behavior
func (h *testHTTPHandler) HandleWebDAV(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Write(h.Data)
	case "PUT":
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, r.Body); err != nil {
			w.WriteHeader(500)
		}
		if reflect.DeepEqual(h.Data, buf.Bytes()) {
			h.Data = buf.Bytes()
			w.WriteHeader(204)
		} else {
			h.Data = buf.Bytes()
			w.WriteHeader(201)
		}
	case "DELETE":
		h.Data = nil
		w.WriteHeader(200)
	default:
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("Unknown method: %s", r.Method)))
	}
}

type testBrokenHTTPHandler struct {
	lastRequestWasBroken bool
	handler              *testHTTPHandler
}

func (h *testBrokenHTTPHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h.lastRequestWasBroken {
		h.lastRequestWasBroken = false
		h.handler.Handle(w, r)
	} else {
		h.lastRequestWasBroken = true
		w.WriteHeader(500)
	}
}

// Tests the IsLockingEnabled method for the HTTP client.
// It checks whether locking is enabled based on the presence of the UnlockURL.
func TestHttpClient_IsLockingEnabled(t *testing.T) {
	tests := []struct {
		name       string
		unlockURL  string
		wantResult bool
	}{
		{
			name:       "Locking enabled when UnlockURL is set",
			unlockURL:  "http://http-endpoint.com:3333",
			wantResult: true,
		},
		{
			name:       "Locking disabled when UnlockURL is nil",
			unlockURL:  "", // Empty string will result in nil *url.URL
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var unlockURL *url.URL
			if tt.unlockURL != "" {
				var err error
				unlockURL, err = url.Parse(tt.unlockURL)
				if err != nil {
					t.Fatalf("Failed to parse unlockURL: %v", err)
				}
			} else {
				unlockURL = nil
			}

			client := &httpClient{
				UnlockURL: unlockURL,
			}

			gotResult := client.IsLockingEnabled()
			if gotResult != tt.wantResult {
				t.Errorf("IsLockingEnabled() = %v; want %v", gotResult, tt.wantResult)
			}
		})
	}
}

// Tests the Lock method for the HTTP client.
// Test to see correct lock info is returned
func TestHttpClient_lock(t *testing.T) {
	stateLockInfoA := statemgr.LockInfo{
		ID:        "ada-lovelace-state-lock-id",
		Who:       "AdaLovelace",
		Operation: "TestTypePlan",
		Created:   time.Date(2023, time.August, 16, 15, 9, 26, 0, time.UTC),
	}

	stateLockInfoRemoteB := statemgr.LockInfo{
		ID:        "linus-torvalds-http-remote-state-lock-id",
		Who:       "LinusTorvalds",
		Operation: "TestTypePlan",
		Created:   time.Date(2024, time.August, 15, 9, 0, 26, 0, time.UTC),
	}

	testCases := []struct {
		name                string
		lockInfo            *statemgr.LockInfo
		lockResponseStatus  int
		lockResponseBody    []byte
		expectedStateLockID string
		expectedErrorMsg    error
	}{
		{
			// Successful locking HTTP remote state
			name:                "Successfully locked",
			lockInfo:            &stateLockInfoA,
			lockResponseStatus:  http.StatusOK,
			lockResponseBody:    nil,
			expectedStateLockID: stateLockInfoA.ID,
			expectedErrorMsg:    nil,
		},
		{
			// Failed to lock state, HTTP remote state already locked
			name:                "Locked remote state",
			lockInfo:            &stateLockInfoA,
			lockResponseStatus:  http.StatusLocked,
			lockResponseBody:    stateLockInfoRemoteB.Marshal(),
			expectedStateLockID: "",
			expectedErrorMsg: &statemgr.LockError{
				Info: &stateLockInfoRemoteB,
				Err:  fmt.Errorf("HTTP remote state already locked: ID=%s", stateLockInfoRemoteB.ID),
			},
		},
		{
			// Failed to lock state HTTP remote state already locked. No remote lock details returned
			name:                "Locked remote state failed to unmarshal body",
			lockInfo:            &stateLockInfoA,
			lockResponseStatus:  http.StatusLocked,
			lockResponseBody:    nil,
			expectedStateLockID: "",
			expectedErrorMsg: &statemgr.LockError{
				Info: &stateLockInfoA,
				Err:  fmt.Errorf("HTTP remote state already locked, failed to unmarshal body"),
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.lockResponseStatus)
				_, err := w.Write(tt.lockResponseBody)
				if err != nil {
					t.Fatalf("Failed to write response body: %v", err)
				}
			}

			ts := httptest.NewServer(http.HandlerFunc(handler))
			defer ts.Close()

			lockURL, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("Failed to parse lockURL: %v", err)
			}

			client := &httpClient{
				LockURL:    lockURL,
				LockMethod: "LOCK",
				Client:     retryablehttp.NewClient(),
			}

			lockID, err := client.Lock(tt.lockInfo)
			if tt.expectedErrorMsg != nil && err == nil {
				// no expected error
				t.Errorf("Lock() no expected error = %v", tt.expectedErrorMsg)
			}
			if tt.expectedErrorMsg == nil && err != nil {
				// unexpected error
				t.Errorf("Lock() unexpected error = %v", err)
			}
			if tt.expectedErrorMsg != nil && err.Error() != tt.expectedErrorMsg.Error() {
				// mismatched errors
				t.Errorf("Lock() error = %v, want %v", err, tt.expectedErrorMsg)
			}
			if lockID != tt.expectedStateLockID {
				t.Errorf("Lock() = %v, want %v", lockID, tt.expectedStateLockID)
			}
		})
	}
}
