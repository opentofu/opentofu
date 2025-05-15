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
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform-svchost/disco"
	otelAttr "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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
func (c *Client) Discover(host svchost.Hostname, serviceID string) (*url.URL, error) {
	service, err := c.services.DiscoverServiceURL(host, serviceID)
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
	ctx, span := tracing.Tracer().Start(ctx, "List Versions",
		trace.WithAttributes(
			otelAttr.String("opentofu.module.name", module.RawName),
		),
	)
	defer span.End()

	host, err := module.SvcHost()
	if err != nil {
		return nil, err
	}

	service, err := c.Discover(host, modulesServiceID)
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

	c.addRequestCreds(host, req.Request)
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

func (c *Client) addRequestCreds(host svchost.Hostname, req *http.Request) {
	creds, err := c.services.CredentialsForHost(host)
	if err != nil {
		log.Printf("[WARN] Failed to get credentials for %s: %s (ignoring)", host, err)
		return
	}

	if creds != nil {
		creds.PrepareRequest(req)
	}
}

// ModuleLocation find the download location for a specific version module.
// This returns a string, because the final location may contain special go-getter syntax.
func (c *Client) ModuleLocation(ctx context.Context, module *regsrc.Module, version string) (string, error) {
	ctx, span := tracing.Tracer().Start(ctx, "Find Module Location",
		trace.WithAttributes(
			otelAttr.String(traceattrs.ModuleCallName, module.RawName),
			otelAttr.String(traceattrs.ModuleSource, module.Module()),
			otelAttr.String(traceattrs.ModuleVersion, version),
		),
	)
	defer span.End()

	host, err := module.SvcHost()
	if err != nil {
		return "", err
	}

	service, err := c.Discover(host, modulesServiceID)
	if err != nil {
		return "", err
	}

	var p *url.URL
	if version == "" {
		p, err = url.Parse(path.Join(module.Module(), "download"))
	} else {
		p, err = url.Parse(path.Join(module.Module(), version, "download"))
	}
	if err != nil {
		return "", err
	}
	download := service.ResolveReference(p)

	log.Printf("[DEBUG] looking up module location from %q", download)

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", download.String(), nil)
	if err != nil {
		return "", err
	}

	req = req.WithContext(ctx)

	c.addRequestCreds(host, req.Request)
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
				module, version, body, err)
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
		return "", fmt.Errorf("module %q version %q not found", module, version)

	default:
		// anything else is an error:
		return "", fmt.Errorf("error getting download location for %q: %s resp:%s", module, resp.Status, body)
	}

	if location == "" {
		return "", fmt.Errorf("failed to get download URL for %q: %s resp:%s", module, resp.Status, body)
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
			return "", fmt.Errorf("invalid relative URL for %q: %w", module, err)
		}
		locationURL = download.ResolveReference(locationURL)
		location = locationURL.String()
	}

	return location, nil
}
