// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package views

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestMetadataFunctions_Diagnostics(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v MetadataFunctions)
		wantStdout string
		wantStderr string
	}{
		// Diagnostics
		"warning": {
			viewCall: func(v MetadataFunctions) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
		},
		"error": {
			viewCall: func(v MetadataFunctions) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
		},
		"multiple_diagnostics": {
			viewCall: func(v MetadataFunctions) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testMetadataFunctionsHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
		})
	}
}

func TestMetadataFunctions_printFunctions(t *testing.T) {
	view, done := testView(t)
	v := NewMetadataFunctions(view)
	status := v.PrintFunctions()
	output := done(t)
	if !status {
		t.Fatalf("failed to generate the functions output: %s", output.All())
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

	// test that ignored functions are not part of the json
	for _, v := range ignoredFunctions {
		if _, ok := got.Signatures[v.Name]; ok {
			t.Errorf("found ignored function %q inside output", v)
		}
		corePrefixed := v.String()
		if _, ok := got.Signatures[corePrefixed]; ok {
			t.Fatalf("found ignored function %q inside output", corePrefixed)
		}
	}
}

type functions struct {
	FormatVersion string                     `json:"format_version"`
	Signatures    map[string]json.RawMessage `json:"function_signatures,omitempty"`
}

func testMetadataFunctionsHuman(t *testing.T, call func(v MetadataFunctions), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewMetadataFunctions(view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}
