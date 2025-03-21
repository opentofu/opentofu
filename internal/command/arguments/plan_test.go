// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

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
		"invalid target file and exclude": {
			args:    []string{"-target-file=foo_file", "-exclude=foo_bar.baz"},
			want:    nil,
			wantErr: "Cannot combine both target and exclude flags. Please only target or exclude resource",
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
	testCases := map[string]struct {
		fileContent string
		want        []addrs.Targetable
		wantErr     string
	}{
		"target file valid": {
			fileContent: "foo_bar.baz",
			want:        []addrs.Targetable{foobarbaz.Subject},
		},
		"target file invalid": {
			fileContent: "foo.",
			want:        nil,
			wantErr:     "Invalid target \"foo.\": Dot must be followed by attribute name",
		},
		//	Other required tests
		//		* First character is `#` is invalid, comments not allowed
		//		* Has lines that start with spaces and tabs on lines that contain
		//			errors so that we can make sure the error diagnostics report
		//			correct positions for the invalid tokens in those cases
		//		* Empty file
		//		* File with many valid lines
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			file := tempFileWriter(tc.fileContent)
			defer os.Remove(file.Name())
			got, diags := ParsePlan([]string{"-target-file=" + file.Name()})
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

func TestParsePlan_excludeAndTarget(t *testing.T) {
	got, gotDiags := ParsePlan([]string{"-exclude=foo_bar.baz", "-target=foo_bar.bar"})
	if len(gotDiags) == 0 {
		t.Fatalf("expected error, but there was none")
	}

	wantDiags := tfdiags.Diagnostics{
		tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid combination of arguments",
			"-target and -exclude flags cannot be used together. Please remove one of the flags",
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

// Don't forget to os.Remove(file) after calling this function
func tempFileWriter(fileContent string) *os.File {
	file, err := os.CreateTemp("", "opentofu-test-arguments")
	if err != nil {
		log.Fatal(err)
	}
	file.WriteString(fileContent)
	if err != nil {
		log.Fatal(err)
	}

	file.Seek(0, 0)
	s := bufio.NewScanner(file)
	for s.Scan() {
		fmt.Println("tempFileWriter function printLin", s.Text())
	}
	if err = s.Err(); err != nil {
		log.Fatal("error reading temp file", err)
	}
	return file
}
