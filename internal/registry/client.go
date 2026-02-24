// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	regaddr "github.com/opentofu/registry-address/v2"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/registry/response"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
	"github.com/opentofu/opentofu/version"
)

const (
	xTerraformGet     = "X-Terraform-Get"
	xTerraformVersion = "X-Terraform-Version"
	modulesServiceID  = "modules.v1"
)

var (
	tfVersion = version.String()
)

// Client provides methods to query OpenTofu module registries.
//
// This client implements the "modules.v1" protocol. It does not implement
// any other OpenTofu registry protocols, and in particular the client for
// provider registry clients lives elsewhere.
//
// (The overly-general name of this package is a historical accident, and
// perhaps one day this package should move to "getmodules/registry" instead of
// just "registry" to make the scope a little clearer.)
type Client struct {
	// this is the client to be used for all requests.
	client *retryablehttp.Client

	// services is a required *disco.Disco, which may have services and
	// credentials pre-loaded.
	services *disco.Disco
}

// NewClient returns a new initialized registry client.
func NewClient(ctx context.Context, services *disco.Disco, client *retryablehttp.Client) *Client {
	if services == nil {
		services = disco.New()
	}

	if client == nil {
		// The following is a fallback client configuration intended primarily
		// for our test cases that directly call this function.
		client = httpclient.NewForRegistryRequests(ctx, 1, 10*time.Second)
	}

	return &Client{
		client:   client,
		services: services,
	}
}

// discoverBaseURL performs service discovery to find the base URL for the
// module registry implementation on the given host.
func (c *Client) discoverBaseURL(ctx context.Context, host svchost.Hostname) (*url.URL, error) {
	service, err := c.services.DiscoverServiceURL(ctx, host, modulesServiceID)
	if err != nil {
		return nil, &ServiceUnreachableError{err}
	}
	if !strings.HasSuffix(service.Path, "/") {
		service.Path += "/"
	}
	return service, nil
}

// ModulePackageVersions queries the registry for a module package, and returns the available versions.
func (c *Client) ModulePackageVersions(ctx context.Context, packageAddr regaddr.ModulePackage) (*response.ModuleVersions, error) {
	ctx, span := tracing.Tracer().Start(ctx, "List Versions", tracing.SpanAttributes(
		traceattrs.OpenTofuModuleSource(packageAddr.String()),
	))
	defer span.End()

	host := packageAddr.Host
	baseURL, err := c.discoverBaseURL(ctx, host)
	if err != nil {
		return nil, err
	}
	versionsURL := modulePackageEndpointURL(baseURL, packageAddr, "versions")

	log.Printf("[DEBUG] fetching module versions from %q", versionsURL)

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", versionsURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	c.addRequestCreds(ctx, host, req.Request)
	req.Header.Set(xTerraformVersion, tfVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// OK
	case http.StatusNotFound:
		return nil, &errModuleNotFound{packageAddr: packageAddr}
	default:
		return nil, fmt.Errorf("error looking up module versions: %s", resp.Status)
	}

	var versions response.ModuleVersions

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&versions); err != nil {
		return nil, err
	}

	for _, mod := range versions.Modules {
		for _, v := range mod.Versions {
			log.Printf("[DEBUG] found available version %q for %s", v.Version, packageAddr)
		}
	}

	return &versions, nil
}

// ModulePackageLocation find the download location for a specific module package version.
//
// This returns a string, because the final location may contain special go-getter syntax.
func (c *Client) ModulePackageLocation(ctx context.Context, packageAddr regaddr.ModulePackage, version string) (string, error) {
	ctx, span := tracing.Tracer().Start(ctx, "Find Module Location", tracing.SpanAttributes(
		traceattrs.OpenTofuModuleSource(packageAddr.String()),
		traceattrs.OpenTofuModuleVersion(version),
	))
	defer span.End()

	host := packageAddr.Host
	baseURL, err := c.discoverBaseURL(ctx, host)
	if err != nil {
		return "", err
	}

	// Historical note: an older version of this client code accepted "version"
	// being empty and constructed a different form of URL where the version
	// component was completely omitted, but the documentation for the registry
	// protocol doesn't define the meaning of a URL scheme like that and in
	// practice the callers of the client always populate the version, so we
	// don't support omitting that anymore: a version string is now always expected.
	metadataURL := modulePackageEndpointURL(baseURL, packageAddr, version, "download")
	log.Printf("[DEBUG] looking up module location from %q", metadataURL)

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", metadataURL.String(), nil)
	if err != nil {
		return "", err
	}

	req = req.WithContext(ctx)

	c.addRequestCreds(ctx, host, req.Request)
	req.Header.Set(xTerraformVersion, tfVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body from registry: %w", err)
	}

	var location string

	switch resp.StatusCode {
	case http.StatusOK:
		var v response.ModuleLocationRegistryResp
		if err := json.Unmarshal(body, &v); err != nil {
			return "", fmt.Errorf("module %q version %q failed to deserialize response body %s: %w",
				packageAddr, version, body, err)
		}

		location = v.Location

		// if the location is empty, we will fallback to the header
		if location == "" {
			location = resp.Header.Get(xTerraformGet)
		}

	case http.StatusNoContent:
		// FALLBACK: set the found location from the header
		location = resp.Header.Get(xTerraformGet)

	case http.StatusNotFound:
		return "", fmt.Errorf("module %q version %q not found", packageAddr, version)

	default:
		// anything else is an error:
		return "", fmt.Errorf("error getting download location for %q: %s resp:%s", packageAddr, resp.Status, body)
	}

	if location == "" {
		return "", fmt.Errorf("failed to get download URL for %q: %s resp:%s", packageAddr, resp.Status, body)
	}

	// If location looks like it's trying to be a relative URL, treat it as
	// one.
	//
	// We don't do this for just _any_ location, since the X-Terraform-Get
	// header is a go-getter location rather than a URL, and so not all
	// possible values will parse reasonably as URLs.)
	//
	// When used in conjunction with go-getter we normally require this header
	// to be an absolute URL, but we are more liberal here because third-party
	// registry implementations may not "know" their own absolute URLs if
	// e.g. they are running behind a reverse proxy frontend, or such.
	if strings.HasPrefix(location, "/") || strings.HasPrefix(location, "./") || strings.HasPrefix(location, "../") {
		locationURL, err := url.Parse(location)
		if err != nil {
			return "", fmt.Errorf("invalid relative URL for %q: %w", packageAddr, err)
		}
		locationURL = metadataURL.ResolveReference(locationURL)
		location = locationURL.String()
	}

	return location, nil
}

func (c *Client) addRequestCreds(ctx context.Context, host svchost.Hostname, req *http.Request) {
	creds, err := c.services.CredentialsForHost(ctx, host)
	if err != nil {
		log.Printf("[WARN] Failed to get credentials for %s: %s (ignoring)", host, err)
		return
	}

	if creds != nil {
		creds.PrepareRequest(req)
	}
}

func modulePackageEndpointURL(baseURL *url.URL, packageAddr regaddr.ModulePackage, subComponents ...string) *url.URL {
	parts := make([]string, 3, 3+len(subComponents))
	parts[0] = packageAddr.Namespace
	parts[1] = packageAddr.Name
	parts[2] = packageAddr.TargetSystem
	parts = append(parts, subComponents...)
	relPath := path.Join(parts...)
	relURL, err := url.Parse(relPath)
	if err != nil {
		// We control all of the inputs here, so if there's an error then it's
		// a bug in whatever created the values given as arguments.
		panic(fmt.Sprintf("constructed invalid relative URL %q for module package endpoint", relPath))
	}
	return baseURL.ResolveReference(relURL)
}
