package views

import (
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestView_DiagnosticsInPedanticMode(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("error setting up reader and writer: %s", err)
	}

	streams := &terminal.Streams{
		Stderr: &terminal.OutputStream{
			File: writer,
		},
	}

	view := NewView(streams)
	view.PedanticMode = true

	diags := tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Warning, "Output as error", "")}
	view.Diagnostics(diags)

	writer.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("error reading from reader: %s", err)
	}

	got := strings.TrimSpace(string(out))
	want := "Error: Output as error"

	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected: %v got: %v", want, got)
	}

	if !view.WarningFlagged {
		t.Errorf("expected: true, got: %v", view.WarningFlagged)
	}
}
