// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/svchost"
	disco "github.com/opentofu/svchost/disco"
)

// testRegistryServices starts up a local HTTP server running a fake provider registry
// service and returns a service discovery object pre-configured to consider
// the host "example.com" to be served by the fake registry service.
//
// The returned discovery object also knows the hostname "not.example.com"
// which does not have a provider registry at all and "too-new.example.com"
// which has a "providers.v99" service that is inoperable but could be useful
// to test the error reporting for detecting an unsupported protocol version.
// It also knows fails.example.com but it refers to an endpoint that doesn't
// correctly speak HTTP, to simulate a protocol error.
//
// The second return value is a function to call at the end of a test function
// to shut down the test server. After you call that function, the discovery
// object becomes useless.
func testRegistryServices(t *testing.T) (services *disco.Disco, baseURL string, cleanup func()) {
	server := httptest.NewServer(http.HandlerFunc(fakeRegistryHandler))

	services = disco.New()
	services.ForceHostServices(svchost.Hostname("example.com"), map[string]interface{}{
		"providers.v1": server.URL + "/providers/v1/",
	})
	services.ForceHostServices(svchost.Hostname("not.example.com"), map[string]interface{}{})
	services.ForceHostServices(svchost.Hostname("too-new.example.com"), map[string]interface{}{
		// This service doesn't actually work; it's here only to be
		// detected as "too new" by the discovery logic.
		"providers.v99": server.URL + "/providers/v99/",
	})
	services.ForceHostServices(svchost.Hostname("fails.example.com"), map[string]interface{}{
		"providers.v1": server.URL + "/fails-immediately/",
	})

	// We'll also permit registry.opentofu.org here just because it's our
	// default and has some unique features that are not allowed on any other
	// hostname. It behaves the same as example.com, which should be preferred
	// if you're not testing something specific to the default registry in order
	// to ensure that most things are hostname-agnostic.
	services.ForceHostServices(svchost.Hostname("registry.opentofu.org"), map[string]interface{}{
		"providers.v1": server.URL + "/providers/v1/",
	})

	return services, server.URL, func() {
		server.Close()
	}
}

// testRegistrySource is a wrapper around testServices that uses the created
// discovery object to produce a Source instance that is ready to use with the
// fake registry services.
//
// As with testServices, the second return value is a function to call at the end
// of your test in order to shut down the test server.
func testRegistrySource(t *testing.T) (source *RegistrySource, baseURL string, cleanup func()) {
	services, baseURL, close := testRegistryServices(t)
	source = NewRegistrySource(t.Context(), services, nil, LocationConfig{ProviderDownloadRetries: 0})
	return source, baseURL, close
}

func testRegistrySourceWithLocationConfig(t *testing.T, config LocationConfig) (source *RegistrySource, baseURL string, cleanup func()) {
	services, baseURL, close := testRegistryServices(t)
	source = NewRegistrySource(t.Context(), services, nil, config)
	return source, baseURL, close
}

func fakeRegistryHandler(resp http.ResponseWriter, req *http.Request) {
	// Helper that assumes http writes will always succeed
	write := func(data []byte) {
		if _, err := resp.Write(data); err != nil {
			panic(err)
		}
	}

	path := req.URL.EscapedPath()
	if strings.HasPrefix(path, "/fails-immediately/") {
		// Here we take over the socket and just close it immediately, to
		// simulate one possible way a server might not be an HTTP server.
		hijacker, ok := resp.(http.Hijacker)
		if !ok {
			// Not hijackable, so we'll just fail normally.
			// If this happens, tests relying on this will fail.
			resp.WriteHeader(500)
			write([]byte(`cannot hijack`))
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			resp.WriteHeader(500)
			write([]byte(`hijack failed`))
			return
		}
		conn.Close()
		return
	}

	if strings.HasPrefix(path, "/pkg/") {
		switch path {
		case "/pkg/awesomesauce/happycloud_1.2.0.zip":
			write([]byte("some zip file"))
		case "/pkg/awesomesauce/happycloud_1.2.0_SHA256SUMS":
			write([]byte("000000000000000000000000000000000000000000000000000000000000f00d happycloud_1.2.0.zip\n000000000000000000000000000000000000000000000000000000000000face happycloud_1.2.0_face.zip\n"))
		case "/pkg/awesomesauce/happycloud_1.2.0_SHA256SUMS.sig":
			write([]byte("GPG signature"))
		case "/pkg/missing/providerbinary_1.2.0.zip":
			// Just return a retryable status code
			resp.WriteHeader(http.StatusInternalServerError)
		case "/pkg/missing/providerbinary_1.2.0_SHA256SUMS":
			write([]byte("000000000000000000000000000000000000000000000000000000000000f00d providerbinary_1.2.0.zip\n000000000000000000000000000000000000000000000000000000000000face providerbinary_1.2.0_face.zip\n"))
		case "/pkg/missing/providerbinary_1.2.0_SHA256SUMS.sig":
			write([]byte("GPG signature"))
		default:
			resp.WriteHeader(404)
			write([]byte("unknown package file download"))
		}
		return
	}

	if !strings.HasPrefix(path, "/providers/v1/") {
		resp.WriteHeader(404)
		write([]byte(`not a provider registry endpoint`))
		return
	}

	pathParts := strings.Split(path, "/")[3:]
	if len(pathParts) < 3 {
		resp.WriteHeader(404)
		write([]byte(`unexpected number of path parts`))
		return
	}
	log.Printf("[TRACE] fake provider registry request for %#v", pathParts)

	if pathParts[2] == "versions" {
		if len(pathParts) != 3 {
			resp.WriteHeader(404)
			write([]byte(`extraneous path parts`))
			return
		}

		switch pathParts[0] + "/" + pathParts[1] {
		case "awesomesauce/happycloud":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			// Note that these version numbers are intentionally misordered
			// so we can test that the client-side code places them in the
			// correct order (lowest precedence first).
			write([]byte(`{"versions":[{"version":"0.1.0","protocols":["1.0"]},{"version":"2.0.0","protocols":["99.0"]},{"version":"1.2.0","protocols":["5.0"]}, {"version":"1.0.0","protocols":["5.0"]}]}`))
		case "weaksauce/unsupported-protocol":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			write([]byte(`{"versions":[{"version":"1.0.0","protocols":["0.1"]}]}`))
		case "weaksauce/protocol-six":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			write([]byte(`{"versions":[{"version":"1.0.0","protocols":["6.0"]}]}`))
		case "weaksauce/no-versions":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			write([]byte(`{"versions":[],"warnings":["this provider is weaksauce"]}`))
		case "-/legacy":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			// This response is used for testing LookupLegacyProvider
			write([]byte(`{"id":"legacycorp/legacy"}`))
		case "-/moved":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			// This response is used for testing LookupLegacyProvider
			write([]byte(`{"id":"hashicorp/moved","moved_to":"acme/moved"}`))
		case "-/changetype":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			// This (unrealistic) response is used for error handling code coverage
			write([]byte(`{"id":"legacycorp/newtype"}`))
		case "-/invalid":
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			// This (unrealistic) response is used for error handling code coverage
			write([]byte(`{"id":"some/invalid/id/string"}`))
		default:
			resp.WriteHeader(404)
			write([]byte(`unknown namespace or provider type`))
		}
		return
	}

	if len(pathParts) == 6 && pathParts[3] == "download" {
		switch pathParts[0] + "/" + pathParts[1] {
		case "awesomesauce/happycloud", "missing/providerbinary":
			pNamespace := pathParts[0]
			pType := pathParts[1]
			if pathParts[4] == "nonexist" {
				resp.WriteHeader(404)
				write([]byte(`unsupported OS`))
				return
			}
			var protocols []string
			version := pathParts[2]
			switch version {
			case "0.1.0":
				protocols = []string{"1.0"}
			case "2.0.0":
				protocols = []string{"99.0"}
			default:
				protocols = []string{"5.0"}
			}

			body := map[string]interface{}{
				"protocols":             protocols,
				"os":                    pathParts[4],
				"arch":                  pathParts[5],
				"filename":              fmt.Sprintf("%s_%s.zip", pType, version),
				"shasum":                "000000000000000000000000000000000000000000000000000000000000f00d",
				"download_url":          fmt.Sprintf("/pkg/%s/%s_%s.zip", pNamespace, pType, version),
				"shasums_url":           fmt.Sprintf("/pkg/%s/%s_%s_SHA256SUMS", pNamespace, pType, version),
				"shasums_signature_url": fmt.Sprintf("/pkg/%s/%s_%s_SHA256SUMS.sig", pNamespace, pType, version),
				"signing_keys": map[string]interface{}{
					"gpg_public_keys": []map[string]interface{}{
						{
							"ascii_armor": TestingPublicKey,
						},
					},
				},
			}
			enc, err := json.Marshal(body)
			if err != nil {
				resp.WriteHeader(500)
				write([]byte("failed to encode body"))
			}
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(200)
			write(enc)
		default:
			resp.WriteHeader(404)
			write([]byte(`unknown namespace/provider/version/architecture`))
		}
		return
	}

	resp.WriteHeader(404)
	write([]byte(`unrecognized path scheme`))
}

func TestProviderVersions(t *testing.T) {
	source, _, close := testRegistrySource(t)
	defer close()

	tests := []struct {
		provider     addrs.Provider
		wantVersions map[string][]string
		wantErr      string
	}{
		{
			addrs.MustParseProviderSourceString("example.com/awesomesauce/happycloud"),
			map[string][]string{
				"0.1.0": {"1.0"},
				"1.0.0": {"5.0"},
				"1.2.0": {"5.0"},
				"2.0.0": {"99.0"},
			},
			``,
		},
		{
			addrs.MustParseProviderSourceString("example.com/weaksauce/no-versions"),
			nil,
			``,
		},
		{
			addrs.MustParseProviderSourceString("example.com/nonexist/nonexist"),
			nil,
			`provider registry example.com does not have a provider named example.com/nonexist/nonexist`,
		},
	}
	for _, test := range tests {
		t.Run(test.provider.String(), func(t *testing.T) {
			client, err := source.registryClient(t.Context(), test.provider.Hostname)
			if err != nil {
				t.Fatal(err)
			}

			gotVersions, _, err := client.ProviderVersions(t.Context(), test.provider)

			if err != nil {
				if test.wantErr == "" {
					t.Fatalf("wrong error\ngot:  %s\nwant: <nil>", err.Error())
				}
				if got, want := err.Error(), test.wantErr; got != want {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", got, want)
				}
				return
			}

			if test.wantErr != "" {
				t.Fatalf("wrong error\ngot:  <nil>\nwant: %s", test.wantErr)
			}

			if diff := cmp.Diff(test.wantVersions, gotVersions); diff != "" {
				t.Errorf("wrong result\n%s", diff)
			}
		})
	}
}

func TestFindClosestProtocolCompatibleVersion(t *testing.T) {
	source, _, close := testRegistrySource(t)
	defer close()

	tests := map[string]struct {
		provider       addrs.Provider
		version        Version
		wantSuggestion Version
		wantErr        string
	}{
		"pinned version too old": {
			addrs.MustParseProviderSourceString("example.com/awesomesauce/happycloud"),
			MustParseVersion("0.1.0"),
			MustParseVersion("1.2.0"),
			``,
		},
		"pinned version too new": {
			addrs.MustParseProviderSourceString("example.com/awesomesauce/happycloud"),
			MustParseVersion("2.0.0"),
			MustParseVersion("1.2.0"),
			``,
		},
		// This should not actually happen, the function is only meant to be
		// called when the requested provider version is not supported
		"pinned version just right": {
			addrs.MustParseProviderSourceString("example.com/awesomesauce/happycloud"),
			MustParseVersion("1.2.0"),
			MustParseVersion("1.2.0"),
			``,
		},
		"nonexisting provider": {
			addrs.MustParseProviderSourceString("example.com/nonexist/nonexist"),
			MustParseVersion("1.2.0"),
			versions.Unspecified,
			`provider registry example.com does not have a provider named example.com/nonexist/nonexist`,
		},
		"versionless provider": {
			addrs.MustParseProviderSourceString("example.com/weaksauce/no-versions"),
			MustParseVersion("1.2.0"),
			versions.Unspecified,
			``,
		},
		"unsupported provider protocol": {
			addrs.MustParseProviderSourceString("example.com/weaksauce/unsupported-protocol"),
			MustParseVersion("1.0.0"),
			versions.Unspecified,
			``,
		},
		"provider protocol six": {
			addrs.MustParseProviderSourceString("example.com/weaksauce/protocol-six"),
			MustParseVersion("1.0.0"),
			MustParseVersion("1.0.0"),
			``,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client, err := source.registryClient(t.Context(), test.provider.Hostname)
			if err != nil {
				t.Fatal(err)
			}

			got, err := client.findClosestProtocolCompatibleVersion(t.Context(), test.provider, test.version)

			if err != nil {
				if test.wantErr == "" {
					t.Fatalf("wrong error\ngot:  %s\nwant: <nil>", err.Error())
				}
				if got, want := err.Error(), test.wantErr; got != want {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", got, want)
				}
				return
			}

			if test.wantErr != "" {
				t.Fatalf("wrong error\ngot:  <nil>\nwant: %s", test.wantErr)
			}

			fmt.Printf("Got: %s, Want: %s\n", got, test.wantSuggestion)

			if !got.Same(test.wantSuggestion) {
				t.Fatalf("wrong result\ngot:  %s\nwant: %s", got.String(), test.wantSuggestion.String())
			}
		})
	}
}

// Checks that the [LocationConfig] is used properly to configure the [PackageHTTPURL] http client,
// meaning that the retries are configured as expected.
func TestLocationRetriesConfiguredCorrectly(t *testing.T) {
	source, _, close := testRegistrySourceWithLocationConfig(t, LocationConfig{ProviderDownloadRetries: 2})
	defer close()

	parts := strings.Split("example.com/missing/providerbinary", "/")
	providerAddr := addrs.Provider{
		Hostname:  svchost.Hostname(parts[0]),
		Namespace: parts[1],
		Type:      parts[2],
	}

	version := versions.MustParseVersion("1.2.0")

	got, err := source.PackageMeta(t.Context(), providerAddr, version, Platform{"linux", "amd64"})
	if err != nil {
		t.Fatalf("unexpected error got from packageMeta: %s", err)
	}
	tmp := t.TempDir()
	_, err = got.Location.InstallProviderPackage(t.Context(), got, tmp, nil)
	if err == nil {
		t.Fatalf("expected error but got nothing")
	}
	if expectedSuffix := "giving up after 3 attempt(s)"; !strings.HasSuffix(err.Error(), expectedSuffix) {
		t.Fatalf("expected err %q to have suffix %q", err.Error(), expectedSuffix)
	}
}
