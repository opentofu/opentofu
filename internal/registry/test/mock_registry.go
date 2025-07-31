package test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"

	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"

	"github.com/opentofu/opentofu/internal/registry/regsrc"
	"github.com/opentofu/opentofu/internal/registry/response"
)

// Disco return a *disco.Disco mapping registry.opentofu.org, localhost,
// localhost.localdomain, and example.com to the test server.
func Disco(s *httptest.Server) *disco.Disco {
	services := map[string]interface{}{
		// Note that both with and without trailing slashes are supported behaviours
		// TODO: add specific tests to enumerate both possibilities.
		"modules.v1":   fmt.Sprintf("%s/v1/modules", s.URL),
		"providers.v1": fmt.Sprintf("%s/v1/providers", s.URL),
	}
	d := disco.New(
		disco.WithCredentials(credsSrc),
		disco.WithHTTPClient(s.Client()),
	)

	d.ForceHostServices(svchost.Hostname("registry.opentofu.org"), services)
	d.ForceHostServices(svchost.Hostname("localhost"), services)
	d.ForceHostServices(svchost.Hostname("localhost.localdomain"), services)
	d.ForceHostServices(svchost.Hostname("example.com"), services)
	return d
}

// Map of module names and location of test modules.
// Only one version for now, as we only lookup latest from the registry.
type testMod struct {
	location string
	version  string
}

// Map of provider names and location of test providers.
// Only one version for now, as we only lookup latest from the registry.
type testProvider struct {
	version string
	url     string
}

const (
	testCred = "test-auth-token"
)

var (
	regHost  = svchost.Hostname(regsrc.PublicRegistryHost.Normalized())
	credsSrc = svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{
		regHost: svcauth.HostCredentialsToken(testCred),
	})
)

// All the locations from the mockRegistry start with a file:// scheme. If
// the location string here doesn't have a scheme, the mockRegistry will
// find the absolute path and return a complete URL.
var testMods = map[string][]testMod{
	"registry/foo/bar": {{
		location: "file:///download/registry/foo/bar/0.2.3//*?archive=tar.gz",
		version:  "0.2.3",
	}},
	"registry/foo/baz": {{
		location: "file:///download/registry/foo/baz/1.10.0//*?archive=tar.gz",
		version:  "1.10.0",
	}},
	"registry/local/sub": {{
		location: "testdata/registry-tar-subdir/foo.tgz//*?archive=tar.gz",
		version:  "0.1.2",
	}},
	"exists-in-registry/identifier/provider": {{
		location: "file:///registry/exists",
		version:  "0.2.0",
	}},
	"relative/foo/bar": {{ // There is an exception for the "relative/" prefix in the test registry server
		location: "/relative-path",
		version:  "0.2.0",
	}},
	"test-versions/name/provider": {
		{version: "2.2.0"},
		{version: "2.1.1"},
		{version: "1.2.2"},
		{version: "1.2.1"},
	},
	"private/name/provider": {
		{version: "1.0.0"},
	},
}

var testProviders = map[string][]testProvider{
	"-/foo": {
		{
			version: "0.2.3",
			url:     "https://releases.hashicorp.com/terraform-provider-foo/0.2.3/terraform-provider-foo.zip",
		},
		{version: "0.3.0"},
	},
	"-/bar": {
		{
			version: "0.1.1",
			url:     "https://releases.hashicorp.com/terraform-provider-bar/0.1.1/terraform-provider-bar.zip",
		},
		{version: "0.1.2"},
	},
}

func providerAlias(provider string) string {
	re := regexp.MustCompile("^-/")
	if re.MatchString(provider) {
		return re.ReplaceAllString(provider, "terraform-providers/")
	}
	return provider
}

func init() {
	// Add provider aliases
	for provider, info := range testProviders {
		alias := providerAlias(provider)
		testProviders[alias] = info
	}
}

func mockRegHandler(config map[uint8]struct{}) http.Handler {
	mux := http.NewServeMux()

	moduleDownload := func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimLeft(r.URL.Path, "/")
		// handle download request
		re := regexp.MustCompile(`^([-a-z]+/\w+/\w+).*/download$`)
		// download lookup
		matches := re.FindStringSubmatch(p)
		if len(matches) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check for auth
		if strings.Contains(matches[0], "private/") {
			if !strings.Contains(r.Header.Get("Authorization"), testCred) {
				http.Error(w, "", http.StatusForbidden)
				return
			}
		}

		versions, ok := testMods[matches[1]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		mod := versions[0]

		location := mod.location
		if !strings.HasPrefix(matches[0], "relative/") && !strings.HasPrefix(location, "file:///") {
			// we can't use filepath.Abs because it will clean `//`
			wd, _ := os.Getwd()
			location = fmt.Sprintf("file://%s/%s", wd, location)
		}

		// the location will be returned in the response header
		_, inHeader := config[WithModuleLocationInHeader]
		// the location will be returned in the response body
		_, inBody := config[WithModuleLocationInBody]

		if inHeader {
			w.Header().Set("X-Terraform-Get", location)
		}

		if inBody {
			w.WriteHeader(http.StatusOK)
			o, err := json.Marshal(response.ModuleLocationRegistryResp{Location: location})
			if err != nil {
				panic("mock error: " + err.Error())
			}
			_, _ = w.Write(o)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}

	moduleVersions := func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimLeft(r.URL.Path, "/")
		re := regexp.MustCompile(`^([-a-z]+/\w+/\w+)/versions$`)
		matches := re.FindStringSubmatch(p)
		if len(matches) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check for auth
		if strings.Contains(matches[1], "private/") {
			if !strings.Contains(r.Header.Get("Authorization"), testCred) {
				http.Error(w, "", http.StatusForbidden)
			}
		}

		name := matches[1]
		versions, ok := testMods[name]
		if !ok {
			http.NotFound(w, r)
			return
		}

		// only adding the single requested module for now
		// this is the minimal that any registry is expected to support
		mpvs := &response.ModuleProviderVersions{
			Source: name,
		}

		for _, v := range versions {
			mv := &response.ModuleVersion{
				Version: v.version,
			}
			mpvs.Versions = append(mpvs.Versions, mv)
		}

		resp := response.ModuleVersions{
			Modules: []*response.ModuleProviderVersions{mpvs},
		}

		js, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	mux.Handle("/v1/modules/",
		http.StripPrefix("/v1/modules/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/download") {
				moduleDownload(w, r)
				return
			}

			if strings.HasSuffix(r.URL.Path, "/versions") {
				moduleVersions(w, r)
				return
			}

			http.NotFound(w, r)
		})),
	)

	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"modules.v1":"http://localhost/v1/modules/", "providers.v1":"http://localhost/v1/providers/"}`)
		if err != nil {
			w.WriteHeader(500)
		}
	})
	return mux
}

const (
	// WithModuleLocationInBody sets to return the module's location in the response body
	WithModuleLocationInBody uint8 = iota
	// WithModuleLocationInHeader sets to return the module's location in the response header
	WithModuleLocationInHeader
)

// Registry returns an httptest server that mocks out some registry functionality.
func Registry(flags ...uint8) *httptest.Server {
	if len(flags) == 0 {
		return httptest.NewServer(mockRegHandler(
			map[uint8]struct{}{
				// default setting
				WithModuleLocationInBody: {},
			},
		))
	}

	cfg := map[uint8]struct{}{}
	for _, flag := range flags {
		cfg[flag] = struct{}{}
	}
	return httptest.NewServer(mockRegHandler(cfg))
}

// RegistryRetryableErrorsServer returns an httptest server that mocks out the
// registry API to return 502 errors.
func RegistryRetryableErrorsServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/modules/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mocked server error", http.StatusBadGateway)
	})
	mux.HandleFunc("/v1/providers/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mocked server error", http.StatusBadGateway)
	})
	return httptest.NewServer(mux)
}
