// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package traceattrs

import (
	"testing"
)

func TestNewResource(t *testing.T) {
	_, err := NewResource(t.Context(), "test-service")
	if err != nil {
		t.Errorf("failed to create OpenTelemetry SDK resource: %s", err)
		t.Errorf("If the above error message is about conflicting schema versions, then make sure that the semconv package imported in semconv.go matches the semconv package imported by \"go.opentelemetry.io/otel/sdk/resource\".")
	}
}
