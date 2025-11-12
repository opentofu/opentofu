// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/mitchellh/colorstring"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/command/jsonentities"
	"github.com/opentofu/opentofu/internal/lang/marks"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestDiagnostic(t *testing.T) {

	tests := map[string]struct {
		Diag interface{}
		Want string
	}{
		"sourceless error": {
			tfdiags.Sourceless(
				tfdiags.Error,
				"A sourceless error",
				"It has no source references but it does have a pretty long detail that should wrap over multiple lines.",
			),
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]A sourceless error[reset]
[red]│[reset]
[red]│[reset] It has no source references but it
[red]│[reset] does have a pretty long detail that
[red]│[reset] should wrap over multiple lines.
[red]╵[reset]
`,
		},
		"sourceless warning": {
			tfdiags.Sourceless(
				tfdiags.Warning,
				"A sourceless warning",
				"It has no source references but it does have a pretty long detail that should wrap over multiple lines.",
			),
			`[yellow]╷[reset]
[yellow]│[reset] [bold][yellow]Warning: [reset][bold]A sourceless warning[reset]
[yellow]│[reset]
[yellow]│[reset] It has no source references but it
[yellow]│[reset] does have a pretty long detail that
[yellow]│[reset] should wrap over multiple lines.
[yellow]╵[reset]
`,
		},
		"error with source code subject": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"error with source code subject and known expression": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah"),
						}),
					},
				},
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]boop.beep[reset] is "blah"
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"error with source code subject and expression referring to sensitive value": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah").Mark(marks.Sensitive),
						}),
					},
				},
				Extra: diagnosticCausedBySensitive(true),
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]boop.beep[reset] has a sensitive value
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"error with source code subject and expression referring to ephemeral value": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah").Mark(marks.Ephemeral),
						}),
					},
				},
				Extra: diagnosticCausedBySensitive(true),
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]boop.beep[reset] has an ephemeral value
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"error with source code subject and unknown string expression": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.UnknownVal(cty.String),
						}),
					},
				},
				Extra: diagnosticCausedByUnknown(true),
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]boop.beep[reset] is a string, known only after apply
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"error with source code subject and unknown expression of unknown type": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.UnknownVal(cty.DynamicPseudoType),
						}),
					},
				},
				Extra: diagnosticCausedByUnknown(true),
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]boop.beep[reset] will be known only after apply
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"error with source code subject and function call annotation": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprLiteral(cty.True),
				EvalContext: &hcl.EvalContext{
					Functions: map[string]function.Function{
						"beep": function.New(&function.Spec{
							Params: []function.Parameter{
								{
									Name: "pos_param_0",
									Type: cty.String,
								},
								{
									Name: "pos_param_1",
									Type: cty.Number,
								},
							},
							VarParam: &function.Parameter{
								Name: "var_param",
								Type: cty.Bool,
							},
						}),
					},
				},
				// This is simulating what the HCL function call expression
				// type would generate on evaluation, by implementing the
				// same interface it uses.
				Extra: fakeDiagFunctionCallExtra("beep"),
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] while calling [bold]beep[reset](pos_param_0, pos_param_1, var_param...)
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"test number assertion difference": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad testing",
				Detail:   "Number testing went wrong.",
				Expression: &hclsyntax.BinaryOpExpr{
					LHS: &hclsyntax.LiteralValueExpr{
						Val: cty.StringVal("3"),
					},
					RHS: &hclsyntax.LiteralValueExpr{
						Val: cty.StringVal("5"),
					},
					Op: hclsyntax.OpEqual,
				},
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				EvalContext: &hcl.EvalContext{},
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad testing[reset]
[red]│[reset]
[red]│[reset]   on test.tf line 1:
[red]│[reset]    1: test [underline]source[reset] code
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]Diff: [reset]
[red]│[reset]     [dark_gray]│[reset]     "3" [yellow]->[reset] "5"
[red]│[reset]
[red]│[reset] Number testing went wrong.
[red]╵[reset]
`,
		},
		"test object assertion difference": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad testing",
				Detail:   "Test object assertion!",
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
				Subject: &hcl.Range{
					Filename: "json_encode.tf",
					Start:    hcl.Pos{Line: 1, Column: 12, Byte: 12},
					End:      hcl.Pos{Line: 4, Column: 20, Byte: 150},
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
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad testing[reset]
[red]│[reset]
[red]│[reset]   on json_encode.tf line 1:
[red]│[reset]    1: condition = [underline]jsonencode(var.json_headers) == jsonencode([
[red]│[reset]    2: 			"Test-Header-1: foo",
[red]│[reset]    3: 			"Test-Header-2: bar"
[red]│[reset]    4: 		])[reset]
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]var.json_headers[reset] is object with 2 attributes
[red]│[reset]     [dark_gray]├────────────────[reset]
[red]│[reset]     [dark_gray]│[reset] [bold]Diff: [reset]
[red]│[reset]     [dark_gray]│[reset]     {
[red]│[reset]     [dark_gray]│[reset]       [yellow]~[reset] Test-Header-2 = "foo" [yellow]->[reset] "bar"
[red]│[reset]     [dark_gray]│[reset]         [dark_gray]# (1 unchanged attribute hidden)[reset]
[red]│[reset]     [dark_gray]│[reset]     }
[red]│[reset]
[red]│[reset] Test object assertion!
[red]╵[reset]
`,
		},

		// Any control characters in the summary, detail, source code snippet,
		// or source filename should be replaced by their corresponding
		// control pictures to ensure that unexpected/malicious data there
		// cannot affect the state of a terminal that stdout/stderr is
		// connected to.
		"control characters in sourceless diagnostic": {
			tfdiags.Sourceless(
				tfdiags.Error,
				"\x1b[2JOh no!",
				"\x1bHControl sequences!",
			),
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]␛[2JOh no![reset]
[red]│[reset]
[red]│[reset] ␛HControl sequences!
[red]╵[reset]
`,
		},
		"control characters in source snippet": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "controlchars\x00.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 1, Column: 7, Byte: 6},
				},
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on controlchars␀.tf line 1:
[red]│[reset]    1: [underline]before[reset]␛[0;0Hafter
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
		"control characters in unavailable source snippet": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "unavailable\x00.tf",
					Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 1, Column: 7, Byte: 6},
				},
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad bad bad[reset]
[red]│[reset]
[red]│[reset]   on unavailable␀.tf line 1:
[red]│[reset]   (source code not available)
[red]│[reset]
[red]│[reset] Whatever shall we do?
[red]╵[reset]
`,
		},
	}

	sources := map[string]*hcl.File{
		"test.tf": {Bytes: []byte(`test source code`)},
		"json_encode.tf": {Bytes: []byte(`condition = jsonencode(var.json_headers) == jsonencode([
			"Test-Header-1: foo",
			"Test-Header-2: bar"
		])`)},
		"controlchars\x00.tf": {Bytes: []byte("before\x1b[0;0Hafter")},
	}

	// This empty Colorize just passes through all of the formatting codes
	// untouched, because it doesn't define any formatting keywords.
	colorize := &colorstring.Colorize{}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var diags tfdiags.Diagnostics
			diags = diags.Append(test.Diag) // to normalize it into a tfdiag.Diagnostic
			diag := diags[0]
			got := strings.TrimSpace(Diagnostic(diag, sources, colorize, 40))
			want := strings.TrimSpace(test.Want)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("diff:\n%s", diff)
			}
		})
	}
}

func TestDiagnosticPlain(t *testing.T) {

	tests := map[string]struct {
		Diag interface{}
		Want string
	}{
		"sourceless error": {
			tfdiags.Sourceless(
				tfdiags.Error,
				"A sourceless error",
				"It has no source references but it does have a pretty long detail that should wrap over multiple lines.",
			),
			`
Error: A sourceless error

It has no source references but it does
have a pretty long detail that should
wrap over multiple lines.
`,
		},
		"sourceless warning": {
			tfdiags.Sourceless(
				tfdiags.Warning,
				"A sourceless warning",
				"It has no source references but it does have a pretty long detail that should wrap over multiple lines.",
			),
			`
Warning: A sourceless warning

It has no source references but it does
have a pretty long detail that should
wrap over multiple lines.
`,
		},
		"error with source code subject": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code

Whatever shall we do?
`,
		},
		"error with source code subject and known expression": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah"),
						}),
					},
				},
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code
    ├────────────────
    │ boop.beep is "blah"

Whatever shall we do?
`,
		},
		"error with source code subject and expression referring to sensitive value": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah").Mark(marks.Sensitive),
						}),
					},
				},
				Extra: diagnosticCausedBySensitive(true),
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code
    ├────────────────
    │ boop.beep has a sensitive value

Whatever shall we do?
`,
		},
		"error with source code subject and expression referring to sensitive value when not related to sensitivity": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah").Mark(marks.Sensitive),
						}),
					},
				},
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code

Whatever shall we do?
`,
		},
		"error with source code subject and expression referring to ephemeral value": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah").Mark(marks.Ephemeral),
						}),
					},
				},
				Extra: diagnosticCausedBySensitive(true),
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code
    ├────────────────
    │ boop.beep has an ephemeral value

Whatever shall we do?
`,
		},
		"error with source code subject and expression referring to ephemeral value when not related to sensitivity": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.StringVal("blah").Mark(marks.Ephemeral),
						}),
					},
				},
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code

Whatever shall we do?
`,
		},
		"error with source code subject and unknown string expression": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.UnknownVal(cty.String),
						}),
					},
				},
				Extra: diagnosticCausedByUnknown(true),
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code
    ├────────────────
    │ boop.beep is a string, known only after apply

Whatever shall we do?
`,
		},
		"error with source code subject and unknown string expression when problem isn't unknown-related": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.UnknownVal(cty.String),
						}),
					},
				},
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code
    ├────────────────
    │ boop.beep is a string

Whatever shall we do?
`,
		},
		"error with source code subject and unknown expression of unknown type": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.UnknownVal(cty.DynamicPseudoType),
						}),
					},
				},
				Extra: diagnosticCausedByUnknown(true),
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code
    ├────────────────
    │ boop.beep will be known only after apply

Whatever shall we do?
`,
		},
		"error with source code subject and unknown expression of unknown type when problem isn't unknown-related": {
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Bad bad bad",
				Detail:   "Whatever shall we do?",
				Subject: &hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 6, Byte: 5},
					End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
				},
				Expression: hcltest.MockExprTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "boop"},
					hcl.TraverseAttr{Name: "beep"},
				}),
				EvalContext: &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"boop": cty.ObjectVal(map[string]cty.Value{
							"beep": cty.UnknownVal(cty.DynamicPseudoType),
						}),
					},
				},
			},
			`
Error: Bad bad bad

  on test.tf line 1:
   1: test source code

Whatever shall we do?
`,
		},
	}

	sources := map[string]*hcl.File{
		"test.tf": {Bytes: []byte(`test source code`)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var diags tfdiags.Diagnostics
			diags = diags.Append(test.Diag) // to normalize it into a tfdiag.Diagnostic
			diag := diags[0]
			got := strings.TrimSpace(DiagnosticPlain(diag, sources, 40))
			want := strings.TrimSpace(test.Want)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("diff:\n%s", diff)
			}
		})
	}
}

func TestDiagnosticWarningsCompact(t *testing.T) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.SimpleWarning("foo"))
	diags = diags.Append(tfdiags.SimpleWarning("foo"))
	diags = diags.Append(tfdiags.SimpleWarning("bar"))
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "source foo",
		Detail:   "...",
		Subject: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 2, Column: 1, Byte: 5},
			End:      hcl.Pos{Line: 2, Column: 1, Byte: 5},
		},
	})
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "source foo",
		Detail:   "...",
		Subject: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 3, Column: 1, Byte: 7},
			End:      hcl.Pos{Line: 3, Column: 1, Byte: 7},
		},
	})
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "source bar",
		Detail:   "...",
		Subject: &hcl.Range{
			Filename: "source2.tf",
			Start:    hcl.Pos{Line: 1, Column: 1, Byte: 1},
			End:      hcl.Pos{Line: 1, Column: 1, Byte: 1},
		},
	})

	// ConsolidateWarnings groups together the ones
	// that have source location information and that
	// have the same summary text.
	diags = diags.Consolidate(1, tfdiags.Warning)

	// A zero-value Colorize just passes all the formatting
	// codes back to us, so we can test them literally.
	got := DiagnosticWarningsCompact(diags, &colorstring.Colorize{})
	want := `[bold][yellow]Warnings:[reset]

- foo
- foo
- bar
- source foo
  on source.tf line 2 (and 1 more)
- source bar
  on source2.tf line 1
`
	if got != want {
		t.Errorf(
			"wrong result\ngot:\n%s\n\nwant:\n%s\n\ndiff:\n%s",
			got, want, cmp.Diff(want, got),
		)
	}
}

// Test case via https://github.com/hashicorp/terraform/issues/21359
func TestDiagnostic_nonOverlappingHighlightContext(t *testing.T) {
	var diags tfdiags.Diagnostics

	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Some error",
		Detail:   "...",
		Subject: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 1, Column: 5, Byte: 5},
			End:      hcl.Pos{Line: 1, Column: 5, Byte: 5},
		},
		Context: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 1, Column: 5, Byte: 5},
			End:      hcl.Pos{Line: 4, Column: 2, Byte: 60},
		},
	})
	sources := map[string]*hcl.File{
		"source.tf": {Bytes: []byte(`x = somefunc("testing", {
  alpha = "foo"
  beta  = "bar"
})
`)},
	}
	color := &colorstring.Colorize{
		Colors:  colorstring.DefaultColors,
		Reset:   true,
		Disable: true,
	}
	expected := `╷
│ Error: Some error
│
│   on source.tf line 1:
│    1: x = somefunc("testing", {
│    2:   alpha = "foo"
│    3:   beta  = "bar"
│    4: })
│
│ ...
╵
`
	output := Diagnostic(diags[0], sources, color, 80)

	if diff := cmp.Diff(output, expected); diff != "" {
		t.Errorf("diff:\n%s", diff)
	}
}

func TestDiagnostic_emptyOverlapHighlightContext(t *testing.T) {
	var diags tfdiags.Diagnostics

	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Some error",
		Detail:   "...",
		Subject: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 3, Column: 10, Byte: 38},
			End:      hcl.Pos{Line: 4, Column: 1, Byte: 39},
		},
		Context: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 2, Column: 13, Byte: 27},
			End:      hcl.Pos{Line: 4, Column: 1, Byte: 39},
		},
	})
	sources := map[string]*hcl.File{
		"source.tf": {Bytes: []byte(`variable "x" {
  default = {
    "foo"
  }
`)},
	}
	color := &colorstring.Colorize{
		Colors:  colorstring.DefaultColors,
		Reset:   true,
		Disable: true,
	}
	expected := `╷
│ Error: Some error
│
│   on source.tf line 3, in variable "x":
│    2:   default = {
│    3:     "foo"
│    4:   }
│
│ ...
╵
`
	output := Diagnostic(diags[0], sources, color, 80)

	if diff := cmp.Diff(output, expected); diff != "" {
		t.Errorf("diff:\n%s", diff)
	}
}

func TestDiagnosticPlain_emptyOverlapHighlightContext(t *testing.T) {
	var diags tfdiags.Diagnostics

	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Some error",
		Detail:   "...",
		Subject: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 3, Column: 10, Byte: 38},
			End:      hcl.Pos{Line: 4, Column: 1, Byte: 39},
		},
		Context: &hcl.Range{
			Filename: "source.tf",
			Start:    hcl.Pos{Line: 2, Column: 13, Byte: 27},
			End:      hcl.Pos{Line: 4, Column: 1, Byte: 39},
		},
	})
	sources := map[string]*hcl.File{
		"source.tf": {Bytes: []byte(`variable "x" {
  default = {
    "foo"
  }
`)},
	}

	expected := `
Error: Some error

  on source.tf line 3, in variable "x":
   2:   default = {
   3:     "foo"
   4:   }

...
`
	output := DiagnosticPlain(diags[0], sources, 80)

	if diff := cmp.Diff(output, expected); diff != "" {
		t.Errorf("diff:\n%s", diff)
	}
}

func TestDiagnostic_wrapDetailIncludingCommand(t *testing.T) {
	var diags tfdiags.Diagnostics

	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Everything went wrong",
		Detail:   "This is a very long sentence about whatever went wrong which is supposed to wrap onto multiple lines. Thank-you very much for listening.\n\nTo fix this, run this very long command:\n  terraform read-my-mind -please -thanks -but-do-not-wrap-this-line-because-it-is-prefixed-with-spaces\n\nHere is a coda which is also long enough to wrap and so it should eventually make it onto multiple lines. THE END",
	})
	color := &colorstring.Colorize{
		Colors:  colorstring.DefaultColors,
		Reset:   true,
		Disable: true,
	}
	expected := `╷
│ Error: Everything went wrong
│
│ This is a very long sentence about whatever went wrong which is supposed
│ to wrap onto multiple lines. Thank-you very much for listening.
│
│ To fix this, run this very long command:
│   terraform read-my-mind -please -thanks -but-do-not-wrap-this-line-because-it-is-prefixed-with-spaces
│
│ Here is a coda which is also long enough to wrap and so it should
│ eventually make it onto multiple lines. THE END
╵
`
	output := Diagnostic(diags[0], nil, color, 76)

	if diff := cmp.Diff(output, expected); diff != "" {
		t.Errorf("diff:\n%s", diff)
	}
}

func TestDiagnosticPlain_wrapDetailIncludingCommand(t *testing.T) {
	var diags tfdiags.Diagnostics

	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Everything went wrong",
		Detail:   "This is a very long sentence about whatever went wrong which is supposed to wrap onto multiple lines. Thank-you very much for listening.\n\nTo fix this, run this very long command:\n  terraform read-my-mind -please -thanks -but-do-not-wrap-this-line-because-it-is-prefixed-with-spaces\n\nHere is a coda which is also long enough to wrap and so it should eventually make it onto multiple lines. THE END",
	})

	expected := `
Error: Everything went wrong

This is a very long sentence about whatever went wrong which is supposed to
wrap onto multiple lines. Thank-you very much for listening.

To fix this, run this very long command:
  terraform read-my-mind -please -thanks -but-do-not-wrap-this-line-because-it-is-prefixed-with-spaces

Here is a coda which is also long enough to wrap and so it should
eventually make it onto multiple lines. THE END
`
	output := DiagnosticPlain(diags[0], nil, 76)

	if diff := cmp.Diff(output, expected); diff != "" {
		t.Errorf("diff:\n%s", diff)
	}
}

// Test cases covering invalid JSON diagnostics which should still render
// correctly. These JSON diagnostic values cannot be generated from the
// json.NewDiagnostic code path, but we may read and display JSON diagnostics
// in future from other sources.
func TestDiagnosticFromJSON_invalid(t *testing.T) {
	tests := map[string]struct {
		Diag *jsonentities.Diagnostic
		Want string
	}{
		"zero-value end range and highlight end byte": {
			&jsonentities.Diagnostic{
				Severity: jsonentities.DiagnosticSeverityError,
				Summary:  "Bad end",
				Detail:   "It all went wrong.",
				Range: &jsonentities.DiagnosticRange{
					Filename: "ohno.tf",
					Start:    jsonentities.Pos{Line: 1, Column: 23, Byte: 22},
					End:      jsonentities.Pos{Line: 0, Column: 0, Byte: 0},
				},
				Snippet: &jsonentities.DiagnosticSnippet{
					Code:                 `resource "foo_bar "baz" {`,
					StartLine:            1,
					HighlightStartOffset: 22,
					HighlightEndOffset:   0,
				},
			},
			`[red]╷[reset]
[red]│[reset] [bold][red]Error: [reset][bold]Bad end[reset]
[red]│[reset]
[red]│[reset]   on ohno.tf line 1:
[red]│[reset]    1: resource "foo_bar "baz[underline]"[reset] {
[red]│[reset]
[red]│[reset] It all went wrong.
[red]╵[reset]
`,
		},
	}

	// This empty Colorize just passes through all of the formatting codes
	// untouched, because it doesn't define any formatting keywords.
	colorize := &colorstring.Colorize{}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := strings.TrimSpace(DiagnosticFromJSON(test.Diag, colorize, 40))
			want := strings.TrimSpace(test.Want)
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("wrong result\n%s", diff)
			}
		})
	}
}

// fakeDiagFunctionCallExtra is a fake implementation of the interface that
// HCL uses to provide "extra information" associated with diagnostics that
// describe errors during a function call.
type fakeDiagFunctionCallExtra string

var _ hclsyntax.FunctionCallDiagExtra = fakeDiagFunctionCallExtra("")

func (e fakeDiagFunctionCallExtra) CalledFunctionName() string {
	return string(e)
}

func (e fakeDiagFunctionCallExtra) FunctionCallError() error {
	return nil
}

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

var _ tfdiags.DiagnosticExtraBecauseConfidentialValues = diagnosticCausedBySensitive(true)

func (e diagnosticCausedBySensitive) DiagnosticCausedByConfidentialValues() bool {
	return bool(e)
}
