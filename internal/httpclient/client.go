// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"context"
	"net/http"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	otelTrace "go.opentelemetry.io/otel/trace"

	"github.com/opentofu/opentofu/version"
)

// New returns the DefaultPooledClient from the cleanhttp
// package that will also send a OpenTofu User-Agent string.
//
// If the given context has an active OpenTelemetry trace span associated with
// it then the returned client is also configured to collect traces for
// outgoing requests. However, those traces will be children of the span
// associated with the context passed _in each individual request_, rather
// than of the span in the context passed to this function; this function
// only checks for the presence of any span as a heuristic for whether the
// caller is in a part of the codebase that has OpenTelemetry plumbing in
// place, and does not actually make use of any information from that span.
func New(ctx context.Context) *http.Client {
	cli := cleanhttp.DefaultPooledClient()
	cli.Transport = &userAgentRoundTripper{
		userAgent: OpenTofuUserAgent(version.Version),
		inner:     cli.Transport,
	}

	if span := otelTrace.SpanFromContext(ctx); span != nil && span.IsRecording() {
		// We consider the presence of an active span -- that is, one whose
		// presence is going to be reported to a trace collector outside of
		// the OpenTofu process -- as sufficient signal that generating
		// spans for requests made with the returned client will be useful.
		//
		// The following has two important implications:
		// - Any request made using the returned client will generate an
		//   OpenTelemetry tracing span using the standard semantic conventions
		//   for an outgoing HTTP request. Therefore all requests made with
		//   this client must also be passed a context.Context carrying a
		//   suitable parent span that the request will be reported as a child
		//   of.
		// - The outgoing request will include trace context metadata using
		//   the conventions from following specification, which would allow
		//   the recieving server to contribute its own child spans to the
		//   trace if it has access to the same collector:
		//
		//        https://www.w3.org/TR/trace-context/
		//
		// We do this only when there seems to be an active span because
		// otherwise each HTTP request without an active trace context will
		// cause a separate trace to begin, containing only that HTTP request,
		// which would create confusing noise for whoever is consuming the
		// traces.
		cli.Transport = otelhttp.NewTransport(cli.Transport)
	}

	return cli
}
