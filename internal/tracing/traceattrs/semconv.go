// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package traceattrs

// This file contains wrappers and reexports of some symbols from the
// OpenTelemetry "semconv" package and the "resource" package from the
// OpenTelemetry SDK, which we centralize here because their version numbers
// must be coordinated carefully to avoid runtime panics of mismatched versions.

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk"
	"go.opentelemetry.io/otel/sdk/resource"

	// The version number at the end of this package path MUST match the
	// semconv version imported by the "go.opentelemetry.io/otel/sdk/resource",
	// because we also use some semconv symbols indirectly through that
	// package, and so we need to update this each time we upgrade the module
	// "go.opentelemetry.io/otel/sdk".
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/opentofu/opentofu/version"
)

// NewResource constructs a *resource.Resource that should be used when
// constructing our global tracer provider.
//
// This is factored out here because its correct behavior depends on correctly
// matching our import of an "go.opentelemetry.io/otel/semconv/*" package
// for direct attribute definitions with the version used indirectly by
// "go.opentelemetry.io/otel/sdk/resource". If they don't match then this
// function will fail with an error.
//
// The unit test [TestNewResource] runs this function in isolation so we can
// make sure it succeeds without having to actually initialize the telemetry
// system.
func NewResource(ctx context.Context, serviceName string) (*resource.Resource, error) {
	return resource.New(ctx,
		// Use built-in detectors to simplify the collation of the tracing information
		resource.WithOS(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(),

		// Add custom service attributes
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version.Version),

			// We add in the telemetry SDK information so that we don't end up with
			// duplicate schema urls that clash
			semconv.TelemetrySDKName("opentelemetry"),
			semconv.TelemetrySDKLanguageGo,
			semconv.TelemetrySDKVersion(sdk.Version()),
		),
	)
}

// URLFull returns an attribute representing an absolute URL associated with
// a trace span, using the attribute name defined by our currently-selected
// version of the OpenTelemetry semantic conventions.
//
// This wraps [semconv.URLFull].
func URLFull(val string) attribute.KeyValue {
	return semconv.URLFull(val)
}

// FilePath returns an attribute representing an absolute file path associated
// with a trace span, using the attribute name defined by our currently-selected
// version of the OpenTelemetry semantic conventions.
//
// This wraps [semconv.FilePath].
func FilePath(val string) attribute.KeyValue {
	return semconv.FilePath(val)
}

// FileSize returns an attribute representing the size in bytes of a file
// associated with a trace span, using the attribute name defined by our
// currently-selected version of the OpenTelemetry semantic conventions.
//
// This wraps [semconv.FileSize].
func FileSize(val int) attribute.KeyValue {
	return semconv.FileSize(val)
}

// OCIManifestDigest returns an attribute representing an OCI manifest
// digest associated with a trace span, using the attribute name defined
// by our currently-selected version of the OpenTelemetry semantic conventions.
//
// This wraps [semconv.OCIManifestDigest].
func OCIManifestDigest(digest string) attribute.KeyValue {
	return semconv.OCIManifestDigest(digest)
}
