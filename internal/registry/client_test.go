// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/registry/regsrc"
	"github.com/opentofu/opentofu/internal/registry/test"
	tfversion "github.com/opentofu/opentofu/version"
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
		defer func(retryEnv string) {
			os.Setenv(registryDiscoveryRetryEnvName, retryEnv)
			discoveryRetry = defaultRetry
		}(os.Getenv(registryDiscoveryRetryEnvName))
		os.Setenv(registryDiscoveryRetryEnvName, "2")

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
		defer func(timeoutEnv string) {
			os.Setenv(registryClientTimeoutEnvName, timeoutEnv)
			requestTimeout = defaultRequestTimeout
		}(os.Getenv(registryClientTimeoutEnvName))
		os.Setenv(registryClientTimeoutEnvName, "20")

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

func TestModuleLocation(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(test.Disco(server), nil)

	cases := map[string]struct {
		src          string
		wantErrorStr string
	}{
		"shall find the module relying on the OpenTofu registry protocol": {
			src: "exists-in-registry/identifier/provider",
		},
		"shall find the module relying on fallback - OpenTofu alpha registry protocol": {
			src: "alpha/identifier/provider",
		},
		"shall fail to find the module": {
			src: "not-exist/identifier/provider",
			// note that the version is fixed in the mock
			// see: /internal/registry/test/mock_registry.go:testMods
			wantErrorStr: `module "not-exist/identifier/provider" version 0.2.0: not found`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mod, err := regsrc.ParseModuleSource(tc.src)
			if err != nil {
				t.Fatal(err)
			}

			_, err = client.ModuleLocation(context.Background(), mod, "0.2.0")
			if err != nil && tc.wantErrorStr == "" {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && err.Error() != tc.wantErrorStr {
				t.Fatalf("unexpected error content: want=%s, got=%v", tc.wantErrorStr, err)
			}
		})
	}
}

func Test_readModuleLocation(t *testing.T) {
	newHeader := func(m map[string]string) http.Header {
		o := http.Header{}
		for k, v := range m {
			o.Set(k, v)
		}
		return o
	}

	cases := map[string]struct {
		arg          *http.Response
		want         string
		wantErrorStr string
	}{
		"shall return location from the response body - OpenTofu stable registry protocol": {
			arg: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"location":"git::https://github.com/foo/terraform-bar-baz?ref=v0.0.1"}`)),
			},
			want: "git::https://github.com/foo/terraform-bar-baz?ref=v0.0.1",
		},
		"shall return location from the header - OpenTofu alpha registry protocol": {
			arg: &http.Response{
				StatusCode: http.StatusNoContent,
				Header: newHeader(map[string]string{
					xTerraformGet: "git::https://github.com/foo/terraform-bar-baz?ref=v0.0.1",
				}),
				Body: http.NoBody,
			},
			want: "git::https://github.com/foo/terraform-bar-baz?ref=v0.0.1",
		},
		"shall fail to read response body": {
			arg: &http.Response{
				StatusCode: http.StatusOK,
				Body:       mockReadCloser{err: errors.New("foo")},
			},
			wantErrorStr: "error reading response body from registry: foo",
		},
		"shall fail to deserialize JSON": {
			arg: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{")),
			},
			wantErrorStr: "error deserializing {: unexpected end of JSON input",
		},
		"shall fail because no version found": {
			arg: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       http.NoBody,
			},
			wantErrorStr: "not found",
		},
		"shall fail - 500 status code": {
			arg: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "foo",
				Body:       io.NopCloser(strings.NewReader("some server error")),
			},
			wantErrorStr: "error getting download location: foo resp:some server error",
		},
		"shall fail because no location was found neither in body, nor in header": {
			arg: &http.Response{
				StatusCode: http.StatusOK,
				Status:     "foo",
				Body:       io.NopCloser(strings.NewReader("{}")),
			},
			wantErrorStr: "failed to get download URL: foo resp:{}",
		},
	}

	t.Parallel()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := readModuleLocation(tc.arg)
			if err != nil && tc.wantErrorStr == "" {
				t.Fatalf("readModuleLocation() unexpected error")
			}
			if err != nil && err.Error() != tc.wantErrorStr {
				t.Fatalf("readModuleLocation() error = %v, wantErrorStr %s", err, tc.wantErrorStr)
			}
			if got != tc.want {
				t.Errorf("readModuleLocation() got = %v, want %v", got, tc.want)
			}
		})
	}
}

type mockReadCloser struct {
	err error
	v   []byte
}

func (m mockReadCloser) Read(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	return copy(p, m.v), nil
}

func (m mockReadCloser) Close() error {
	return m.err
}
