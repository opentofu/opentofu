// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"reflect"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestView_DiagnosticsInPedanticMode(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)
	view.PedanticMode = true

	diags := tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Warning, "Output as error", "")}
	view.Diagnostics(diags)

	got := strings.TrimSpace(done(t).Stderr())
	want := "Error: Output as error"

	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected: %v got: %v", want, got)
	}

	if !view.LegacyViewPedanticErrors {
		t.Errorf("expected: true, got: %v", view.LegacyViewPedanticErrors)
	}
}
