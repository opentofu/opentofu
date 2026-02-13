// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package httpclient

import (
	"io"
	"net/http"
	"sync"

	"github.com/opentofu/opentofu/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// tracingTransport wraps a http.RoundTripper to augment
// traces for outgoing HTTP requests with additional attributes
// that may otherwise only be available via metrics from otelhttp
type tracingTransport struct {
	inner http.RoundTripper
}

var _ http.RoundTripper = (*tracingTransport)(nil)

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Get the span from the response's request context, which contains
	// the HTTP request span that otelhttp created (not the parent span)
	if resp.Request != nil {
		span := tracing.SpanFromContext(resp.Request.Context())
		if span != nil && span.IsRecording() {
			t.addResponseHeaderAttributes(span, resp)

			if resp.Body != nil {
				resp.Body = &trackingReadCloser{
					inner: resp.Body,
					span:  span,
				}
			}
		}
	}

	return resp, nil
}

// capturedHeaders defines the HTTP response headers to capture as span attributes. (adhering to the http semconv)
// Keys are the HTTP header names (case-insensitive), values are the OpenTelemetry attribute names.
// Only headers that are useful for debugging and do NOT contain sensitive information should be added here.
// If you are adding something to this list, please consider whether it is likely to contain personally identifiable information or other sensitive data, and if so, do not add it here.
var capturedHeaders = map[string]string{
	// Content metadata
	"Content-Type":     "http.response.header.content-type",
	"Content-Encoding": "http.response.header.content-encoding",
	"Content-Length":   "http.response.header.content-length",

	// Cache validation
	"ETag":          "http.response.header.etag",
	"Last-Modified": "http.response.header.last-modified",

	// Cache status (CDN/registry performance)
	"X-Cache":         "http.response.header.x-cache",
	"X-Cache-Hits":    "http.response.header.x-cache-hits",
	"CF-Cache-Status": "http.response.header.cf-cache-status",
	"Age":             "http.response.header.age",
	"X-Served-By":     "http.response.header.x-served-by",
	"Via":             "http.response.header.via",

	// Performance timing
	"X-Timer":       "http.response.header.x-timer",
	"Server-Timing": "http.response.header.server-timing",

	// Request IDs (for support tickets)
	"X-GitHub-Request-Id": "http.response.header.x-github-request-id",
	"X-Request-Id":        "http.response.header.x-request-id",
	"X-Ms-Request-Id":     "http.response.header.x-ms-request-id",
	"X-Amz-Request-Id":    "http.response.header.x-amz-request-id",
	"CF-Ray":              "http.response.header.cf-ray",
}

// addResponseHeaderAttributes extracts relevant headers from the HTTP response
// and adds them as attributes to the given span. This method purposely focuses on a set amount of headers
// that SHOULD not contain sensitive information, and that are commonly used for debugging and performance analysis of HTTP interactions.
func (t *tracingTransport) addResponseHeaderAttributes(span tracing.Span, resp *http.Response) {
	// Capture configured headers
	for headerName, attrName := range capturedHeaders {
		if value := resp.Header.Get(headerName); value != "" {
			span.SetAttributes(attribute.String(attrName, value))
		}
	}

	// Special case: redirect location (only for 3xx responses)
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		// unconditionally capture this even if it's empty, this may help someone debug a misbehaving server that is sending back an invalid redirect with no location
		span.SetAttributes(attribute.String("http.response.header.location", location))
	}
}

// trackingReadCloser wraps an io.ReadCloser to track bytes read
type trackingReadCloser struct {
	inner     io.ReadCloser
	span      tracing.Span
	bytesRead int64
	closeOnce sync.Once
	closeErr  error
}

func (r *trackingReadCloser) Read(p []byte) (n int, err error) {
	// Simply track the number of bytes read and store it for use in the Close()
	n, err = r.inner.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func (r *trackingReadCloser) Close() error {
	// Ensure Close is only executed once, even if called multiple times
	r.closeOnce.Do(func() {
		// Always add the bytes downloaded as a span attribute, even if zero
		// This makes it clear that tracking is working vs missing data
		r.span.SetAttributes(attribute.Int64("http.response.body.size", r.bytesRead))
		r.closeErr = r.inner.Close()
	})
	return r.closeErr
}
