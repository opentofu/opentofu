// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

//	type testFileBackup struct {
//		filePath    string
//		fileContent string
//		hasError    bool
//	}
type testFile struct {
	filePath    string
	fileContent string
	diags       hcl.Diagnostics
}

func TestParsePlan_basicValid(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Plan
	}{
		"defaults": {
			nil,
			&Plan{
				DetailedExitCode: false,
				InputEnabled:     true,
				OutPath:          "",
				ViewType:         ViewHuman,
				State:            &State{Lock: true},
				Vars:             &Vars{},
				Operation: &Operation{
					PlanMode:    plans.NormalMode,
					Parallelism: 10,
					Refresh:     true,
				},
			},
		},
		"setting all options": {
			[]string{"-destroy", "-detailed-exitcode", "-input=false", "-out=saved.tfplan"},
			&Plan{
				DetailedExitCode: true,
				InputEnabled:     false,
				OutPath:          "saved.tfplan",
				ViewType:         ViewHuman,
				State:            &State{Lock: true},
				Vars:             &Vars{},
				Operation: &Operation{
					PlanMode:    plans.DestroyMode,
					Parallelism: 10,
					Refresh:     true,
				},
			},
		},
		"JSON view disables input": {
			[]string{"-json"},
			&Plan{
				DetailedExitCode: false,
				InputEnabled:     false,
				OutPath:          "",
				ViewType:         ViewJSON,
				State:            &State{Lock: true},
				Vars:             &Vars{},
				Operation: &Operation{
					PlanMode:    plans.NormalMode,
					Parallelism: 10,
					Refresh:     true,
				},
			},
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Operation{}, Vars{}, State{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParsePlan(tc.args)
			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParsePlan_invalid(t *testing.T) {
	got, diags := ParsePlan([]string{"-frob"})
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

func TestParsePlan_tooManyArguments(t *testing.T) {
	got, diags := ParsePlan([]string{"saved.tfplan"})
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

func TestParsePlan_targets(t *testing.T) {
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
			wantErr: "Invalid target \"foo.\": Dot must be followed by attribute name",
		},
		"invalid target": {
			args:    []string{"-target=data[0].foo"},
			want:    nil,
			wantErr: "Invalid target \"data[0].foo\": A data source name is required",
		},
		"empty target": {
			args:    []string{"-target="},
			want:    nil,
			wantErr: "Invalid target \"\": Must begin with a variable name.", // The error is `Invalid target "": Must begin with a variable name.`
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParsePlan(tc.args)
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

func TestParsePlan_targetFile(t *testing.T) {
	foobarbaz, _ := addrs.ParseTargetStr("foo_bar.baz")
	boop, _ := addrs.ParseTargetStr("module.boop")
	wow, _ := addrs.ParseTargetStr("wow.ham")
	testCases := map[string]struct {
		files []testFile
		want  []addrs.Targetable
	}{
		"target file no targets": {
			files: []testFile{},
			want:  nil,
		},
		"target file valid single target": {
			files: []testFile{
				{fileContent: "foo_bar.baz"}},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"target file valid multiple targets": {
			files: []testFile{
				{fileContent: "foo_bar.baz\nmodule.boop"},
			},
			want: []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"target file invalid target": {
			files: []testFile{
				{
					fileContent: "foo.",
					diags: hcl.Diagnostics{
						&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Attribute name required",
							Detail:   "Dot must be followed by attribute name.",
							Subject: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 5, Byte: 4},
								End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
							},
							Context: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 1},
								End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
							},
						},
					},
				},
			},
			want: nil,
		},
		"multiple files valid targets": {
			files: []testFile{
				{fileContent: "foo_bar.baz"},
				{fileContent: "module.boop"},
			},
			want: []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"multiple files invalid target": {
			files: []testFile{
				{fileContent: "foo_bar.baz"},
				{
					fileContent: "modu(le.boop",
					diags: hcl.Diagnostics{
						&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid character",
							Detail:   `Expected an attribute access or an index operator.`,
							Subject: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 5, Byte: 4},
								End:   hcl.Pos{Line: 1, Column: 6, Byte: 5},
							},
							Context: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 1},
								End:   hcl.Pos{Line: 1, Column: 6, Byte: 5},
							},
						},
					},
				},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"multiple files multiple invalid targets": {
			files: []testFile{
				{
					fileContent: "modu(le.boop",
					diags: hcl.Diagnostics{
						&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid character",
							Detail:   "Expected an attribute access or an index operator.",
							Subject: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 5, Byte: 4},
								End:   hcl.Pos{Line: 1, Column: 6, Byte: 5},
							},
							Context: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 1},
								End:   hcl.Pos{Line: 1, Column: 6, Byte: 5},
							},
						},
					},
				},
				{fileContent: "foo_bar.baz"},
				{
					fileContent: "wow^.ham",
					diags: []*hcl.Diagnostic{
						&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Unsupported operator",
							Detail:   `Bitwise operators are not supported.`,
							Subject: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 4, Byte: 3},
								End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
							},
						},
						&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid character",
							Detail:   "Expected an attribute access or an index operator.",
							Subject: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 4, Byte: 3},
								End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
							},
							Context: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 1},
								End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
							},
						},
					},
				},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"target file valid comment": {
			files: []testFile{
				{fileContent: "#foo_bar.baz"},
			},
			want: nil,
		},
		"target file valid spaces": {
			files: []testFile{
				{fileContent: "   foo_bar.baz"},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"target file valid tab": {
			files: []testFile{
				{fileContent: "\tfoo_bar.baz"},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"target file valid complicated": {
			files: []testFile{
				{fileContent: "\tmodule.boop\n#foo_bar.baz\nwow.ham"},
			},
			want: []addrs.Targetable{boop.Subject, wow.Subject},
		},
		"target file invalid bracket with spaces": {
			files: []testFile{
				{
					fileContent: `    [boop]`,
					diags: hcl.Diagnostics{
						&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Variable name required",
							Detail:   `Must begin with a variable name.`,
							Subject: &hcl.Range{
								Start: hcl.Pos{Line: 1, Column: 5, Byte: 4},
								End:   hcl.Pos{Line: 1, Column: 6, Byte: 5},
							},
						},
					},
				},
			},
			want: nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			targetFileArguments := []string{}
			wantDiags := tfdiags.Diagnostics{}
			for _, testFile := range tc.files {
				testFile.tempFileWriter(t)
				defer os.Remove(testFile.filePath)
				targetFileArguments = append(targetFileArguments, "-target-file="+testFile.filePath)

				// for setting the correct filePath on each wantDiag
				if len(testFile.diags) > 0 {
					for _, diag := range testFile.diags {
						diag.Subject.Filename = testFile.filePath
						if diag.Context != nil {
							diag.Context.Filename = testFile.filePath
						}
						wantDiags = wantDiags.Append(diag)
					}
				}
			}

			wantDiagsExported := wantDiags.ForRPC()

			got, gotDiags := ParsePlan(targetFileArguments)
			gotDiagsExported := gotDiags.ForRPC()

			if len(wantDiagsExported) != 0 || len(gotDiags) != 0 {
				if len(gotDiags) == 0 {
					t.Fatalf("expected diags but got none")
				}
				if len(wantDiagsExported) == 0 {
					t.Fatalf("got diags but didn't want any: %v", gotDiags.ErrWithWarnings())
				}

				if diff := cmp.Diff(gotDiagsExported, wantDiagsExported); diff != "" {
					t.Fatalf("wrong diagnostics\n%s", diff)
				}
			}
			if !cmp.Equal(got.Operation.Targets, tc.want) {
				t.Fatalf("unexpected result\n%s", cmp.Diff(got.Operation.Targets, tc.want))
			}
		})
	}
}

func TestParsePlan_excludes(t *testing.T) {
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
			wantErr: "Invalid exclude \"foo.\": Dot must be followed by attribute name",
		},
		"invalid exclude": {
			args:    []string{"-exclude=data[0].foo"},
			want:    nil,
			wantErr: "Invalid exclude \"data[0].foo\": A data source name is required",
		},
		"empty exclude": {
			args:    []string{"-exclude="},
			want:    nil,
			wantErr: "Invalid exclude \"\": Must begin with a variable name.",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParsePlan(tc.args)
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

func TestParsePlan_excludeFile(t *testing.T) {
	foobarbaz, _ := addrs.ParseTargetStr("foo_bar.baz")
	boop, _ := addrs.ParseTargetStr("module.boop")
	wow, _ := addrs.ParseTargetStr("wow.ham")
	testCases := map[string]struct {
		files     []testFile
		want      []addrs.Targetable
		wantDiags hcl.Diagnostics
	}{
		"exclude file no targets": {
			files: []testFile{},
			want:  nil,
		},
		"exclude file valid single target": {
			files: []testFile{
				{fileContent: "foo_bar.baz"},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"exclude file valid multiple targets": {
			files: []testFile{
				{fileContent: "foo_bar.baz\nmodule.boop"},
			},
			want: []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"exclude multiple files valid targets": {
			files: []testFile{
				{fileContent: "foo_bar.baz"},
				{fileContent: "module.boop"},
			},
			want: []addrs.Targetable{foobarbaz.Subject, boop.Subject},
		},
		"exclude file invalid target": {
			files: []testFile{
				{fileContent: "foo."},
			},
			want: nil,
			wantDiags: hcl.Diagnostics{
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid syntax",
					Detail:   `For exclude "foo.": Dot must be followed by attribute name.`,
					Subject: &hcl.Range{
						Start: hcl.Pos{Line: 1, Column: 1},
						End:   hcl.Pos{Line: 1, Column: 5, Byte: 4},
					},
				},
			},
		},
		"exclude file valid comment": {
			files: []testFile{
				{fileContent: "#foo_bar.baz"},
			},
			want: nil,
		},
		"exclude file valid spaces": {
			files: []testFile{
				{fileContent: "   foo_bar.baz"},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"exclude file valid tab": {
			files: []testFile{
				{fileContent: "\tfoo_bar.baz"},
			},
			want: []addrs.Targetable{foobarbaz.Subject},
		},
		"exclude file complicated": {
			files: []testFile{
				{fileContent: "\tmodule.boop\n#foo_bar.baz\nwow.ham"},
			},
			want: []addrs.Targetable{boop.Subject, wow.Subject},
		},
		"exclude file invalid bracket with spaces": {
			files: []testFile{
				{fileContent: `    [boop]`},
			},
			want: nil,
			wantDiags: hcl.Diagnostics{
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid syntax",
					Detail:   `For exclude "    [boop]": Must begin with a variable name.`,
					Subject: &hcl.Range{
						Start: hcl.Pos{Line: 1, Column: 1},
						End:   hcl.Pos{Line: 1, Column: 11, Byte: 10},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			targetFileArguments := []string{}

			for _, testFile := range tc.files {
				testFile.tempFileWriter(t)
				targetFileArguments = append(targetFileArguments, "-exclude-file="+testFile.filePath)
				for _, diag := range tc.wantDiags {
					if diag.Subject != nil {
						diag.Subject.Filename = testFile.filePath
					}
				}
			}
			wantDiagsExported := tfdiags.Diagnostics(nil).Append(tc.wantDiags).ForRPC()

			got, gotDiags := ParsePlan(targetFileArguments)
			if len(tc.wantDiags) != 0 || len(gotDiags) != 0 {
				if len(gotDiags) == 0 {
					t.Fatalf("expected diags but got none")
				}
				if len(tc.wantDiags) == 0 {
					t.Fatalf("got diags but didn't want any: %v", gotDiags.ErrWithWarnings())
				}
				gotDiagsExported := gotDiags.ForRPC()

				if diff := cmp.Diff(wantDiagsExported, gotDiagsExported); diff != "" {
					t.Error("wrong diagnostics\n" + diff)
				}
			}
			if !cmp.Equal(got.Operation.Excludes, tc.want) {
				t.Fatalf("unexpected result\n%s", cmp.Diff(got.Operation.Excludes, tc.want))
			}
		})
	}
}

func TestParsePlan_excludeAndTarget(t *testing.T) {
	testCases := [][]string{
		[]string{"-target-file=foo_file", "-exclude=foo_bar.baz"},
		[]string{"-target=foo.bar", "-exclude=module.baz"},
		[]string{"-exclude-file=foo_exclude_file", "-target=foo.targetdirect"},
	}
	for _, tc := range testCases {
		got, gotDiags := ParsePlan(tc)
		if len(gotDiags) == 0 {
			t.Fatalf("expected error, but there was none")
		}

		wantDiags := tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid combination of arguments",
				"The target and exclude planning options are mutually-exclusive. Each plan must use either only the target options or only the exclude options",
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
}

func TestParsePlan_vars(t *testing.T) {
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
			got, diags := ParsePlan(tc.args)
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

func (t *testFile) tempFileWriter(tt *testing.T) {
	tt.Helper()
	tempFile, err := os.CreateTemp(tt.TempDir(), "opentofu-test-files")
	if err != nil {
		tt.Fatal(err)
	}
	t.filePath = tempFile.Name()
	tempFile.WriteString(t.fileContent)
	if err != nil {
		tt.Fatal(err)
	}
	if err := tempFile.Close(); err != nil {
		tt.Fatal(err)
	}
}
