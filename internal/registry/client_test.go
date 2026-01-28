// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-retryablehttp"
	version "github.com/hashicorp/go-version"
	regaddr "github.com/opentofu/registry-address/v2"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/registry/response"
	"github.com/opentofu/opentofu/internal/registry/test"
)

func TestLookupModuleVersions(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(t.Context(), test.Disco(server), nil)

	// test with and without a hostname
	for _, src := range []string{
		"example.com/test-versions/name/provider",
		"test-versions/name/provider",
	} {
		modsrc := testParseModulePackageAddr(t, src)
		resp, err := client.ModulePackageVersions(context.Background(), modsrc)
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

	client := NewClient(t.Context(), test.Disco(server), nil)

	src := "non-existent.localhost.localdomain/test-versions/name/provider"
	modsrc := testParseModulePackageAddr(t, src)

	if _, err := client.ModulePackageVersions(context.Background(), modsrc); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegistryAuth(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(t.Context(), test.Disco(server), nil)

	src := "private/name/provider"
	modsrc := testParseModulePackageAddr(t, src)

	_, err := client.ModulePackageVersions(context.Background(), modsrc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ModulePackageLocation(context.Background(), modsrc, "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}

	// Also test without a credentials source
	client.services.SetCredentialsSource(nil)

	// both should fail without auth
	_, err = client.ModulePackageVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = client.ModulePackageLocation(context.Background(), modsrc, "1.0.0", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLookupModuleLocationRelative(t *testing.T) {
	server := test.Registry()
	defer server.Close()

	client := NewClient(t.Context(), test.Disco(server), nil)

	src := "relative/foo/bar"
	modsrc := testParseModulePackageAddr(t, src)

	got, err := client.ModulePackageLocation(context.Background(), modsrc, "0.2.0", "")
	if err != nil {
		t.Fatal(err)
	}

	want := PackageLocationIndirect{
		SourceAddr: addrs.ModuleSourceRemote{
			Package: addrs.ModulePackage(server.URL + "/relative-path"),
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong location\n" + diff)
	}
}

func TestAccLookupModuleVersions(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip()
	}
	regDisco := disco.New(
		disco.WithHTTPClient(httpclient.New(t.Context())),
	)

	// test with and without a hostname
	for _, src := range []string{
		"terraform-aws-modules/vpc/aws",
		regaddr.DefaultModuleRegistryHost.String() + "/terraform-aws-modules/vpc/aws",
	} {
		modsrc := testParseModulePackageAddr(t, src)

		s := NewClient(t.Context(), regDisco, nil)
		resp, err := s.ModulePackageVersions(context.Background(), modsrc)
		if err != nil {
			t.Fatal(err)
		}

		if len(resp.Modules) != 1 {
			t.Fatal("expected 1 module, got", len(resp.Modules))
		}

		mod := resp.Modules[0]
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

	client := NewClient(t.Context(), test.Disco(server), nil)

	// this should not be found in the registry
	src := "bad/local/path"
	modsrc := testParseModulePackageAddr(t, src)

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

	_, err := client.ModulePackageLocation(context.Background(), modsrc, "0.2.0", "")
	if err == nil {
		t.Fatal("expected error")
	}

	// Check for the exact quoted string to ensure we prepended the registry
	// hostname. As a convention we always use the fully-qualified address
	// syntax when reporting errors because it's helpful for debugging to be
	// explicit about what registry we were trying to talk to.
	//
	// (Historically this code made the opposite decision and just echoed back
	// the provided source address exactly as given, but we later made a
	// cross-cutting decision to use canonical forms of addresses in error
	// messages unless we're reporting a syntax error in particular, which
	// is not the case here: this is a failed lookup of a valid address.)
	if !strings.Contains(err.Error(), `"registry.opentofu.org/bad/local/path"`) {
		t.Fatal("error should not include the hostname. got:", err)
	}
}

func TestLookupModuleRetryError(t *testing.T) {
	server := test.RegistryRetryableErrorsServer()
	defer server.Close()

	client := NewClient(t.Context(), test.Disco(server), nil)

	src := "example.com/test-versions/name/provider"
	modsrc := testParseModulePackageAddr(t, src)
	resp, err := client.ModulePackageVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected requests to exceed retry", err)
	}
	if resp != nil {
		t.Fatal("unexpected response", *resp)
	}

	// verify maxRetryErrorHandler handler returned the error
	if !strings.Contains(err.Error(), "request failed after 2 attempts") {
		t.Fatal("unexpected error, got:", err)
	}
}

func TestLookupModuleNoRetryError(t *testing.T) {
	server := test.RegistryRetryableErrorsServer()
	defer server.Close()

	client := NewClient(
		t.Context(), test.Disco(server),
		// Retries are disabled by the second argument to this function
		httpclient.NewForRegistryRequests(t.Context(), 0, 10*time.Second),
	)

	src := "example.com/test-versions/name/provider"
	modsrc := testParseModulePackageAddr(t, src)
	resp, err := client.ModulePackageVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected request to fail", err)
	}
	if resp != nil {
		t.Fatal("unexpected response", *resp)
	}

	// verify maxRetryErrorHandler handler returned the error
	if !strings.Contains(err.Error(), "request failed:") {
		t.Fatal("unexpected error, got:", err)
	}
}

func TestLookupModuleNetworkError(t *testing.T) {
	server := test.RegistryRetryableErrorsServer()
	client := NewClient(t.Context(), test.Disco(server), nil)

	// Shut down the server to simulate network failure
	server.Close()

	src := "example.com/test-versions/name/provider"
	modsrc := testParseModulePackageAddr(t, src)
	resp, err := client.ModulePackageVersions(context.Background(), modsrc)
	if err == nil {
		t.Fatal("expected request to fail", err)
	}
	if resp != nil {
		t.Fatal("unexpected response", *resp)
	}

	// verify maxRetryErrorHandler handler returned the correct error
	if !strings.Contains(err.Error(), "request failed after 2 attempts") {
		t.Fatal("unexpected error, got:", err)
	}
}

func TestModuleLocation_readRegistryResponse(t *testing.T) {
	makeIndirectLocation := func(packageAddr string, subDir string) PackageLocationIndirect {
		return PackageLocationIndirect{
			SourceAddr: addrs.ModuleSourceRemote{
				Package: addrs.ModulePackage(packageAddr),
				Subdir:  subDir,
			},
		}
	}
	mustParseURL := func(s string) *url.URL {
		ret, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return ret
	}

	cases := map[string]struct {
		src                  string
		handlerFunc          func(w http.ResponseWriter, r *http.Request)
		registryFlags        []uint8
		want                 PackageLocation
		wantErrorStr         string
		wantToReadFromHeader bool
		wantStatusCode       int
	}{
		"shall find direct module location in the registry response body, opting to use the registry's credentials": {
			src: "exists-in-registry/identifier/provider",
			want: PackageLocationDirect{
				packageAddr:            testParseModulePackageAddr(t, "exists-in-registry/identifier/provider"),
				packageURL:             mustParseURL("https://example.com/package.zip"),
				useRegistryCredentials: true,
			},
			wantStatusCode: http.StatusOK,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"location":"https://example.com/package.zip","use_registry_credentials":true}`))
			},
		},
		"shall find direct module location in the registry response body, not opting to use the registry's credentials": {
			src: "exists-in-registry/identifier/provider",
			want: PackageLocationDirect{
				packageAddr:            testParseModulePackageAddr(t, "exists-in-registry/identifier/provider"),
				packageURL:             mustParseURL("https://example.com/package.zip"),
				useRegistryCredentials: false,
			},
			wantStatusCode: http.StatusOK,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"location":"https://example.com/package.zip","use_registry_credentials":false}`))
			},
		},
		"shall find indirect module location in the registry response body": {
			src:            "exists-in-registry/identifier/provider",
			want:           makeIndirectLocation("file:///registry/exists", ""),
			wantStatusCode: http.StatusOK,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response.ModuleLocationRegistryResp{Location: "file:///registry/exists"})
			},
		},
		"shall find indirect module location in the registry response header": {
			src:                  "exists-in-registry/identifier/provider",
			registryFlags:        []uint8{test.WithModuleLocationInHeader},
			want:                 makeIndirectLocation("file:///registry/exists", ""),
			wantToReadFromHeader: true,
			wantStatusCode:       http.StatusNoContent,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Terraform-Get", "file:///registry/exists")
				w.WriteHeader(http.StatusNoContent)
			},
		},
		"shall read indirect location from the registry response body even if the header with location address is also set": {
			src:                  "exists-in-registry/identifier/provider",
			want:                 makeIndirectLocation("file:///registry/exists", ""),
			wantStatusCode:       http.StatusOK,
			wantToReadFromHeader: false,
			registryFlags:        []uint8{test.WithModuleLocationInBody, test.WithModuleLocationInHeader},
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Terraform-Get", "file:///registry/exists-header")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response.ModuleLocationRegistryResp{Location: "file:///registry/exists"})
			},
		},
		"shall fail to find the module": {
			src: "not-exist/identifier/provider",
			// note that the version is fixed in the mock
			// see: /internal/registry/test/mock_registry.go:testMods
			wantErrorStr:   `module "registry.opentofu.org/not-exist/identifier/provider" version "0.2.0" not found`,
			wantStatusCode: http.StatusNotFound,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		},
		"shall fail because of reading response body error": {
			src:            "foo/bar/baz",
			wantErrorStr:   "error reading response body from registry",
			wantStatusCode: http.StatusOK,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "1000") // Set incorrect content length
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{")) // Only write a partial response
				// The connection will close after handler returns, but client will expect more data
			},
		},
		"shall fail to deserialize JSON response": {
			src:            "foo/bar/baz",
			wantErrorStr:   `module "registry.opentofu.org/foo/bar/baz" version "0.2.0" failed to deserialize response body {: unexpected end of JSON input`,
			wantStatusCode: http.StatusOK,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{"))
			},
		},
		"shall fail because of unexpected protocol change - 422 http status": {
			src:            "foo/bar/baz",
			wantErrorStr:   `error getting download location for "registry.opentofu.org/foo/bar/baz": 422 Unprocessable Entity resp:bar`,
			wantStatusCode: http.StatusUnprocessableEntity,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte("bar"))
			},
		},
		"shall fail because location is not found in the response": {
			src:            "foo/bar/baz",
			wantErrorStr:   `registry did not return a location for this package`,
			wantStatusCode: http.StatusOK,
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				// note that the response emulates a contract change
				_, _ = w.Write([]byte(`{"foo":"git::https://github.com/foo/terraform-baz-bar?ref=v0.2.0"}`))
			},
		},
	}

	t.Parallel()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(tc.handlerFunc))
			defer mockServer.Close()

			registryServer := test.Registry(tc.registryFlags...)
			defer registryServer.Close()

			transport := &testTransport{
				mockURL: mockServer.URL,
			}
			httpClient := retryablehttp.NewClient()
			httpClient.HTTPClient.Transport = transport
			client := NewClient(t.Context(), test.Disco(registryServer), httpClient)
			modsrc := testParseModulePackageAddr(t, tc.src)

			got, err := client.ModulePackageLocation(context.Background(), modsrc, "0.2.0", "")

			// Validate the results
			if err != nil && tc.wantErrorStr == "" {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && !strings.Contains(err.Error(), tc.wantErrorStr) {
				t.Fatalf("unexpected error content: want=%s, got=%v", tc.wantErrorStr, err)
			}
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(PackageLocationDirect{})); diff != "" {
				t.Fatal("unexpected location\n" + diff)
			}

			// Verify status code if we have a successful response
			if transport.lastResponse != nil {
				gotStatusCode := transport.lastResponse.StatusCode
				if tc.wantStatusCode != gotStatusCode {
					t.Fatalf("unexpected response status code: want=%d, got=%d", tc.wantStatusCode, gotStatusCode)
				}

				// Check if we expected to read from header
				if tc.wantToReadFromHeader && err == nil {
					headerVal := transport.lastResponse.Header.Get("X-Terraform-Get")
					if headerVal == "" {
						t.Fatalf("expected to read location from header but X-Terraform-Get header was not set")
					}
				}
			}
		})
	}
}

func testParseModulePackageAddr(t *testing.T, src string) regaddr.ModulePackage {
	sourceAddr, err := addrs.ParseModuleSourceRegistry(src)
	if err != nil {
		t.Fatalf("invalid module package address %q: %s", src, err)
	}
	// We used ParseModuleSourceRegistry and it didn't return an error, so this
	// type assertion should always succeed.
	registrySourceAddr := sourceAddr.(addrs.ModuleSourceRegistry)
	if registrySourceAddr.Subdir != "" {
		// Handling the "subdir" part of a source address is not part of
		// the registry client's scope: that must be handled by the main
		// module installer code after the registry client deals with
		// the module-package-level questions.
		t.Fatalf("invalid module package address %q: subdirectory part not allowed", src)
	}
	return registrySourceAddr.Package
}

// testTransport is a custom http.RoundTripper that redirects requests to the mock server
// and captures the response for inspection
type testTransport struct {
	mockURL string
	// Store the last response received from the mock server
	lastResponse *http.Response
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Create a new request to the mock server with the same path, method, body, etc.
	mockReq := &http.Request{
		Method: req.Method,
		URL: &url.URL{
			Scheme: "http",
			Host:   strings.TrimPrefix(t.mockURL, "http://"),
			Path:   req.URL.Path,
		},
		Header:     req.Header,
		Body:       req.Body,
		Host:       req.Host,
		Proto:      req.Proto,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
	}

	// Send the request to the mock server
	resp, err := http.DefaultTransport.RoundTrip(mockReq)
	if err == nil {
		t.lastResponse = resp
	}
	return resp, err
}

// TestInstallModulePackage tests the InstallModulePackage method
func TestInstallModulePackage(t *testing.T) {
	t.Parallel()

	// Create a minimal valid zip file for testing
	createTestZip := func(t *testing.T, files map[string]string) []byte {
		t.Helper()
		var buf strings.Builder
		w := zip.NewWriter(&buf)
		for name, content := range files {
			f, err := w.Create(name)
			if err != nil {
				t.Fatalf("failed to create file in zip: %v", err)
			}
			_, err = f.Write([]byte(content))
			if err != nil {
				t.Fatalf("failed to write file content: %v", err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("failed to close zip writer: %v", err)
		}
		return []byte(buf.String())
	}

	t.Run("successful download and extraction", func(t *testing.T) {
		zipContent := createTestZip(t, map[string]string{
			"main.tf": "# test module",
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipContent)
		}))
		defer server.Close()

		client := NewClient(t.Context(), nil, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr:            testParseModulePackageAddr(t, "test/module/provider"),
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		modDir, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if modDir != targetDir {
			t.Errorf("expected modDir %q, got %q", targetDir, modDir)
		}

		// Verify the file was extracted
		content, err := os.ReadFile(targetDir + "/main.tf")
		if err != nil {
			t.Fatalf("failed to read extracted file: %v", err)
		}
		if string(content) != "# test module" {
			t.Errorf("unexpected file content: %q", content)
		}
	})

	t.Run("Content-Length mismatch error", func(t *testing.T) {
		zipContent := createTestZip(t, map[string]string{
			"main.tf": "# test module",
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Claim more bytes than we actually send
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipContent)+100))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipContent)
		}))
		defer server.Close()

		client := NewClient(t.Context(), nil, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr:            testParseModulePackageAddr(t, "test/module/provider"),
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		_, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err == nil {
			t.Fatal("expected error for Content-Length mismatch")
		}
		// The error could be either a Content-Length mismatch or an EOF error
		// depending on how the HTTP client handles the truncated response
		if !strings.Contains(err.Error(), "server promised") && !strings.Contains(err.Error(), "EOF") {
			t.Errorf("expected Content-Length mismatch or EOF error, got: %v", err)
		}
	})

	t.Run("network failure handling", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Close connection immediately to simulate network failure
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server doesn't support hijacking")
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
		}))
		defer server.Close()

		client := NewClient(t.Context(), nil, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr:            testParseModulePackageAddr(t, "test/module/provider"),
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		_, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err == nil {
			t.Fatal("expected error for network failure")
		}
	})

	t.Run("subdirectory path handling", func(t *testing.T) {
		zipContent := createTestZip(t, map[string]string{
			"subdir/main.tf": "# test module in subdir",
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipContent)
		}))
		defer server.Close()

		client := NewClient(t.Context(), nil, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr:            testParseModulePackageAddr(t, "test/module/provider"),
			subdir:                 "subdir",
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		modDir, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedModDir := targetDir + "/subdir"
		if modDir != expectedModDir {
			t.Errorf("expected modDir %q, got %q", expectedModDir, modDir)
		}

		// Verify the file was extracted to the subdirectory
		content, err := os.ReadFile(modDir + "/main.tf")
		if err != nil {
			t.Fatalf("failed to read extracted file: %v", err)
		}
		if string(content) != "# test module in subdir" {
			t.Errorf("unexpected file content: %q", content)
		}
	})

	t.Run("HTTP error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		}))
		defer server.Close()

		client := NewClient(t.Context(), nil, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr:            testParseModulePackageAddr(t, "test/module/provider"),
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		_, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err == nil {
			t.Fatal("expected error for HTTP error response")
		}
		// The error should be about extraction failure since we got a non-archive response
		if !strings.Contains(err.Error(), "extracting package archive") {
			t.Errorf("expected extraction error, got: %v", err)
		}
	})

	t.Run("invalid archive format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("this is not a valid archive"))
		}))
		defer server.Close()

		client := NewClient(t.Context(), nil, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr:            testParseModulePackageAddr(t, "test/module/provider"),
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		_, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err == nil {
			t.Fatal("expected error for invalid archive format")
		}
		if !strings.Contains(err.Error(), "module package is not zip archive") {
			t.Errorf("expected archive format error, got: %v", err)
		}
	})
}

// TestInstallModulePackage_CredentialAttachment tests that credentials are
// correctly attached or omitted based on the useRegistryCredentials flag
func TestInstallModulePackage_CredentialAttachment(t *testing.T) {
	t.Parallel()

	// Create a minimal valid zip file for testing
	createTestZip := func(t *testing.T) []byte {
		t.Helper()
		var buf strings.Builder
		w := zip.NewWriter(&buf)
		f, _ := w.Create("main.tf")
		_, _ = f.Write([]byte("# test"))
		_ = w.Close()
		return []byte(buf.String())
	}

	t.Run("credentials attached when useRegistryCredentials is true", func(t *testing.T) {
		zipContent := createTestZip(t)
		var receivedAuthHeader string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipContent)
		}))
		defer server.Close()

		// Create a disco with credentials using the test package's approach
		d := test.Disco(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))

		client := NewClient(t.Context(), d, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr: regaddr.ModulePackage{
				Host:         "registry.opentofu.org",
				Namespace:    "test",
				Name:         "module",
				TargetSystem: "provider",
			},
			packageURL:             packageURL,
			useRegistryCredentials: true,
		}

		_, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedAuthHeader == "" {
			t.Error("expected Authorization header to be set when useRegistryCredentials is true")
		}
		if !strings.Contains(receivedAuthHeader, "test-auth-token") {
			t.Errorf("expected Authorization header to contain token, got: %q", receivedAuthHeader)
		}
	})

	t.Run("credentials not attached when useRegistryCredentials is false", func(t *testing.T) {
		zipContent := createTestZip(t)
		var receivedAuthHeader string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipContent)
		}))
		defer server.Close()

		// Create a disco with credentials using the test package's approach
		d := test.Disco(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))

		client := NewClient(t.Context(), d, nil)
		targetDir := t.TempDir()

		packageURL, _ := url.Parse(server.URL + "/package.zip")
		location := PackageLocationDirect{
			packageAddr: regaddr.ModulePackage{
				Host:         "registry.opentofu.org",
				Namespace:    "test",
				Name:         "module",
				TargetSystem: "provider",
			},
			packageURL:             packageURL,
			useRegistryCredentials: false,
		}

		_, err := client.InstallModulePackage(context.Background(), location, targetDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedAuthHeader != "" {
			t.Errorf("expected no Authorization header when useRegistryCredentials is false, got: %q", receivedAuthHeader)
		}
	})
}
