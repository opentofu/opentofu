// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"encoding/json"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/workdir"
)

func TestMetadataFunctions_error(t *testing.T) {
	view, done := testView(t)
	c := &MetadataFunctionsCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	// This test will always error because it's missing the -json flag
	code := c.Run(nil)
	output := done(t)
	if code != cli.RunResultHelp {
		t.Fatalf("expected error, got:\n%s", output.All())
	}
}

func TestMetadataFunctions_output(t *testing.T) {
	view, done := testView(t)
	m := Meta{
		View: view,
	}
	c := &MetadataFunctionsCommand{Meta: m}

	code := c.Run([]string{"-json"})
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit status %d; want 0\nstderr: %s", code, output.Stderr())
	}

	var got functions
	gotString := output.Stdout()
	err := json.Unmarshal([]byte(gotString), &got)
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Signatures) < 100 {
		t.Fatalf("expected at least 100 function signatures, got %d", len(got.Signatures))
	}

	// check if one particular stable function is correct
	gotMax, ok := got.Signatures["max"]
	wantMax := "{\"description\":\"`max` takes one or more numbers and returns the greatest number from the set.\",\"return_type\":\"number\",\"variadic_parameter\":{\"name\":\"numbers\",\"type\":\"number\"}}"
	if !ok {
		t.Fatal(`missing function signature for "max"`)
	}
	if string(gotMax) != wantMax {
		t.Fatalf("wrong function signature for \"max\":\ngot: %q\nwant: %q", gotMax, wantMax)
	}

	stderr := output.Stderr()
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
}

type functions struct {
	FormatVersion string                     `json:"format_version"`
	Signatures    map[string]json.RawMessage `json:"function_signatures,omitempty"`
}
