package views

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestViewDiagnosticsInPedanticMode(t *testing.T) {
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

	diags := tfdiags.Diagnostics{tfdiags.SimpleWarning("TEST")}
	view.Diagnostics(diags)

	writer.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("error reading from reader: %s", err)
	}

	got := strings.TrimSpace(string(out))
	want := fmt.Sprintf("%s: TEST", tfdiags.Error)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected: %v got: %v", want, got)
	}
}
