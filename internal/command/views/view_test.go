package views

import (
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"reflect"
	"strings"
	"testing"
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

	if !view.WarningFlagged {
		t.Errorf("expected: true, got: %v", view.WarningFlagged)
	}
}
