// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/terramate-io/opentofulib/internal/httpclient"
	"github.com/terramate-io/opentofulib/internal/registry/regsrc"
	"github.com/terramate-io/opentofulib/internal/registry/test"
	tfversion "github.com/terramate-io/opentofulib/version"
)

func TestConfigureDiscoveryRetry(t *testing.T) {
	t.Run("default retry", func(t *testing.T) {
		if discoveryRetry != defaultRetry {
			t.Fatalf("expected retry %q, got %q", defaultRetry, discoveryRetry)
		}

		rc := NewClient(nil, nil)
		if rc.client.RetryMax != defaultRetry {
			t.Fatalf("expected client retry %q, got %q",
				defaultRetry, rc.client.RetryMax)
		}
	})

	t.Run("configured retry", func(t *testing.T) {
		defer func() {
			discoveryRetry = defaultRetry
		}()
		t.Setenv(registryDiscoveryRetryEnvName, "2")

		configureDiscoveryRetry()
		expected := 2
		if discoveryRetry != expected {
			t.Fatalf("expected retry %q, got %q",
				expected, discoveryRetry)
		}

		rc := NewClient(nil, nil)
		if rc.client.RetryMax != expected {
			t.Fatalf("expected client retry %q, got %q",
				expected, rc.client.RetryMax)
		}
	})
}

func TestConfigureRegistryClientTimeout(t *testing.T) {
	t.Run("default timeout", func(t *testing.T) {
		if requestTimeout != defaultRequestTimeout {
			t.Fatalf("expected timeout %q, got %q",
				defaultRequestTimeout.String(), requestTimeout.String())
		}

		rc := NewClient(nil, nil)
		if rc.client.HTTPClient.Timeout != defaultRequestTimeout {
			t.Fatalf("expected client timeout %q, got %q",
				defaultRequestTimeout.String(), rc.client.HTTPClient.Timeout.String())
		}
	})

	t.Run("configured timeout", func(t *testing.T) {
		defer func() {
			requestTimeout = defaultRequestTimeout
		}()
		t.Setenv(registryClientTimeoutEnvName, "20")

		configureRequestTimeout()
		expected := 20 * time.Second
		if requestTimeout != expected {
			t.Fatalf("expected timeout %q, got %q",
				expected, requestTimeout.String())
		}

		rc := NewClient(nil, nil)
		if rc.client.HTTPClient.Timeout != expected {
			t.Fatalf("expected client timeout %q, got %q",
				expected, rc.client.HTTPClient.Timeout.String())
		}
	})
}

func TestLookupModuleVersions(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	// test with and without a hostname
	for _, src := range []string{
		"example.com/test-versions/name/provider",
		"test-versions/name/provider",
	} {
		modsrc, err := regsrc.ParseModuleSource(src)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := client.ModuleVersions(context.Background(), modsrc)
		if err != nil {
			t.Fatal(err)
		}

		if len(resp.Modules) != 1 {
			t.Fatal("expected 1 module, got", len(resp.Modules))
		}

		mod := resp.Modules[0]
		name := "test-versions/name/provider"
		if mod.Source != name {
			t.Fatalf("expected module name %q, got %q", name, mod.Source)
		}

		if len(mod.Versions) != 4 {
			t.Fatal("expected 4 versions, got", len(mod.Versions))
		}

		for _, v := range mod.Versions {
			_, err := version.NewVersion(v.Version)
			if err != nil {
				t.Fatalf("invalid version %q: %s", v.Version, err)
			}
		}
	}
}

func TestInvalidRegistry(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	src := "non-existent.localhost.localdomain/test-versions/name/provider"
	modsrc, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.ModuleVersions(context.Background(), modsrc); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegistryAuth(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	src := "private/name/provider"
	mod, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ModuleVersions(context.Background(), mod)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ModuleLocation(context.Background(), mod, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Also test without a credentials source
	client.services.SetCredentialsSource(nil)

	// both should fail without auth
	_, err = client.ModuleVersions(context.Background(), mod)
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = client.ModuleLocation(context.Background(), mod, "1.0.0")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLookupModuleLocationRelative(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	src := "relative/foo/bar"
	mod, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.ModuleLocation(context.Background(), mod, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}

	want := server.URL + "/relative-path"
	if got != want {
		t.Errorf("wrong location %s; want %s", got, want)
	}
}

func TestAccLookupModuleVersions(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip()
	}
	regDisco := disco.New()
	regDisco.SetUserAgent(httpclient.OpenTofuUserAgent(tfversion.String()))

	// test with and without a hostname
	for _, src := range []string{
		"terraform-aws-modules/vpc/aws",
		regsrc.PublicRegistryHost.String() + "/terraform-aws-modules/vpc/aws",
	} {
		modsrc, err := regsrc.ParseModuleSource(src)
		if err != nil {
			t.Fatal(err)
		}

		s := NewClient(regDisco, nil)
		resp, err := s.ModuleVersions(context.Background(), modsrc)
		if err != nil {
			t.Fatal(err)
		}

		if len(resp.Modules) != 1 {
			t.Fatal("expected 1 module, got", len(resp.Modules))
		}

		mod := resp.Modules[0]
		name := "terraform-aws-modules/vpc/aws"
		if mod.Source != name {
			t.Fatalf("expected module name %q, got %q", name, mod.Source)
		}

		if len(mod.Versions) == 0 {
			t.Fatal("expected multiple versions, got 0")
		}

		for _, v := range mod.Versions {
			_, err := version.NewVersion(v.Version)
			if err != nil {
				t.Fatalf("invalid version %q: %s", v.Version, err)
			}
		}
	}
}

// the error should reference the config source exactly, not the discovered path.
func TestLookupLookupModuleError(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	// this should not be found in the registry
	src := "bad/local/path"
	mod, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}

	// Instrument CheckRetry to make sure 404s are not retried
	retries := 0
	oldCheck := client.client.CheckRetry
	client.client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if retries > 0 {
			t.Fatal("retried after module not found")
		}
		retries++
		return oldCheck(ctx, resp, err)
	}

	_, err = client.ModuleLocation(context.Background(), mod, "0.2.0")
	if err == nil {
		t.Fatal("expected error")
	}

	// check for the exact quoted string to ensure we didn't prepend a hostname.
	if !strings.Contains(err.Error(), `"bad/local/path"`) {
		t.Fatal("error should not include the hostname. got:", err)
	}
}

func TestLookupModuleRetryError(t *testing.T) {
	server := test.RegistryRetryableErrorsServer()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	src := "example.com/test-versions/name/provider"
	modsrc, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ModuleVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected requests to exceed retry", err)
	}
	if resp != nil {
		t.Fatal("unexpected response", *resp)
	}

	// verify maxRetryErrorHandler handler returned the error
	if !strings.Contains(err.Error(), "the request failed after 2 attempts, please try again later") {
		t.Fatal("unexpected error, got:", err)
	}
}

func TestLookupModuleNoRetryError(t *testing.T) {
	// Disable retries
	discoveryRetry = 0
	defer configureDiscoveryRetry()

	server := test.RegistryRetryableErrorsServer()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	src := "example.com/test-versions/name/provider"
	modsrc, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ModuleVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected request to fail", err)
	}
	if resp != nil {
		t.Fatal("unexpected response", *resp)
	}

	// verify maxRetryErrorHandler handler returned the error
	if !strings.Contains(err.Error(), "the request failed, please try again later") {
		t.Fatal("unexpected error, got:", err)
	}
}

func TestLookupModuleNetworkError(t *testing.T) {
	server := test.RegistryRetryableErrorsServer()
	client := NewClient(test.Disco(server), nil)

	// Shut down the server to simulate network failure
	server.Close()

	src := "example.com/test-versions/name/provider"
	modsrc, err := regsrc.ParseModuleSource(src)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ModuleVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected request to fail", err)
	}
	if resp != nil {
		t.Fatal("unexpected response", *resp)
	}

	// verify maxRetryErrorHandler handler returned the correct error
	if !strings.Contains(err.Error(), "the request failed after 2 attempts, please try again later") {
		t.Fatal("unexpected error, got:", err)
	}
}

func TestModuleLocation_readRegistryResponse(t *testing.T) {
	cases := map[string]struct {
		src                  string
		httpClient           *http.Client
		registryFlags        []uint8
		want                 string
		wantErrorStr         string
		wantToReadFromHeader bool
		wantStatusCode       int
	}{
		"shall find the module location in the registry response body": {
			src:            "exists-in-registry/identifier/provider",
			want:           "file:///registry/exists",
			wantStatusCode: http.StatusOK,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{},
			},
		},
		"shall find the module location in the registry response header": {
			src:                  "exists-in-registry/identifier/provider",
			registryFlags:        []uint8{test.WithModuleLocationInHeader},
			want:                 "file:///registry/exists",
			wantToReadFromHeader: true,
			wantStatusCode:       http.StatusNoContent,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{},
			},
		},
		"shall read location from the registry response body even if the header with location address is also set": {
			src:                  "exists-in-registry/identifier/provider",
			want:                 "file:///registry/exists",
			wantStatusCode:       http.StatusOK,
			wantToReadFromHeader: false,
			registryFlags:        []uint8{test.WithModuleLocationInBody, test.WithModuleLocationInHeader},
			httpClient: &http.Client{
				Transport: &mockRoundTripper{},
			},
		},
		"shall fail to find the module": {
			src: "not-exist/identifier/provider",
			// note that the version is fixed in the mock
			// see: /internal/registry/test/mock_registry.go:testMods
			wantErrorStr:   `module "not-exist/identifier/provider" version "0.2.0" not found`,
			wantStatusCode: http.StatusNotFound,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{},
			},
		},
		"shall fail because of reading response body error": {
			src:            "foo/bar/baz",
			wantErrorStr:   "error reading response body from registry: foo",
			wantStatusCode: http.StatusOK,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{
					forwardResponse: &http.Response{
						StatusCode: http.StatusOK,
						Body:       mockErrorReadCloser{err: errors.New("foo")},
					},
				},
			},
		},
		"shall fail to deserialize JSON response": {
			src:            "foo/bar/baz",
			wantErrorStr:   `module "foo/bar/baz" version "0.2.0" failed to deserialize response body {: unexpected end of JSON input`,
			wantStatusCode: http.StatusOK,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{
					forwardResponse: &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("{")),
					},
				},
			},
		},
		"shall fail because of unexpected protocol change - 422 http status": {
			src:            "foo/bar/baz",
			wantErrorStr:   `error getting download location for "foo/bar/baz": foo resp:bar`,
			wantStatusCode: http.StatusUnprocessableEntity,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{
					forwardResponse: &http.Response{
						StatusCode: http.StatusUnprocessableEntity,
						Status:     "foo",
						Body:       io.NopCloser(strings.NewReader("bar")),
					},
				},
			},
		},
		"shall fail because location is not found in the response": {
			src:            "foo/bar/baz",
			wantErrorStr:   `failed to get download URL for "foo/bar/baz": OK resp:{"foo":"git::https://github.com/foo/terraform-baz-bar?ref=v0.2.0"}`,
			wantStatusCode: http.StatusOK,
			httpClient: &http.Client{
				Transport: &mockRoundTripper{
					forwardResponse: &http.Response{
						StatusCode: http.StatusOK,
						Status:     "OK",
						// note that the response emulates a contract change
						Body: io.NopCloser(strings.NewReader(`{"foo":"git::https://github.com/foo/terraform-baz-bar?ref=v0.2.0"}`)),
					},
				},
			},
		},
	}

	t.Parallel()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := test.Registry(tc.registryFlags...)
			defer server.Close()

			client := NewClient(test.Disco(server), tc.httpClient)

			mod, err := regsrc.ParseModuleSource(tc.src)
			if err != nil {
				t.Fatal(err)
			}

			got, err := client.ModuleLocation(context.Background(), mod, "0.2.0")
			if err != nil && tc.wantErrorStr == "" {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && err.Error() != tc.wantErrorStr {
				t.Fatalf("unexpected error content: want=%s, got=%v", tc.wantErrorStr, err)
			}
			if got != tc.want {
				t.Fatalf("unexpected location: want=%s, got=%v", tc.want, got)
			}

			gotStatusCode := tc.httpClient.Transport.(*mockRoundTripper).reverseResponse.StatusCode
			if tc.wantStatusCode != gotStatusCode {
				t.Fatalf("unexpected response status code: want=%d, got=%d", tc.wantStatusCode, gotStatusCode)
			}

			if tc.wantToReadFromHeader {
				resp := tc.httpClient.Transport.(*mockRoundTripper).reverseResponse
				if !reflect.DeepEqual(resp.Body, http.NoBody) {
					t.Fatalf("expected no body")
				}
			}
		})
	}
}

type mockRoundTripper struct {
	// response to return without calling the server
	// SET TO USE AS A REVERSE PROXY
	forwardResponse *http.Response
	// the response from the server will be written here
	// DO NOT SET
	reverseResponse *http.Response
	err             error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.forwardResponse != nil {
		m.reverseResponse = m.forwardResponse
		return m.forwardResponse, nil
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	m.reverseResponse = resp
	return resp, err
}

type mockErrorReadCloser struct {
	err error
}

func (m mockErrorReadCloser) Read(_ []byte) (n int, err error) {
	return 0, m.err
}

func (m mockErrorReadCloser) Close() error {
	return m.err
}
