// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonentities

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/command/jsonplan"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestNewDiagnostic(t *testing.T) {
	// Common HCL for diags with source ranges. This does not have any real
	// semantic errors, but we can synthesize fake HCL errors which will
	// exercise the diagnostic rendering code using this
	sources := map[string]*hcl.File{
		"test.tf": {Bytes: []byte(`resource "test_resource" "test" {
  foo = var.boop["hello!"]
  bar = {
    baz = maybe
  }
}
`)},
		"short.tf":       {Bytes: []byte("bad source code")},
		"odd-comment.tf": {Bytes: []byte("foo\n\n#\n")},
		"values.tf": {Bytes: []byte(`[
  var.a,
  var.b,
  var.c,
  var.d,
  var.e,
  var.f,
  var.g,
  var.h,
  var.i,
  var.j,
  var.k,
]
`)},
		"test.tftest.hcl": {Bytes: []byte(`run "fails_without_useful_diff" {
  command = plan
  plan_options {
    refresh = false
  }
  assert {
    condition = 3 == 5
    error_message = "Error."
  }

  assert {
    condition = jsonencode(var.json_headers) == jsonencode([
      "Test-Header-1: foo",
      "Test-Header-2: bar",
    ])
    error_message = "Error."
  }
}
		`)},
	}
	testCases := map[string]struct {
		diag interface{} // allow various kinds of diags
		want *Diagnostic
	}{
		"sourceless warning": {
			tfdiags.Sourceless(
				tfdiags.Warning,
				"Oh no",
				"Something is broken",
			),
			&Diagnostic{
				Severity: "warning",
				Summary:  "Oh no",
				Detail:   "Something is broken",
			},
		},
		"error with source code unavailable": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad news",
				Detail:   "It went wrong",
				Subject: &hcl.Range{
					Filename: "modules/oops/missing.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 2, Column: 12, Byte: 33},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Bad news",
				Detail:   "It went wrong",
				Range: &DiagnosticRange{
					Filename: "modules/oops/missing.tf",
					Start: Pos{
						Line:   1,
						Column: 6,
						Byte:   5,
					},
					End: Pos{
						Line:   2,
						Column: 12,
						Byte:   33,
					},
				},
			},
		},
		"error with source code subject": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Tiny explosion",
				Detail:   "Unexpected detonation while parsing",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 10, Byte: 9},
					End:      hcl.Pos{Line: 1, Column: 25, Byte: 24},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Tiny explosion",
				Detail:   "Unexpected detonation while parsing",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   1,
						Column: 10,
						Byte:   9,
					},
					End: Pos{
						Line:   1,
						Column: 25,
						Byte:   24,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 `resource "test_resource" "test" {`,
					StartLine:            1,
					HighlightStartOffset: 9,
					HighlightEndOffset:   24,
					Values:               []DiagnosticExpressionValue{},
				},
			},
		},
		"error with source code subject but no context": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Nonsense input",
				Detail:   "What you wrote makes no sense",
				Subject: &hcl.Range{
					Filename: "short.tf",
					Start:    hcl.Pos{Line: 1, Column: 5, Byte: 4},
					End:      hcl.Pos{Line: 1, Column: 10, Byte: 9},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Nonsense input",
				Detail:   "What you wrote makes no sense",
				Range: &DiagnosticRange{
					Filename: "short.tf",
					Start: Pos{
						Line:   1,
						Column: 5,
						Byte:   4,
					},
					End: Pos{
						Line:   1,
						Column: 10,
						Byte:   9,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              nil,
					Code:                 (`bad source code`),
					StartLine:            (1),
					HighlightStartOffset: (4),
					HighlightEndOffset:   (9),
					Values:               []DiagnosticExpressionValue{},
				},
			},
		},
		"error with multi-line snippet": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "In this house we respect booleans",
				Detail:   "True or false, there is no maybe",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 4, Column: 11, Byte: 81},
					End:      hcl.Pos{Line: 4, Column: 16, Byte: 86},
				},
				Context: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 3, Column: 3, Byte: 63},
					End:      hcl.Pos{Line: 5, Column: 4, Byte: 90},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "In this house we respect booleans",
				Detail:   "True or false, there is no maybe",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   4,
						Column: 11,
						Byte:   81,
					},
					End: Pos{
						Line:   4,
						Column: 16,
						Byte:   86,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 "  bar = {\n    baz = maybe\n  }",
					StartLine:            3,
					HighlightStartOffset: 20,
					HighlightEndOffset:   25,
					Values:               []DiagnosticExpressionValue{},
				},
			},
		},
		"error with empty highlight range at end of source code": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "You forgot something",
				Detail:   "Please finish your thought",
				Subject: &hcl.Range{
					Filename: "short.tf",
					Start:    hcl.Pos{Line: 1, Column: 16, Byte: 15},
					End:      hcl.Pos{Line: 1, Column: 16, Byte: 15},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "You forgot something",
				Detail:   "Please finish your thought",
				Range: &DiagnosticRange{
					Filename: "short.tf",
					Start: Pos{
						Line:   1,
						Column: 16,
						Byte:   15,
					},
					End: Pos{
						Line:   1,
						Column: 17,
						Byte:   16,
					},
				},
				Snippet: &DiagnosticSnippet{
					Code:                 ("bad source code"),
					StartLine:            (1),
					HighlightStartOffset: (15),
					HighlightEndOffset:   (15),
					Values:               []DiagnosticExpressionValue{},
				},
			},
		},
		"error with unset highlight end position": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "There is no end",
				Detail:   "But there is a beginning",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 16, Byte: 15},
					End:      hcl.Pos{Line: 0, Column: 0, Byte: 0},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "There is no end",
				Detail:   "But there is a beginning",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   1,
						Column: 16,
						Byte:   15,
					},
					End: Pos{
						Line:   1,
						Column: 17,
						Byte:   16,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 `resource "test_resource" "test" {`,
					StartLine:            1,
					HighlightStartOffset: 15,
					HighlightEndOffset:   16,
					Values:               []DiagnosticExpressionValue{},
				},
			},
		},
		"error whose range starts at a newline": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid newline",
				Detail:   "How awkward!",
				Subject: &hcl.Range{
					Filename: "odd-comment.tf",
					Start:    hcl.Pos{Line: 2, Column: 5, Byte: 4},
					End:      hcl.Pos{Line: 3, Column: 1, Byte: 6},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Invalid newline",
				Detail:   "How awkward!",
				Range: &DiagnosticRange{
					Filename: "odd-comment.tf",
					Start: Pos{
						Line:   2,
						Column: 5,
						Byte:   4,
					},
					End: Pos{
						Line:   3,
						Column: 1,
						Byte:   6,
					},
				},
				Snippet: &DiagnosticSnippet{
					Code:      `#`,
					StartLine: 2,
					Values:    []DiagnosticExpressionValue{},

					// Due to the range starting at a newline on a blank
					// line, we end up stripping off the initial newline
					// to produce only a one-line snippet. That would
					// therefore cause the start offset to naturally be
					// -1, just before the Code we returned, but then we
					// force it to zero so that the result will still be
					// in range for a byte-oriented slice of Code.
					HighlightStartOffset: 0,
					HighlightEndOffset:   1,
				},
			},
		},
		"error with source code subject and known expression": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
					hcl.TraverseIndex{Key: cty.StringVal("hello!")},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.StringVal("bleurgh"),
							}),
						}),
					},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `var.boop["hello!"]`,
							Statement: `is "bleurgh"`,
						},
					},
				},
			},
		},
		"error with source code subject and expression with local symbol": {
			// The EvalContext in this case is set up to resemble what
			// happens in nested scopes such as those created by "for"
			// expressions, where there are local symbols that are not
			// subject to OpenTofu's usual rules for references and can
			// potentially shadow the prefixes normally used for references.
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					// NOTE: This subject is not particularly realistic
					// for this situation since the focus of this test
					// case is on the interpretation of the Expression
					// and EvalContext fields, and so we have this here
					// only because a source snippet is a prerequisite
					// for generating expression context.
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "i"},
				}),
				EvalContext: (func() *hcl.EvalContext {
					root := &hcl.EvalContext{
						Variables: map[string]cty.Value{
							"var": cty.ObjectVal(map[string]cty.Value{
								"boop": cty.MapVal(map[string]cty.Value{
									"hello!": cty.StringVal("bleurgh"),
								}),
							}),
						},
					}
					child := root.NewChild()
					child.Variables = map[string]cty.Value{
						"i": cty.NumberIntVal(5),
					}
					return child
				})(),
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `i`,
							Statement: `is 5`,
						},
					},
				},
			},
		},
		"error with source code subject and expression referring to sensitive value": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
					hcl.TraverseIndex{Key: cty.StringVal("hello!")},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.StringVal("bleurgh").Mark(marks.Sensitive),
							}),
						}),
					},
				},
				Extra: diagnosticCausedBySensitive(true),
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `var.boop["hello!"]`,
							Statement: `has a sensitive value`,
						},
					},
				},
			},
		},
		"error with source code subject and expression referring to sensitive value when not caused by sensitive values": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
					hcl.TraverseIndex{Key: cty.StringVal("hello!")},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.StringVal("bleurgh").Mark(marks.Sensitive),
							}),
						}),
					},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values:               []DiagnosticExpressionValue{
						// The sensitive value is filtered out because this is
						// not a sensitive-value-related diagnostic message.
					},
				},
			},
		},
		"error with source code subject and expression referring to a collection containing a sensitive value": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.StringVal("bleurgh").Mark(marks.Sensitive),
							}),
						}),
					},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `var.boop`,
							Statement: `is map of string with 1 element`,
						},
					},
				},
			},
		},
		"error with source code subject and unknown string expression": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
					hcl.TraverseIndex{Key: cty.StringVal("hello!")},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.UnknownVal(cty.String),
							}),
						}),
					},
				},
				Extra: diagnosticCausedByUnknown(true),
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `var.boop["hello!"]`,
							Statement: `is a string, known only after apply`,
						},
					},
				},
			},
		},
		"error with source code subject and unknown expression of unknown type": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
					hcl.TraverseIndex{Key: cty.StringVal("hello!")},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.UnknownVal(cty.DynamicPseudoType),
							}),
						}),
					},
				},
				Extra: diagnosticCausedByUnknown(true),
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `var.boop["hello!"]`,
							Statement: `will be known only after apply`,
						},
					},
				},
			},
		},
		"error with source code subject and unknown expression of unknown type when not caused by unknown values": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 9, Byte: 42},
					End:      hcl.Pos{Line: 2, Column: 26, Byte: 59},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "var"},
					hcl.TraverseAttr{Name: "boop"},
					hcl.TraverseIndex{Key: cty.StringVal("hello!")},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"boop": cty.MapVal(map[string]cty.Value{
								"hello!": cty.UnknownVal(cty.DynamicPseudoType),
							}),
						}),
					},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Wrong noises",
				Detail:   "Biological sounds are not allowed",
				Range: &DiagnosticRange{
					Filename: "test.tf",
					Start: Pos{
						Line:   2,
						Column: 9,
						Byte:   42,
					},
					End: Pos{
						Line:   2,
						Column: 26,
						Byte:   59,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`resource "test_resource" "test"`),
					Code:                 (`  foo = var.boop["hello!"]`),
					StartLine:            (2),
					HighlightStartOffset: (8),
					HighlightEndOffset:   (25),
					Values:               []DiagnosticExpressionValue{
						// The unknown value is filtered out because this is
						// not an unknown-value-related diagnostic message.
					},
				},
			},
		},
		"error with integer binary comparisons": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Test assertion failed",
				Subject: &hcl.Range{
					Filename: "test.tftest.hcl",
					Start:    hcl.Pos{Line: 7, Column: 17, Byte: 118},
					End:      hcl.Pos{Line: 7, Column: 23, Byte: 125},
				},
				Expression: &hclsyntax.BinaryOpExpr{
					LHS: &hclsyntax.LiteralValueExpr{
						Val: cty.StringVal("3"),
					},
					RHS: &hclsyntax.LiteralValueExpr{
						Val: cty.StringVal("5"),
					},
					Op: hclsyntax.OpEqual,
				},
				EvalContext: &hcl.EvalContext{},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Test assertion failed",
				Range: &DiagnosticRange{
					Filename: "test.tftest.hcl",
					Start: Pos{
						Line:   7,
						Column: 17,
						Byte:   118,
					},
					End: Pos{
						Line:   7,
						Column: 23,
						Byte:   125,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context:              strPtr(`run "fails_without_useful_diff"`),
					Code:                 ("    condition = 3 == 5"),
					StartLine:            (7),
					HighlightStartOffset: (15),
					HighlightEndOffset:   (22),
					Values:               []DiagnosticExpressionValue{},
				},
				Difference: &jsonplan.Change{
					Before:          json.RawMessage(`"3"`),
					After:           json.RawMessage(`"5"`),
					AfterUnknown:    json.RawMessage(`false`),
					AfterSensitive:  json.RawMessage(`false`),
					BeforeSensitive: json.RawMessage(`false`),
					ReplacePaths:    nil,
				},
			},
		},
		"error with object binary comparisons": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "object assertion failed",
				Subject: &hcl.Range{
					Filename: "test.tftest.hcl",
					Start:    hcl.Pos{Line: 12, Column: 17, Byte: 171},
					End:      hcl.Pos{Line: 20, Column: 6, Byte: 288},
				},
				Expression: &hclsyntax.BinaryOpExpr{
					LHS: &hclsyntax.ScopeTraversalExpr{
						Traversal: hcl.Traversal{
							hcl.TraverseRoot{Name: "var"},
							hcl.TraverseAttr{Name: "json_headers"},
						},
					},
					RHS: &hclsyntax.LiteralValueExpr{
						Val: cty.ObjectVal(map[string]cty.Value{
							"Test-Header-1": cty.StringVal("foo"),
							"Test-Header-2": cty.StringVal("bar"),
						}),
					},
					Op: hclsyntax.OpEqual,
				},
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"json_headers": cty.ObjectVal(map[string]cty.Value{
								"Test-Header-1": cty.StringVal("foo"),
								"Test-Header-2": cty.StringVal("foo"),
							}),
						}),
					},
				},
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "object assertion failed",
				Range: &DiagnosticRange{
					Filename: "test.tftest.hcl",
					Start: Pos{
						Line:   12,
						Column: 17,
						Byte:   171,
					},
					End: Pos{
						Line:   20,
						Column: 6,
						Byte:   288,
					},
				},
				Snippet: &DiagnosticSnippet{
					Context: strPtr(`run "fails_without_useful_diff"`),
					Code: (`    condition = jsonencode(var.json_headers) == jsonencode([
      "Test-Header-1: foo",
      "Test-Header-2: bar",
    ])`),
					StartLine:            (12),
					HighlightStartOffset: (0),
					HighlightEndOffset:   (117),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: "var.json_headers", Statement: "is object with 2 attributes",
						},
					},
				},
				Difference: &jsonplan.Change{
					Before:          json.RawMessage(`{"Test-Header-1":"foo","Test-Header-2":"foo"}`),
					After:           json.RawMessage(`{"Test-Header-1":"foo","Test-Header-2":"bar"}`),
					AfterUnknown:    json.RawMessage(`false`),
					AfterSensitive:  json.RawMessage(`{}`),
					BeforeSensitive: json.RawMessage(`{}`),
					ReplacePaths:    nil,
				},
			},
		},
		"error with source code subject with multiple expression values": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Catastrophic failure",
				Detail:   "Basically, everything went wrong",
				Subject: &hcl.Range{
					Filename: "values.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 13, Column: 2, Byte: 102},
				},
				Expression: hcltest.MockExprList([]hcl.Expression{
					hcltest.MockExprTraversalSrc("var.a"),
					hcltest.MockExprTraversalSrc("var.b"),
					hcltest.MockExprTraversalSrc("var.c"),
					hcltest.MockExprTraversalSrc("var.d"),
					hcltest.MockExprTraversalSrc("var.e"),
					hcltest.MockExprTraversalSrc("var.f"),
					hcltest.MockExprTraversalSrc("var.g"),
					hcltest.MockExprTraversalSrc("var.h"),
					hcltest.MockExprTraversalSrc("var.i"),
					hcltest.MockExprTraversalSrc("var.j"),
					hcltest.MockExprTraversalSrc("var.k"),
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"var": cty.ObjectVal(map[string]cty.Value{
							"a": cty.True,
							"b": cty.NumberFloatVal(123.45),
							"c": cty.NullVal(cty.String),
							"d": cty.StringVal("secret").Mark(marks.Sensitive),
							"e": cty.False,
							"f": cty.ListValEmpty(cty.String),
							"g": cty.MapVal(map[string]cty.Value{
								"boop": cty.StringVal("beep"),
							}),
							"h": cty.ListVal([]cty.Value{
								cty.StringVal("boop"),
								cty.StringVal("beep"),
								cty.StringVal("blorp"),
							}),
							"i": cty.EmptyObjectVal,
							"j": cty.ObjectVal(map[string]cty.Value{
								"foo": cty.StringVal("bar"),
							}),
							"k": cty.ObjectVal(map[string]cty.Value{
								"a": cty.True,
								"b": cty.False,
							}),
						}),
					},
				},
				Extra: diagnosticCausedBySensitive(true),
			},
			&Diagnostic{
				Severity: "error",
				Summary:  "Catastrophic failure",
				Detail:   "Basically, everything went wrong",
				Range: &DiagnosticRange{
					Filename: "values.tf",
					Start: Pos{
						Line:   1,
						Column: 1,
						Byte:   0,
					},
					End: Pos{
						Line:   13,
						Column: 2,
						Byte:   102,
					},
				},
				Snippet: &DiagnosticSnippet{
					Code: `[
  var.a,
  var.b,
  var.c,
  var.d,
  var.e,
  var.f,
  var.g,
  var.h,
  var.i,
  var.j,
  var.k,
]`,
					StartLine:            (1),
					HighlightStartOffset: (0),
					HighlightEndOffset:   (102),
					Values: []DiagnosticExpressionValue{
						{
							Traversal: `var.a`,
							Statement: `is true`,
						},
						{
							Traversal: `var.b`,
							Statement: `is 123.45`,
						},
						{
							Traversal: `var.c`,
							Statement: `is null`,
						},
						{
							Traversal: `var.d`,
							Statement: `has a sensitive value`,
						},
						{
							Traversal: `var.e`,
							Statement: `is false`,
						},
						{
							Traversal: `var.f`,
							Statement: `is empty list of string`,
						},
						{
							Traversal: `var.g`,
							Statement: `is map of string with 1 element`,
						},
						{
							Traversal: `var.h`,
							Statement: `is list of string with 3 elements`,
						},
						{
							Traversal: `var.i`,
							Statement: `is object with no attributes`,
						},
						{
							Traversal: `var.j`,
							Statement: `is object with 1 attribute "foo"`,
						},
						{
							Traversal: `var.k`,
							Statement: `is object with 2 attributes`,
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Convert the diag into a tfdiags.Diagnostic
			var diags tfdiags.Diagnostics
			diags = diags.Append(tc.diag)

			got := NewDiagnostic(diags[0], sources)
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("wrong result\n:%s", cmp.Diff(tc.want, got))
			}
		})

		t.Run(fmt.Sprintf("golden test for %s", name), func(t *testing.T) {
			// Convert the diag into a tfdiags.Diagnostic
			var diags tfdiags.Diagnostics
			diags = diags.Append(tc.diag)

			got := NewDiagnostic(diags[0], sources)

			// Render the diagnostic to indented JSON
			gotBytes, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}

			// Compare against the golden reference
			filename := path.Join(
				"testdata",
				"diagnostic",
				fmt.Sprintf("%s.json", strings.ReplaceAll(name, " ", "-")),
			)

			// Generate golden reference by uncommenting the next two lines:
			// gotBytes = append(gotBytes, '\n')
			// os.WriteFile(filename, gotBytes, 0644)

			wantFile, err := os.Open(filename)
			if err != nil {
				t.Fatalf("failed to open golden file: %s", err)
			}
			defer wantFile.Close()
			wantBytes, err := io.ReadAll(wantFile)
			if err != nil {
				t.Fatalf("failed to read output file: %s", err)
			}

			// Don't care about leading or trailing whitespace
			gotString := normaliseNewlines(strings.TrimSpace(string(gotBytes)))
			wantString := normaliseNewlines(strings.TrimSpace(string(wantBytes)))

			if !cmp.Equal(wantString, gotString) {
				t.Fatalf("wrong result\n:%s", cmp.Diff(wantString, gotString))
			}
		})
	}
}

// Function to normalise newlines in a string for Windows
func normaliseNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// Helper function to make constructing literal Diagnostics easier. There
// are fields which are pointer-to-string to ensure that the rendered JSON
// results in `null` for an empty value, rather than `""`.
func strPtr(s string) *string { return &s }

// diagnosticCausedByUnknown is a testing helper for exercising our logic
// for selectively showing unknown values alongside our source snippets for
// diagnostics that are explicitly marked as being caused by unknown values.
type diagnosticCausedByUnknown bool

var _ tfdiags.DiagnosticExtraBecauseUnknown = diagnosticCausedByUnknown(true)

func (e diagnosticCausedByUnknown) DiagnosticCausedByUnknown() bool {
	return bool(e)
}

// diagnosticCausedBySensitive is a testing helper for exercising our logic
// for selectively showing sensitive values alongside our source snippets for
// diagnostics that are explicitly marked as being caused by sensitive values.
type diagnosticCausedBySensitive bool

var _ tfdiags.DiagnosticExtraBecauseSensitive = diagnosticCausedBySensitive(true)

func (e diagnosticCausedBySensitive) DiagnosticCausedBySensitive() bool {
	return bool(e)
}
