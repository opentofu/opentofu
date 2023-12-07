// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"github.com/opentofu/opentofu/version"
)

// New returns the DefaultPooledClient from the cleanhttp
// package that will also send a OpenTofu User-Agent string.
func New() *http.Client {
	cli := cleanhttp.DefaultPooledClient()
	cli.Transport = &userAgentRoundTripper{
		userAgent: OpenTofuUserAgent(version.Version),
		inner:     cli.Transport,
	}
	return cli
}

// NewWithCustomTrustedCertificates is identical to New but trusts the provided certificate pool instead of the default
// system pool. This is useful mainly for testing against mock servers with dynamically generated certificates.
func NewWithCustomTrustedCertificates(pool *x509.CertPool) *http.Client {
	return &http.Client{
		Transport: NewTransportWithCustomTrustedCertificates(pool),
	}
}

// NewTransportWithCustomTrustedCertificates returns an HTTP transport usable with an HTTP client with a custom
// trusted certificate pool. This is useful mainly for testing against mock servers with dynamically generated
// certificates.
func NewTransportWithCustomTrustedCertificates(pool *x509.CertPool) http.RoundTripper {
	transport := cleanhttp.DefaultPooledTransport()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	transport.TLSClientConfig.RootCAs = pool

	return &userAgentRoundTripper{
		userAgent: OpenTofuUserAgent(version.Version),
		inner:     transport,
	}
}
