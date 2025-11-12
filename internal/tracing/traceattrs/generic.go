// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package traceattrs

import (
	"go.opentelemetry.io/otel/attribute"
)

// String wraps [attribute.String] just so that we can keep most of our direct
// OpenTelemetry package imports centralized in this package where it's
// easier to keep our version selections consistent.
func String(name string, val string) attribute.KeyValue {
	return attribute.String(name, val)
}

// StringSlice wraps [attribute.StringSlice] just so that we can keep most of
// our direct OpenTelemetry package imports centralized in this package where
// it's easier to keep our version selections consistent.
//
// If the items you want to report are not yet assembled into a string slice,
// consider using [tracing.StringSlice] with an [iter.Seq[string]] argument
// to skip constructing the slice when tracing isn't enabled.
func StringSlice(name string, val []string) attribute.KeyValue {
	return attribute.StringSlice(name, val)
}

// Bool wraps [attribute.Bool] just so that we can keep most of our direct
// OpenTelemetry package imports centralized in this package where it's
// easier to keep our version selections consistent.
func Bool(name string, val bool) attribute.KeyValue {
	return attribute.Bool(name, val)
}

// Int64 wraps [attribute.Int64] just so that we can keep most of our direct
// OpenTelemetry package imports centralized in this package where it's
// easier to keep our version selections consistent.
func Int64(name string, val int64) attribute.KeyValue {
	return attribute.Int64(name, val)
}
