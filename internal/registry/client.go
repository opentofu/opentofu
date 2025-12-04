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
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/registry/regsrc"
	"github.com/opentofu/opentofu/internal/registry/response"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
	"github.com/opentofu/opentofu/version"
)

const (
	xTerraformGet      = "X-Terraform-Get"
	xTerraformVersion  = "X-Terraform-Version"
	modulesServiceID   = "modules.v1"
	providersServiceID = "providers.v1"
)

var (
	tfVersion = version.String()
)

// Client provides methods to query OpenTofu Registries.
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

// Discover queries the host, and returns the url for the registry.
func (c *Client) Discover(ctx context.Context, host svchost.Hostname, serviceID string) (*url.URL, error) {
	service, err := c.services.DiscoverServiceURL(ctx, host, serviceID)
	if err != nil {
		return nil, &ServiceUnreachableError{err}
	}
	if !strings.HasSuffix(service.Path, "/") {
		service.Path += "/"
	}
	return service, nil
}

// ModuleVersions queries the registry for a module, and returns the available versions.
func (c *Client) ModuleVersions(ctx context.Context, module *regsrc.Module) (*response.ModuleVersions, error) {
	ctx, span := tracing.Tracer().Start(ctx, "List Versions", tracing.SpanAttributes(
		traceattrs.OpenTofuModuleCallName(module.RawName),
	))
	defer span.End()

	host, err := module.SvcHost()
	if err != nil {
		return nil, err
	}

	service, err := c.Discover(ctx, host, modulesServiceID)
	if err != nil {
		return nil, err
	}

	p, err := url.Parse(path.Join(module.Module(), "versions"))
	if err != nil {
		return nil, err
	}

	service = service.ResolveReference(p)

	log.Printf("[DEBUG] fetching module versions from %q", service)

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", service.String(), nil)
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
		return nil, &errModuleNotFound{addr: module}
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
			log.Printf("[DEBUG] found available version %q for %s", v.Version, module.Module())
		}
	}

	return &versions, nil
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

// ModuleLocation find the package location for a specific module version.
//
// This returns one of the concrete implementations of the closed interface
// [PackageLocation], depending on what type of location the registry chooses
// to report. Refer to the documentation of those types for information on
// how each variant should be used to actually install the package.
func (c *Client) ModuleLocation(ctx context.Context, module *regsrc.Module, version string) (PackageLocation, error) {
	ctx, span := tracing.Tracer().Start(ctx, "Find Module Location", tracing.SpanAttributes(
		traceattrs.OpenTofuModuleCallName(module.RawName),
		traceattrs.OpenTofuModuleSource(module.Module()),
		traceattrs.OpenTofuModuleVersion(version),
	))
	defer span.End()

	host, err := module.SvcHost()
	if err != nil {
		return nil, err
	}

	service, err := c.Discover(ctx, host, modulesServiceID)
	if err != nil {
		return nil, err
	}

	var p *url.URL
	if version == "" {
		p, err = url.Parse(path.Join(module.Module(), "download"))
	} else {
		p, err = url.Parse(path.Join(module.Module(), version, "download"))
	}
	if err != nil {
		return nil, err
	}
	download := service.ResolveReference(p)

	log.Printf("[DEBUG] looking up module location from %q", download)

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", download.String(), nil)
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
				module, version, body, err)
		}

		if v.UseRegistryCredentials == nil {
			// The registry has not opted in to "direct" installation, so we
			// assume that it wants the old-style "indirect" behavior where
			// the registry is essentially just an lookup table for
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
				return preparePackageLocationIndirect(resp.Header.Get(xTerraformGet), module, download)
			}
			return preparePackageLocationIndirect(v.Location, module, download)
		}

		// Otherwise, the registry has opted in to the new-style "direct"
		// installation approach, where the registry returns a URL that's under
		// its own control and we fetch from it directly instead of delegating
		// to go-getter.
		return preparePackageLocationDirect(v.Location, module, download, bool(*v.UseRegistryCredentials))

	case http.StatusNoContent:
		// FALLBACK: set the found location from the header
		return preparePackageLocationIndirect(resp.Header.Get(xTerraformGet), module, download)

	case http.StatusNotFound:
		return nil, fmt.Errorf("module %q version %q not found", module, version)

	default:
		// anything else is an error:
		return nil, fmt.Errorf("error getting download location for %q: %s resp:%s", module, resp.Status, body)
	}
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

	host, err := location.module.SvcHost()
	if err != nil {
		// We should not get here because location.module should be populated
		// correctly by [Client.ModuleLocation].
		return "", fmt.Errorf("package location has invalid registry hostname: %w", err)
	}
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
	if location.module.RawSubmodule != "" {
		subDir := filepath.FromSlash(location.module.RawSubmodule)
		modDir = filepath.Join(modDir, subDir)
	}
	return modDir, nil
}

func preparePackageLocationIndirect(realAddrRaw string, forModule *regsrc.Module, baseURL *url.URL) (PackageLocation, error) {
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
	// source address and the go-getter-style address returned frmo the registry
	// include a "subdirectory" component, in which case we need to resolve
	// the final effective subdirectory path that combines both.
	realAddr = realAddr.FromRegistry(
		// Unfortunately we have some tech debt here where this old registry
		// client code uses some older conventions for representing module
		// registry addresses, so we need to adapt this to the modern
		// representation.
		forModule.AsModuleSourceRegistry(),
	)

	return PackageLocationIndirect{
		SourceAddr: realAddr,
	}, nil
}

func preparePackageLocationDirect(locationRaw string, originalAddr *regsrc.Module, baseURL *url.URL, useRegistryCredentials bool) (PackageLocation, error) {
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
		module:                 originalAddr,
		packageURL:             packageURL,
		useRegistryCredentials: useRegistryCredentials,
	}, nil
}
