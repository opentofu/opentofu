// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"net/http"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"github.com/terramate-io/opentofulib/version"
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
