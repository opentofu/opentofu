// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hashicorp/hcl/v2"
)

// TestParseLoadConfigFileSuccess is a simple test that just verifies that
// a number of test configuration files (in testdata/valid-files) can
// be parsed without raising any diagnostics.
//
// This test does not verify that reading these files produces the correct
// file element contents. More detailed assertions may be made on some subset
// of these configuration files in other tests.
func TestParserLoadConfigFileSuccess(t *testing.T) {
	files, err := os.ReadDir("testdata/valid-files")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range files {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/valid-files", name))
			if err != nil {
				t.Fatal(err)
			}

			parser := testParser(map[string]string{
				name: string(src),
			})

			_, diags := parser.LoadConfigFile(name)
			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}
		})
	}
}

// TestParseLoadConfigFileFailure is a simple test that just verifies that
// a number of test configuration files (in testdata/invalid-files)
// produce errors as expected.
//
// This test does not verify specific error messages, so more detailed
// assertions should be made on some subset of these configuration files in
// other tests.
func TestParserLoadConfigFileFailure(t *testing.T) {
	files, err := os.ReadDir("testdata/invalid-files")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range files {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/invalid-files", name))
			if err != nil {
				t.Fatal(err)
			}

			parser := testParser(map[string]string{
				name: string(src),
			})

			_, diags := parser.LoadConfigFile(name)
			if !diags.HasErrors() {
				t.Errorf("LoadConfigFile succeeded; want errors")
			}
			for _, diag := range diags {
				t.Logf("- %s", diag)
			}
		})
	}
}

// This test uses a subset of the same fixture files as
// TestParserLoadConfigFileFailure, but additionally verifies that each
// file produces the expected diagnostic summary and detail.
func TestParserLoadConfigFileFailureMessages(t *testing.T) {
	tests := []struct {
		Filename     string
		WantSeverity hcl.DiagnosticSeverity
		WantDiag     string
		WantDetail   string
	}{
		{
			"invalid-files/data-resource-lifecycle.tf",
			hcl.DiagError,
			"Invalid data resource lifecycle argument",
			`The lifecycle argument "ignore_changes" is defined only for managed resources ("resource" blocks), and is not valid for data resources.`,
		},
		{
			"invalid-files/variable-type-unknown.tf",
			hcl.DiagError,
			"Invalid type specification",
			`The keyword "notatype" is not a valid type specification.`,
		},
		{
			"invalid-files/unexpected-attr.tf",
			hcl.DiagError,
			"Unsupported argument",
			`An argument named "foo" is not expected here.`,
		},
		{
			"invalid-files/unexpected-block.tf",
			hcl.DiagError,
			"Unsupported block type",
			`Blocks of type "varyable" are not expected here. Did you mean "variable"?`,
		},
		{
			"invalid-files/resource-count-and-for_each.tf",
			hcl.DiagError,
			`Invalid combination of "count" and "for_each"`,
			`The "count" and "for_each" meta-arguments are mutually-exclusive, only one should be used to be explicit about the number of resources to be created.`,
		},
		{
			"invalid-files/data-count-and-for_each.tf",
			hcl.DiagError,
			`Invalid combination of "count" and "for_each"`,
			`The "count" and "for_each" meta-arguments are mutually-exclusive, only one should be used to be explicit about the number of resources to be created.`,
		},
		{
			"invalid-files/resource-lifecycle-badbool.tf",
			hcl.DiagError,
			"Unsuitable value type",
			`Unsuitable value: a bool is required`,
		},
		{
			"invalid-files/variable-complex-bad-default-inner-obj.tf",
			hcl.DiagError,
			"Invalid default value for variable",
			`This default value is not compatible with the variable's type constraint: ["mykey"].field: a bool is required.`,
		},
	}

	for _, test := range tests {
		t.Run(test.Filename, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", test.Filename))
			if err != nil {
				t.Fatal(err)
			}

			parser := testParser(map[string]string{
				test.Filename: string(src),
			})

			_, diags := parser.LoadConfigFile(test.Filename)
			if len(diags) != 1 {
				t.Errorf("Wrong number of diagnostics %d; want 1", len(diags))
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
				return
			}
			if diags[0].Severity != test.WantSeverity {
				t.Errorf("Wrong diagnostic severity %#v; want %#v", diags[0].Severity, test.WantSeverity)
			}
			if diags[0].Summary != test.WantDiag {
				t.Errorf("Wrong diagnostic summary\ngot:  %s\nwant: %s", diags[0].Summary, test.WantDiag)
			}
			if diags[0].Detail != test.WantDetail {
				t.Errorf("Wrong diagnostic detail\ngot:  %s\nwant: %s", diags[0].Detail, test.WantDetail)
			}
		})
	}
}

// TestParseLoadConfigFileWarning is a test that verifies files from
// testdata/warning-files produce particular warnings.
//
// This test does not verify that reading these files produces the correct
// file element contents in spite of those warnings. More detailed assertions
// may be made on some subset of these configuration files in other tests.
func TestParserLoadConfigFileWarning(t *testing.T) {
	files, err := os.ReadDir("testdata/warning-files")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range files {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/warning-files", name))
			if err != nil {
				t.Fatal(err)
			}

			// First we'll scan the file to see what warnings are expected.
			// That's declared inside the files themselves by using the
			// string "WARNING: " somewhere on each line that is expected
			// to produce a warning, followed by the expected warning summary
			// text. A single-line comment (with #) is the main way to do that.
			const marker = "WARNING: "
			sc := bufio.NewScanner(bytes.NewReader(src))
			wantWarnings := make(map[int]string)
			lineNum := 1
			for sc.Scan() {
				lineText := sc.Text()
				if idx := strings.Index(lineText, marker); idx != -1 {
					summaryText := lineText[idx+len(marker):]
					wantWarnings[lineNum] = summaryText
				}
				lineNum++
			}

			parser := testParser(map[string]string{
				name: string(src),
			})

			_, diags := parser.LoadConfigFile(name)
			if diags.HasErrors() {
				t.Errorf("unexpected error diagnostics")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}

			gotWarnings := make(map[int]string)
			for _, diag := range diags {
				if diag.Severity != hcl.DiagWarning || diag.Subject == nil {
					continue
				}
				gotWarnings[diag.Subject.Start.Line] = diag.Summary
			}

			if diff := cmp.Diff(wantWarnings, gotWarnings); diff != "" {
				t.Errorf("wrong warnings\n%s", diff)
			}
		})
	}
}

// TestParseLoadConfigFileError is a test that verifies files from
// testdata/warning-files produce particular errors.
//
// This test does not verify that reading these files produces the correct
// file element contents in spite of those errors. More detailed assertions
// may be made on some subset of these configuration files in other tests.
func TestParserLoadConfigFileError(t *testing.T) {
	files, err := os.ReadDir("testdata/error-files")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range files {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/error-files", name))
			if err != nil {
				t.Fatal(err)
			}

			// First we'll scan the file to see what warnings are expected.
			// That's declared inside the files themselves by using the
			// string "ERROR: " somewhere on each line that is expected
			// to produce a warning, followed by the expected warning summary
			// text. A single-line comment (with #) is the main way to do that.
			const marker = "ERROR: "
			sc := bufio.NewScanner(bytes.NewReader(src))
			wantErrors := make(map[int]string)
			lineNum := 1
			for sc.Scan() {
				lineText := sc.Text()
				if idx := strings.Index(lineText, marker); idx != -1 {
					summaryText := lineText[idx+len(marker):]
					wantErrors[lineNum] = summaryText
				}
				lineNum++
			}

			parser := testParser(map[string]string{
				name: string(src),
			})

			file, diags := parser.LoadConfigFile(name)
			// TODO many of these errors are now deferred until module loading
			// This is a structural issue which existed before static evaluation, but has been made worse by it
			// See https://github.com/opentofu/opentofu/issues/1467 for more details
			eval := NewStaticEvaluator(nil, RootModuleCallForTesting())
			for _, mc := range file.ModuleCalls {
				mDiags := mc.decodeStaticFields(eval)
				diags = append(diags, mDiags...)
			}

			gotErrors := make(map[int]string)
			for _, diag := range diags {
				if diag.Severity != hcl.DiagError || diag.Subject == nil {
					continue
				}
				gotErrors[diag.Subject.Start.Line] = diag.Summary
			}

			if diff := cmp.Diff(wantErrors, gotErrors); diff != "" {
				t.Errorf("wrong errors\n%s", diff)
			}
		})
	}
}
