// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/terminal"
)

func TestNewProvidersSchema(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	defer done(t)

	view := NewView(streams)

	// Test ViewJSON creates ProvidersSchemaJSON
	optsJSON := arguments.ViewOptions{ViewType: arguments.ViewJSON}
	psJSON := NewProvidersSchema(optsJSON, view)
	if _, ok := psJSON.(*ProvidersSchemaJSON); !ok {
		t.Errorf("Expected *ProvidersSchemaJSON for ViewJSON, got %T", psJSON)
	}

	// Test ViewHuman creates ProvidersSchemaHuman
	optsHuman := arguments.ViewOptions{ViewType: arguments.ViewHuman}
	psHuman := NewProvidersSchema(optsHuman, view)
	if _, ok := psHuman.(*ProvidersSchemaHuman); !ok {
		t.Errorf("Expected *ProvidersSchemaHuman for ViewHuman, got %T", psHuman)
	}
}

func TestProvidersSchemaHuman_UnsupportedLocalOp(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)

	psHuman := &ProvidersSchemaHuman{view: view}
	psHuman.UnsupportedLocalOp()

	output := done(t)
	stderr := output.Stderr()

	if !strings.Contains(stderr, "Unsupported local operation") {
		t.Errorf("Expected 'Unsupported local operation' in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "This command requires a local workspace.") {
		t.Errorf("Expected 'This command requires a local workspace.' in stderr, got: %s", stderr)
	}
}

func TestProvidersSchemaJSON_UnsupportedLocalOp(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)

	psJSON := &ProvidersSchemaJSON{view: view}
	psJSON.UnsupportedLocalOp()

	output := done(t)
	stderr := output.Stderr()

	if !strings.Contains(stderr, "Unsupported local operation") {
		t.Errorf("Expected 'Unsupported local operation' in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "This command requires a local workspace.") {
		t.Errorf("Expected 'This command requires a local workspace.' in stderr, got: %s", stderr)
	}
}

func TestProvidersSchemaHuman_OutputPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected Output() to panic for ProvidersSchemaHuman")
		} else {
			if msg, ok := r.(string); ok && msg != "can't produce output in human view for providers schema" {
				t.Errorf("Unexpected panic message: %v", msg)
			}
		}
	}()

	streams, done := terminal.StreamsForTesting(t)
	defer done(t)
	view := NewView(streams)

	psHuman := &ProvidersSchemaHuman{view: view}
	psHuman.Output("{}")
}

func TestProvidersSchemaJSON_Output(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)

	psJSON := &ProvidersSchemaJSON{view: view}
	expectedJSON := `{"format_version":"1.0","provider_schemas":{}}`
	psJSON.Output(expectedJSON)

	output := done(t)
	stdout := output.Stdout()

	// Should completely match the expected JSON with a newline implicitly appended by Print
	if strings.TrimSpace(stdout) != expectedJSON {
		t.Errorf("Expected stdout to be %q, got %q", expectedJSON, strings.TrimSpace(stdout))
	}
}
