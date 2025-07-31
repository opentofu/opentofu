// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-retryablehttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/tracing"
)

// PackageHTTPURL is a provider package location accessible via HTTP.
//
// Its value is a URL string using either the http: scheme or the https: scheme.
// The URL should respond with a .zip archive whose contents are to be extracted
// into a local package directory.
type PackageHTTPURL string

var _ PackageLocation = PackageHTTPURL("")

func (p PackageHTTPURL) String() string { return string(p) }

func (p PackageHTTPURL) InstallProviderPackage(ctx context.Context, meta PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	url := meta.Location.String()

	ctx, span := tracing.Tracer().Start(ctx, "Install (http)", trace.WithAttributes(
		semconv.URLFull(url),
	))
	defer span.End()

	// When we're installing from an HTTP URL we expect the URL to refer to
	// a zip file. We'll fetch that into a temporary file here and then
	// delegate to installFromLocalArchive below to actually extract it.
	// (We're not using go-getter here because its HTTP getter has a bunch
	// of extraneous functionality we don't need or want, like indirection
	// through X-Terraform-Get header, attempting partial fetches for
	// files that already exist, etc.)

	retryableClient := retryablehttp.NewClient()
	retryableClient.HTTPClient = httpclient.New(ctx)
	retryableClient.RetryMax = maxHTTPPackageRetryCount
	retryableClient.RequestLogHook = func(logger retryablehttp.Logger, _ *http.Request, i int) {
		if i > 0 {
			logger.Printf("[INFO] failed to fetch provider package; retrying")
		}
	}
	retryableClient.Logger = log.New(logging.LogOutput(), "", log.Flags())

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid provider download request: %w", err)
	}
	resp, err := retryableClient.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			// "context canceled" is not a user-friendly error message,
			// so we'll return a more appropriate one here.
			return nil, fmt.Errorf("provider download was interrupted")
		}
		return nil, fmt.Errorf("%s: %w", HostFromRequest(req.Request), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsuccessful request to %s: %s", url, resp.Status)
	}

	f, err := os.CreateTemp("", "terraform-provider")
	if err != nil {
		return nil, fmt.Errorf("failed to open temporary file to download from %s: %w", url, err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// We'll borrow go-getter's "cancelable copy" implementation here so that
	// the download can potentially be interrupted partway through.
	n, err := getter.Copy(ctx, f, resp.Body)
	if err == nil && n < resp.ContentLength {
		err = fmt.Errorf("incorrect response size: expected %d bytes, but got %d bytes", resp.ContentLength, n)
	}
	if err != nil {
		return nil, err
	}

	archiveFilename := f.Name()
	localLocation := PackageLocalArchive(archiveFilename)

	var authResult *PackageAuthenticationResult
	if meta.Authentication != nil {
		if authResult, err = meta.Authentication.AuthenticatePackage(localLocation); err != nil {
			return authResult, err
		}
	}

	// We can now delegate to localLocation for extraction. To do so,
	// we construct a new package meta description using the local archive
	// path as the location, and skipping authentication. installFromLocalMeta
	// is responsible for verifying that the archive matches the allowedHashes,
	// though.
	localMeta := PackageMeta{
		Provider:         meta.Provider,
		Version:          meta.Version,
		ProtocolVersions: meta.ProtocolVersions,
		TargetPlatform:   meta.TargetPlatform,
		Filename:         meta.Filename,
		Location:         localLocation,
		Authentication:   nil,
	}
	if _, err := localLocation.InstallProviderPackage(ctx, localMeta, targetDir, allowedHashes); err != nil {
		return nil, err
	}
	return authResult, nil
}

const (
	// httpClientRetryCountEnvName is the environment variable name used to customize
	// the HTTP retry count for module downloads.
	httpClientRetryCountEnvName = "TF_PROVIDER_DOWNLOAD_RETRY"

	httpClientDefaultRetry = 2
)

//nolint:gochecknoinits // this init function predates our use of this linter
func init() {
	configureProviderDownloadRetry()
}

var (
	//nolint:gochecknoglobals // this variable predates our use of this linter
	maxHTTPPackageRetryCount int
)

// will attempt for requests with retryable errors, like 502 status codes
func configureProviderDownloadRetry() {
	maxHTTPPackageRetryCount = httpClientDefaultRetry
	if v := os.Getenv(httpClientRetryCountEnvName); v != "" {
		retry, err := strconv.Atoi(v)
		if err == nil && retry > 0 {
			maxHTTPPackageRetryCount = retry
		}
	}
}
