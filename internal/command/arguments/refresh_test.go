// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"
	"os"

	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/hashicorp/hcl/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
)

func TestParseRefresh_basicValid(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Refresh
	}{
		"defaults": {
			nil,
			&Refresh{
				InputEnabled: true,
				ViewType:     ViewHuman,
			},
		},
		"input=false": {
			[]string{"-input=false"},
			&Refresh{
				InputEnabled: false,
				ViewType:     ViewHuman,
			},
		},
		"JSON view disables input": {
			[]string{"-json"},
			&Refresh{
				InputEnabled: false,
				ViewType:     ViewJSON,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParseRefresh(tc.args)
			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			// Ignore the extended arguments for simplicity
			got.State = nil
			got.Operation = nil
			got.Vars = nil
			if *got != *tc.want {
				t.Fatalf("unexpected result\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}

func TestParseRefresh_invalid(t *testing.T) {
	got, diags := ParseRefresh([]string{"-frob"})
	if len(diags) == 0 {
		t.Fatal("expected diags but got none")
	}
	if got, want := diags.Err().Error(), "flag provided but not defined"; !strings.Contains(got, want) {
		t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
	}
	if got.ViewType != ViewHuman {
		t.Fatalf("wrong view type, got %#v, want %#v", got.ViewType, ViewHuman)
	}
}

func TestParseRefresh_tooManyArguments(t *testing.T) {
	got, diags := ParseRefresh([]string{"saved.tfplan"})
	if len(diags) == 0 {
		t.Fatal("expected diags but got none")
	}
	if got, want := diags.Err().Error(), "Too many command line arguments"; !strings.Contains(got, want) {
		t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
	}
	if got.ViewType != ViewHuman {
		t.Fatalf("wrong view type, got %#v, want %#v", got.ViewType, ViewHuman)
	}
}

func TestParseRefresh_targets(t *testing.T) {
	foobarbaz, _ := addrs.ParseTargetStr("foo_bar.baz")
	boop, _ := addrs.ParseTargetStr("module.boop")
	testCases := map[string]struct {
		args    []string
		want    []addrs.Targetable
		wantErr string
	}{
		"no targets by default": {
			args: nil,
			want: nil,
		},
		"one target": {
			args: []string{"-target=foo_bar.baz"},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"two targets": {
			args: []string{"-target=foo_bar.baz", "-target", "module.boop"},
			want: []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"invalid traversal": {
			args:    []string{"-target=foo."},
			want:    nil,
			wantErr: "Dot must be followed by attribute name",
		},
		"invalid target": {
			args:    []string{"-target=data[0].foo"},
			want:    nil,
			wantErr: "A data source name is required",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParseRefresh(tc.args)
			if tc.wantErr == "" && len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			} else if tc.wantErr != "" {
				if len(diags) == 0 {
					t.Fatalf("expected diags but got none")
				} else if got := diags.Err().Error(); !strings.Contains(got, tc.wantErr) {
					t.Fatalf("wrong diags\n got: %s\nwant: %s", got, tc.wantErr)
				}
			}

			if !cmp.Equal(got.Operation.Targets, tc.want) {
				t.Fatalf("unexpected result\n%s", cmp.Diff(got.Operation.Targets, tc.want))
			}
		})
	}
}

func TestParseRefresh_excludes(t *testing.T) {
	foobarbaz, _ := addrs.ParseTargetStr("foo_bar.baz")
	boop, _ := addrs.ParseTargetStr("module.boop")
	testCases := map[string]struct {
		args    []string
		want    []addrs.Targetable
		wantErr string
	}{
		"no excludes by default": {
			args: nil,
			want: nil,
		},
		"one exclude": {
			args: []string{"-exclude=foo_bar.baz"},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"two excludes": {
			args: []string{"-exclude=foo_bar.baz", "-exclude", "module.boop"},
			want: []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"invalid traversal": {
			args:    []string{"-exclude=foo."},
			want:    nil,
			wantErr: "Dot must be followed by attribute name",
		},
		"invalid target": {
			args:    []string{"-exclude=data[0].foo"},
			want:    nil,
			wantErr: "A data source name is required",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParseRefresh(tc.args)
			if tc.wantErr == "" && len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			} else if tc.wantErr != "" {
				if len(diags) == 0 {
					t.Fatalf("expected diags but got none")
				} else if got := diags.Err().Error(); !strings.Contains(got, tc.wantErr) {
					t.Fatalf("wrong diags\n got: %s\nwant: %s", got, tc.wantErr)
				}
			}
			if !cmp.Equal(got.Operation.Excludes, tc.want) {
				t.Fatalf("unexpected result\n%s", cmp.Diff(got.Operation.Excludes, tc.want))
			}
		})
	}
}

func TestParseRefresh_excludeAndTarget(t *testing.T) {
	got, gotDiags := ParseRefresh([]string{"-exclude=foo_bar.baz", "-target=foo_bar.bar"})
	if len(gotDiags) == 0 {
		t.Fatalf("expected error, but there was none")
	}

	wantDiags := tfdiags.Diagnostics{
		tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid combination of arguments",
			"Cannot combine both target and exclude flags. Please only target or exclude resources",
		),
	}
	if diff := cmp.Diff(wantDiags.ForRPC(), gotDiags.ForRPC()); diff != "" {
		t.Errorf("wrong diagnostics\n%s", diff)
	}
	if len(got.Operation.Targets) > 0 {
		t.Errorf("Did not expect operation to parse targets, but it parsed %d targets", len(got.Operation.Targets))
	}
	if len(got.Operation.Excludes) > 0 {
		t.Errorf("Did not expect operation to parse excludes, but it parsed %d targets", len(got.Operation.Excludes))
	}
}

func ParseRefresh_targetFile(t *testing.T) {
	foobarbaz, _ := addrs.ParseTargetStr("foo_bar.baz")
	boop, _ := addrs.ParseTargetStr("module.boop")
	wow, _ := addrs.ParseTargetStr("wow.ham")
	testCases := map[string]struct {
		fileContent string
		want        []addrs.Targetable
		wantDiags   tfdiags.Diagnostics
	}{
		"target file no targets": {
			fileContent: "",
			want:        nil,
		},
		"target file valid single target": {
			fileContent: "foo_bar.baz",
			want:        []addrs.Targetable{foobarbaz.Subject},
		},
		"target file valid multiple targets": {
			fileContent: "foo_bar.baz\nmodule.boop",
			want:        []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"target file invalid target": {
			fileContent: "foo.",
			want:        nil,
			wantDiags: tfdiags.Diagnostics(nil).Append(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid syntax",
					Detail:   `For target "foo.": Dot must be followed by attribute name.`,
					Subject: &hcl.Range{
						Start: hcl.Pos{Line: 1, Column: 1},
						End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
					},
				},
			),
		},
		"target file valid comment": {
			fileContent: "#foo_bar.baz",
			want:        nil,
		},
		"target file valid spaces": {
			fileContent: "   foo_bar.baz",
			want:        []addrs.Targetable{foobarbaz.Subject},
		},
		"target file valid tab": {
			fileContent: "\tfoo_bar.baz",
			want:        []addrs.Targetable{foobarbaz.Subject},
		},
		"target file complicated": {
			fileContent: "\tmodule.boop\n#foo_bar.baz\nwow.ham",
			want:        []addrs.Targetable{boop.Subject, wow.Subject},
		},
		"target file invalid bracket with spaces": {
			fileContent: `    [boop]`,
			want:        nil,
			wantDiags: tfdiags.Diagnostics(nil).Append(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid syntax",
					Detail:   `For target "    [boop]": Must begin with a variable name.`,
					Subject: &hcl.Range{
						Start: hcl.Pos{Line: 1, Column: 1},
						End:   hcl.Pos{Line: 1, Column: 11, Byte: 10},
					},
				},
			),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			file := tempFileWriter(tc.fileContent)
			defer os.Remove(file.Name())

			got, gotDiags := ParsePlan([]string{"-target-file=" + file.Name()})
			if len(tc.wantDiags) != 0 || len(gotDiags) != 0 {
				if len(gotDiags) == 0 {
					t.Fatalf("expected diags but got none")
				}
				if len(tc.wantDiags) == 0 {
					t.Fatalf("got diags but didn't want any: %v", gotDiags.ErrWithWarnings())
				}
				gotDiagsExported := gotDiags.ForRPC()
				wantDiagsExported := tc.wantDiags.ForRPC()
				wantDiagsExported[0].Source().Subject.Filename = file.Name()
				gotDiagsExported.Sort()
				wantDiagsExported.Sort()
				if diff := cmp.Diff(wantDiagsExported, gotDiagsExported); diff != "" {
					t.Error("wrong diagnostics\n" + diff)
				}
			}
			if !cmp.Equal(got.Operation.Targets, tc.want) {
				t.Fatalf("unexpected result\n%s", cmp.Diff(got.Operation.Targets, tc.want))
			}
		})
	}
}


func TestParseRefresh_vars(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want []FlagNameValue
	}{
		"no var flags by default": {
			args: nil,
			want: nil,
		},
		"one var": {
			args: []string{"-var", "foo=bar"},
			want: []FlagNameValue{
				{Name: "-var", Value: "foo=bar"},
			},
		},
		"one var-file": {
			args: []string{"-var-file", "cool.tfvars"},
			want: []FlagNameValue{
				{Name: "-var-file", Value: "cool.tfvars"},
			},
		},
		"ordering preserved": {
			args: []string{
				"-var", "foo=bar",
				"-var-file", "cool.tfvars",
				"-var", "boop=beep",
			},
			want: []FlagNameValue{
				{Name: "-var", Value: "foo=bar"},
				{Name: "-var-file", Value: "cool.tfvars"},
				{Name: "-var", Value: "boop=beep"},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParseRefresh(tc.args)
			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if vars := got.Vars.All(); !cmp.Equal(vars, tc.want) {
				t.Fatalf("unexpected result\n%s", cmp.Diff(vars, tc.want))
			}
			if got, want := got.Vars.Empty(), len(tc.want) == 0; got != want {
				t.Fatalf("expected Empty() to return %t, but was %t", want, got)
			}
		})
	}
}
