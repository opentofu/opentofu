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
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	regaddr "github.com/opentofu/registry-address/v2"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
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

// ModulePackageLocation finds the package location for a specific module package version.
//
// The subdir parameter is the subdirectory path from the original module source
// address (e.g., from addrs.ModuleSourceRegistry.Subdir). This is passed separately
// because regaddr.ModulePackage represents only the package itself, not the full
// module source address which may include a subdirectory selector. The subdir is
// needed to properly construct the PackageLocation result.
//
// This returns one of the concrete implementations of the closed interface
// [PackageLocation], depending on what type of location the registry chooses
// to report. Refer to the documentation of those types for information on
// how each variant should be used to actually install the package.
func (c *Client) ModulePackageLocation(ctx context.Context, packageAddr regaddr.ModulePackage, version string, subdir string) (PackageLocation, error) {
	ctx, span := tracing.Tracer().Start(ctx, "Find Module Location", tracing.SpanAttributes(
		traceattrs.OpenTofuModuleSource(packageAddr.String()),
		traceattrs.OpenTofuModuleVersion(version),
	))
	defer span.End()

	host := packageAddr.Host
	baseURL, err := c.discoverBaseURL(ctx, host)
	if err != nil {
		return nil, err
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body from registry: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var v response.ModuleLocationRegistryResp
		if err := json.Unmarshal(body, &v); err != nil {
			return nil, fmt.Errorf("module %q version %q failed to deserialize response body %s: %w",
				packageAddr, version, body, err)
		}

		if v.UseRegistryCredentials == nil {
			// The registry has not opted in to "direct" installation, so we
			// assume that it wants the old-style "indirect" behavior where
			// the registry is essentially just a lookup table for
			// go-getter-style source addresses, in which case the registry
			// isn't involved in the final download step at all.

			if v.Location == "" {
				// If the location is empty, we will fallback to the header.
				// Note that this only works if the body contains valid JSON syntax.
				// This was probably not actually the originally intended behavior,
				// since this fallback was introduced to fix a regression in
				// https://github.com/opentofu/opentofu/pull/2079 but that didn't
				// _quite_ restore the original behavior of ignoring the body completely
				// when using this header. Nonetheless, we're keeping this constraint
				// to avoid churning this protocol further since registry
				// implementers tend to want to support many OpenTofu versions
				// at once and so having many different variations is harder
				// to test. Those who want to do the legacy thing of using
				// X-Terraform-Get should use a "204 No Content" status code if
				// they can't provide valid JSON syntax in the body.
				return preparePackageLocationIndirect(resp.Header.Get(xTerraformGet), subdir, metadataURL)
			}
			return preparePackageLocationIndirect(v.Location, subdir, metadataURL)
		}

		// Otherwise, the registry has opted in to the new-style "direct"
		// installation approach, where the registry returns a URL that's under
		// its own control and we fetch from it directly instead of delegating
		// to go-getter.
		return preparePackageLocationDirect(v.Location, packageAddr, subdir, metadataURL, bool(*v.UseRegistryCredentials))

	case http.StatusNoContent:
		// FALLBACK: set the found location from the header
		return preparePackageLocationIndirect(resp.Header.Get(xTerraformGet), subdir, metadataURL)

	case http.StatusNotFound:
		return nil, fmt.Errorf("module %q version %q not found", packageAddr, version)

	default:
		// anything else is an error:
		return nil, fmt.Errorf("error getting download location for %q: %s resp:%s", packageAddr, resp.Status, body)
	}
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

// InstallModulePackage attempts to install a module package from the given
// location into the given target directory.
//
// This method is used only for "direct" package locations, where the registry
// is directly hosting packages in locations under its own control, and possibly
// authenticated using the registry's own credentials. If you have a
// [PackageLocationIndirect] instead then you must handle it separately using
// the "remote source address" installation process.
//
// If successful this returns the final path of the requested module, taking
// into account any subdirectory selection that was included in the original
// module request. If the original source address did not include a subdirectory
// portion then the result is just a normalized version of targetDir.
func (c *Client) InstallModulePackage(ctx context.Context, location PackageLocationDirect, targetDir string) (string, error) {
	urlString := location.packageURL.String()
	ctx, span := tracing.Tracer().Start(ctx, "Fetch Package",
		tracing.SpanAttributes(traceattrs.URLFull(urlString)),
	)
	defer span.End()

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", urlString, nil)
	if err != nil {
		return "", fmt.Errorf("preparing to download from %s: %w", urlString, err)
	}

	host := location.packageAddr.Host
	if location.useRegistryCredentials {
		c.addRequestCreds(ctx, host, req.Request)
	}
	req.Header.Set(xTerraformVersion, tfVersion)
	// We'll set some content negotiation headers just in case that helps
	// someone using a general-purpose static HTTP server serve content
	// compressed on the fly by the server. (We will actually tolerate more
	// than what we report here, in the more common case where the server
	// just returns whatever it has on disk without any transformation,
	// but this is just a hint for some common choices.
	req.Header.Set("Accept", "application/zip, application/x-tar; *;q=0.1")
	req.Header.Set("Accept-Encoding", "identity, gzip, *;q=0.1")

	// We first fetch the raw content at the URL into a temporary file, and
	// then we can sniff what format it seems to be in so that we'll tolerate
	// servers that aren't able to correctly populate Content-Type and other
	// similar header fields (which is relatively common for static file
	// servers used for serving large files like these; e.g. they sometimes
	// report just "application/octet-stream", or misreport which compressor
	// was used for a tar stream, etc.).
	f, err := os.CreateTemp("", "opentofu-modpkg-")
	if err != nil {
		return "", fmt.Errorf("creating temporary file for module package: %w", err)
	}
	defer func() {
		// We make a best-effort to proactively clean the temporary file, but
		// if this fails we'll still let installation succeed and assume that
		// an OS service will clean the temporary directory itself eventually.
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err // net/http includes method and URL in its errors automatically
	}
	defer resp.Body.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return "", fmt.Errorf("copying module package to temporary file: %w", err)
	}
	if wantN, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); err == nil {
		// If the server told us how much data it was expecting to send then
		// we'll make sure we got exactly that much data.
		if n != wantN {
			return "", fmt.Errorf("server promised %d bytes, but returned %d bytes", wantN, n)
		}
	}

	err = extractModulePackage(f, targetDir)
	if err != nil {
		return "", fmt.Errorf("extracting package archive: %w", err)
	}
	modDir := targetDir
	if location.subdir != "" {
		subDir := filepath.FromSlash(location.subdir)
		modDir = filepath.Join(modDir, subDir)
	}
	return modDir, nil
}

// preparePackageLocationIndirect constructs a PackageLocationIndirect from a
// raw go-getter-style address string returned by the registry.
func preparePackageLocationIndirect(realAddrRaw string, subdir string, baseURL *url.URL) (PackageLocation, error) {
	if realAddrRaw == "" {
		return nil, fmt.Errorf("registry did not return a location for this package")
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
	if strings.HasPrefix(realAddrRaw, "/") || strings.HasPrefix(realAddrRaw, "./") || strings.HasPrefix(realAddrRaw, "../") {
		locationURL, err := url.Parse(realAddrRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid relative URL %q: %w", realAddrRaw, err)
		}
		locationURL = baseURL.ResolveReference(locationURL)
		realAddrRaw = locationURL.String()
	}

	realAddrAny, err := addrs.ParseModuleSource(realAddrRaw)
	if err != nil {
		return nil, fmt.Errorf(
			"registry returned invalid package location %q: %w",
			realAddrRaw, err,
		)
	}
	realAddr, ok := realAddrAny.(addrs.ModuleSourceRemote)
	if !ok {
		return nil, fmt.Errorf("registry returned invalid package location %q: must be a direct remote package address", realAddrRaw)
	}

	// When we're installing indirectly it's possible that both the registry
	// source address and the go-getter-style address returned from the registry
	// include a "subdirectory" component, in which case we need to resolve
	// the final effective subdirectory path that combines both.
	if subdir != "" {
		switch {
		case realAddr.Subdir != "":
			realAddr.Subdir = path.Join(realAddr.Subdir, subdir)
		default:
			realAddr.Subdir = subdir
		}
	}

	return PackageLocationIndirect{
		SourceAddr: realAddr,
	}, nil
}

// preparePackageLocationDirect constructs a PackageLocationDirect from a
// URL string returned by the registry for direct package hosting.
func preparePackageLocationDirect(locationRaw string, packageAddr regaddr.ModulePackage, subdir string, baseURL *url.URL, useRegistryCredentials bool) (PackageLocation, error) {
	packageURL, err := url.Parse(locationRaw)
	if err != nil {
		return nil, fmt.Errorf("registry returned an invalid package URL: %w", err)
	}

	if !packageURL.IsAbs() {
		// We resolve relative URLs against the URL we got the location from.
		packageURL = baseURL.ResolveReference(packageURL)
	}
	if packageURL.Scheme != "http" && packageURL.Scheme != "https" {
		return nil, fmt.Errorf("registry returned invalid package URL %q: must be http or https URL", locationRaw)
	}
	if packageURL.Fragment != "" {
		return nil, fmt.Errorf("registry returned invalid package URL %q: must not include fragment part", locationRaw)
	}

	return PackageLocationDirect{
		packageAddr:            packageAddr,
		subdir:                 subdir,
		packageURL:             packageURL,
		useRegistryCredentials: useRegistryCredentials,
	}, nil
}
