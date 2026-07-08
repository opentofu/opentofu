// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/linting"
)

func TestBuild(t *testing.T) {
	type diagFlat struct {
		Severity Severity
		Summary  string
		Detail   string
		Subject  *SourceRange
		Context  *SourceRange
	}

	tests := map[string]struct {
		Cons func(Diagnostics) Diagnostics
		Want []diagFlat
	}{
		"nil": {
			func(diags Diagnostics) Diagnostics {
				return diags
			},
			nil,
		},
		"fmt.Errorf": {
			func(diags Diagnostics) Diagnostics {
				diags = diags.Append(fmt.Errorf("oh no bad"))
				return diags
			},
			[]diagFlat{
				{
					Severity: Error,
					Summary:  "oh no bad",
				},
			},
		},
		"errors.New": {
			func(diags Diagnostics) Diagnostics {
				diags = diags.Append(errors.New("oh no bad"))
				return diags
			},
			[]diagFlat{
				{
					Severity: Error,
					Summary:  "oh no bad",
				},
			},
		},
		"hcl.Diagnostic": {
			func(diags Diagnostics) Diagnostics {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Something bad happened",
					Detail:   "It was really, really bad.",
					Subject: &hcl.Range{
						Filename: "foo.tf",
						Start:    hcl.Pos{Line: 1, Column: 10, Byte: 9},
						End:      hcl.Pos{Line: 2, Column: 3, Byte: 25},
					},
					Context: &hcl.Range{
						Filename: "foo.tf",
						Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
						End:      hcl.Pos{Line: 3, Column: 1, Byte: 30},
					},
				})
				return diags
			},
			[]diagFlat{
				{
					Severity: Error,
					Summary:  "Something bad happened",
					Detail:   "It was really, really bad.",
					Subject: &SourceRange{
						Filename: "foo.tf",
						Start:    SourcePos{Line: 1, Column: 10, Byte: 9},
						End:      SourcePos{Line: 2, Column: 3, Byte: 25},
					},
					Context: &SourceRange{
						Filename: "foo.tf",
						Start:    SourcePos{Line: 1, Column: 1, Byte: 0},
						End:      SourcePos{Line: 3, Column: 1, Byte: 30},
					},
				},
			},
		},
		"hcl.Diagnostics": {
			func(diags Diagnostics) Diagnostics {
				diags = diags.Append(hcl.Diagnostics{
					{
						Severity: hcl.DiagError,
						Summary:  "Something bad happened",
						Detail:   "It was really, really bad.",
					},
					{
						Severity: hcl.DiagWarning,
						Summary:  "Also, somebody sneezed",
						Detail:   "How rude!",
					},
				})
				return diags
			},
			[]diagFlat{
				{
					Severity: Error,
					Summary:  "Something bad happened",
					Detail:   "It was really, really bad.",
				},
				{
					Severity: Warning,
					Summary:  "Also, somebody sneezed",
					Detail:   "How rude!",
				},
			},
		},
		"multierror.Error": {
			func(diags Diagnostics) Diagnostics {
				err := multierror.Append(nil, errors.New("bad thing A"))
				err = multierror.Append(err, errors.New("bad thing B"))
				diags = diags.Append(err)
				return diags
			},
			[]diagFlat{
				{
					Severity: Error,
					Summary:  "bad thing A",
				},
				{
					Severity: Error,
					Summary:  "bad thing B",
				},
			},
		},
		"concat Diagnostics": {
			func(diags Diagnostics) Diagnostics {
				var moreDiags Diagnostics
				moreDiags = moreDiags.Append(errors.New("bad thing A"))
				moreDiags = moreDiags.Append(errors.New("bad thing B"))
				return diags.Append(moreDiags)
			},
			[]diagFlat{
				{
					Severity: Error,
					Summary:  "bad thing A",
				},
				{
					Severity: Error,
					Summary:  "bad thing B",
				},
			},
		},
		"single Diagnostic": {
			func(diags Diagnostics) Diagnostics {
				return diags.Append(SimpleWarning("Don't forget your toothbrush!"))
			},
			[]diagFlat{
				{
					Severity: Warning,
					Summary:  "Don't forget your toothbrush!",
				},
			},
		},
		"multiple appends": {
			func(diags Diagnostics) Diagnostics {
				diags = diags.Append(SimpleWarning("Don't forget your toothbrush!"))
				diags = diags.Append(fmt.Errorf("exploded"))
				return diags
			},
			[]diagFlat{
				{
					Severity: Warning,
					Summary:  "Don't forget your toothbrush!",
				},
				{
					Severity: Error,
					Summary:  "exploded",
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotDiags := test.Cons(nil)
			var got []diagFlat
			for _, item := range gotDiags {
				desc := item.Description()
				source := item.Source()
				got = append(got, diagFlat{
					Severity: item.Severity(),
					Summary:  desc.Summary,
					Detail:   desc.Detail,
					Subject:  source.Subject,
					Context:  source.Context,
				})
			}

			if !reflect.DeepEqual(got, test.Want) {
				t.Errorf("wrong result\ngot: %swant: %s", spew.Sdump(got), spew.Sdump(test.Want))
			}
		})
	}
}

func TestDiagnosticsErr(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var diags Diagnostics
		err := diags.Err()
		if err != nil {
			t.Errorf("got non-nil error %#v; want nil", err)
		}
	})
	t.Run("warning only", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(SimpleWarning("bad"))
		err := diags.Err()
		if err != nil {
			t.Errorf("got non-nil error %#v; want nil", err)
		}
	})
	t.Run("one error", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		err := diags.Err()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		if got, want := err.Error(), "didn't work"; got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("two errors", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		diags = diags.Append(errors.New("didn't work either"))
		err := diags.Err()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		want := strings.TrimSpace(`
2 problems:

- didn't work
- didn't work either
`)
		if got := err.Error(); got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("error and warning", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		diags = diags.Append(SimpleWarning("didn't work either"))
		err := diags.Err()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		// Since this "as error" mode is just a fallback for
		// non-diagnostics-aware situations like tests, we don't actually
		// distinguish warnings and errors here since the point is to just
		// get the messages rendered. User-facing code should be printing
		// each diagnostic separately, so won't enter this codepath,
		want := strings.TrimSpace(`
2 problems:

- didn't work
- didn't work either
`)
		if got := err.Error(); got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestDiagnosticsErrWithWarnings(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var diags Diagnostics
		err := diags.ErrWithWarnings()
		if err != nil {
			t.Errorf("got non-nil error %#v; want nil", err)
		}
	})
	t.Run("warning only", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(SimpleWarning("bad"))
		err := diags.ErrWithWarnings()
		if err == nil {
			t.Errorf("got nil error; want NonFatalError")
			return
		}
		if _, ok := err.(NonFatalError); !ok {
			t.Errorf("got %T; want NonFatalError", err)
		}
	})
	t.Run("one error", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		err := diags.ErrWithWarnings()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		if got, want := err.Error(), "didn't work"; got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("two errors", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		diags = diags.Append(errors.New("didn't work either"))
		err := diags.ErrWithWarnings()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		want := strings.TrimSpace(`
2 problems:

- didn't work
- didn't work either
`)
		if got := err.Error(); got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("error and warning", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		diags = diags.Append(SimpleWarning("didn't work either"))
		err := diags.ErrWithWarnings()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		// Since this "as error" mode is just a fallback for
		// non-diagnostics-aware situations like tests, we don't actually
		// distinguish warnings and errors here since the point is to just
		// get the messages rendered. User-facing code should be printing
		// each diagnostic separately, so won't enter this codepath,
		want := strings.TrimSpace(`
2 problems:

- didn't work
- didn't work either
`)
		if got := err.Error(); got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestDiagnosticsNonFatalErr(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var diags Diagnostics
		err := diags.NonFatalErr()
		if err != nil {
			t.Errorf("got non-nil error %#v; want nil", err)
		}
	})
	t.Run("warning only", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(SimpleWarning("bad"))
		err := diags.NonFatalErr()
		if err == nil {
			t.Errorf("got nil error; want NonFatalError")
			return
		}
		if _, ok := err.(NonFatalError); !ok {
			t.Errorf("got %T; want NonFatalError", err)
		}
	})
	t.Run("one error", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		err := diags.NonFatalErr()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		if got, want := err.Error(), "didn't work"; got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
		if _, ok := err.(NonFatalError); !ok {
			t.Errorf("got %T; want NonFatalError", err)
		}
	})
	t.Run("two errors", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		diags = diags.Append(errors.New("didn't work either"))
		err := diags.NonFatalErr()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		want := strings.TrimSpace(`
2 problems:

- didn't work
- didn't work either
`)
		if got := err.Error(); got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
		if _, ok := err.(NonFatalError); !ok {
			t.Errorf("got %T; want NonFatalError", err)
		}
	})
	t.Run("error and warning", func(t *testing.T) {
		var diags Diagnostics
		diags = diags.Append(errors.New("didn't work"))
		diags = diags.Append(SimpleWarning("didn't work either"))
		err := diags.NonFatalErr()
		if err == nil {
			t.Fatalf("got nil error %#v; want non-nil", err)
		}
		// Since this "as error" mode is just a fallback for
		// non-diagnostics-aware situations like tests, we don't actually
		// distinguish warnings and errors here since the point is to just
		// get the messages rendered. User-facing code should be printing
		// each diagnostic separately, so won't enter this codepath,
		want := strings.TrimSpace(`
2 problems:

- didn't work
- didn't work either
`)
		if got := err.Error(); got != want {
			t.Errorf("wrong error message\ngot:  %s\nwant: %s", got, want)
		}
		if _, ok := err.(NonFatalError); !ok {
			t.Errorf("got %T; want NonFatalError", err)
		}
	})
}

func TestDiagnosticSorting(t *testing.T) {
	// with no source information
	warn1WithNoSource := &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Warning 1",
	}
	warn2WithNoSource := &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Warning 2",
	}
	error1WithNoSource := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Error 1",
	}
	error2WithNoSource := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Error 2",
	}
	lint1WithNoSource := LintMessage(linting.MustParseRuleAddr("foo"), nil, "Linting 1", "", nil, nil)
	lint2WithNoSource := LintMessage(linting.MustParseRuleAddr("foo"), nil, "Linting 1", "", nil, nil)

	// with source information
	warn1 := &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Warning 1",
		Subject:  &hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 20, Column: 1}},
	}
	warn2 := &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Warning 2",
		Subject:  &hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 2, Byte: 10, Column: 1}},
	}
	error1 := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Error 1",
		Subject:  &hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 3, Byte: 40, Column: 1}},
	}
	error2 := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Error 2",
		Subject:  &hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 4, Byte: 30, Column: 1}},
	}
	lint1 := LintMessage(linting.MustParseRuleAddr("foo"), nil, "Linting 1", "", &SourceRange{Filename: "test.tf", Start: SourcePos{Line: 4, Column: 10, Byte: 40}}, nil)
	lint2 := LintMessage(linting.MustParseRuleAddr("foo"), nil, "Linting 2", "", &SourceRange{Filename: "test.tf", Start: SourcePos{Line: 3, Column: 10, Byte: 30}}, nil)

	cases := map[string]struct {
		given Diagnostics
		want  Diagnostics
	}{
		"no sources, only severity": {
			given: New(warn1WithNoSource, error1WithNoSource, lint1WithNoSource, warn2WithNoSource, error2WithNoSource, lint2WithNoSource),
			want:  New(lint1WithNoSource, lint2WithNoSource, warn1WithNoSource, warn2WithNoSource, error1WithNoSource, error2WithNoSource),
		},
		"with sources": {
			given: New(warn1, error1, lint1, warn2, error2, lint2),
			want:  New(lint2, lint1, warn2, warn1, error2, error1),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.given.Sort()
			compareDiagnostics(t, tc.want, tc.given)
		})
	}
}

func compareDiagnostics(t *testing.T, want, got Diagnostics) {
	if len(want) != len(got) {
		t.Fatalf("cannot compare the diagnostic slices. want len = %d; got len = %d", len(want), len(got))
	}

	for i := range want {
		compareDiagnostic(t, i, want[i], got[i])
	}
}

func compareDiagnostic(t *testing.T, i int, want, got Diagnostic) {
	var prefix string
	if i >= 0 {
		prefix = fmt.Sprintf("[idx %d] ", i)
	}
	if wv, gv := want.Severity(), got.Severity(); wv != gv {
		t.Errorf("%sinvalid severity. want: %q but got %q", prefix, wv.String(), gv.String())
	}
	if wv, gv := want.Description().Summary, got.Description().Summary; wv != gv {
		t.Errorf("%sinvalid summary. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Description().Detail, got.Description().Detail; wv != gv {
		t.Errorf("%sinvalid detail. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Description().Address, got.Description().Address; wv != gv {
		t.Errorf("%sinvalid address. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Source(), got.Source(); !wv.Equal(gv) {
		t.Errorf("%sinvalid source. want: %+v but got %+v", prefix, wv, gv)
	}
	wei, gei := want.ExtraInfo(), got.ExtraInfo()
	if diff := cmp.Diff(wei, gei); diff != "" {
		t.Errorf("%sinvalid extra info (-want,+got):\n%s", prefix, diff)
	}
	wexpr, gexpr := want.FromExpr(), got.FromExpr()
	if diff := cmp.Diff(wexpr, gexpr); diff != "" {
		t.Errorf("%sinvalid fromExpr (-want,+got):\n%s", prefix, diff)
	}
}
