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

	// Test NewProvidersSchema creates ProvidersSchemaMixed
	opts := arguments.ViewOptions{ViewType: arguments.ViewJSON}
	ps := NewProvidersSchema(opts, view)
	if _, ok := ps.(*ProvidersSchemaMixed); !ok {
		t.Errorf("Expected *ProvidersSchemaMixed, got %T", ps)
	}
}

func TestProvidersSchemaMixed_UnsupportedLocalOp(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)

	ps := &ProvidersSchemaMixed{view: view}
	ps.UnsupportedLocalOp()

	output := done(t)
	stderr := output.Stderr()

	if !strings.Contains(stderr, "The configured backend doesn't support this operation.") {
		t.Errorf("Expected 'The configured backend doesn't support this operation.' in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "Your configuration") {
		t.Errorf("Expected 'Your configuration' in stderr, got: %s", stderr)
	}
}

func TestProvidersSchemaMixed_Output(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)

	ps := &ProvidersSchemaMixed{view: view}
	expectedJSON := `{"format_version":"1.0","provider_schemas":{}}`
	ps.Output(expectedJSON)

	output := done(t)
	stdout := output.Stdout()

	// Should completely match the expected JSON with a newline implicitly appended by Print
	if strings.TrimSpace(stdout) != expectedJSON {
		t.Errorf("Expected stdout to be %q, got %q", expectedJSON, strings.TrimSpace(stdout))
	}
}
