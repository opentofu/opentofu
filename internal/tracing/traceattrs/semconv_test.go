// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package traceattrs

import (
	"testing"
)

func TestNewResource(t *testing.T) {
	rsrc, err := NewResource(t.Context(), "Default Service Name")
	if err != nil {
		t.Errorf("failed to create OpenTelemetry SDK resource: %s", err)
		t.Errorf("If the above error message is about conflicting schema versions, then make sure that the semconv package imported in semconv.go matches the semconv package imported by \"go.opentelemetry.io/otel/sdk/resource\".")
		t.FailNow()
	}
	var serviceName string
	for _, kv := range rsrc.Attributes() {
		if kv.Key == "service.name" {
			serviceName = kv.Value.AsString()
		}
	}
	if got, want := serviceName, "Default Service Name"; got != want {
		t.Errorf("wrong service.name value %q; want %q", got, want)
	}
}

func TestNewResourceWithAttrsFromEnv(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "host.name=foobarbaz,service.name=SomethingElse")
	rsrc, err := NewResource(t.Context(), "test-service")
	if err != nil {
		t.Fatalf("failed to create OpenTelemetry SDK resource: %s", err)
	}
	var hostname, serviceName string
	for _, kv := range rsrc.Attributes() {
		if kv.Key == "host.name" {
			hostname = kv.Value.AsString()
		}
		if kv.Key == "service.name" {
			serviceName = kv.Value.AsString()
		}
	}
	if got, want := serviceName, "SomethingElse"; got != want {
		t.Errorf("wrong service.name value %q; want %q", got, want)
	}
	if got, want := hostname, "foobarbaz"; got != want {
		t.Errorf("wrong host.name value %q; want %q", got, want)
		t.Log("(if the actual host.name value matches the system where you're running this test then the detector priority order is probably wrong: WithFromEnv should come last)")
	}
}
