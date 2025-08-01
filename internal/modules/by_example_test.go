// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package modules

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// TestByExample attempts to load each of the modules in subdirectories of
// testdata/by-example, expecting them to either be valid or to have expected
// errors/warnings marked by end-of-line comments.
//
// There's more information on how this is intended to work in
// testdata/by-example/README.
//
// This is intended only for broad regression testing, focused on whether
// previously-valid things remain valid and previously-failing things
// continue to fail with similar errors. Most language features should also be
// covered by more specific unit tests, although it's okay to reuse some
// of the modules under testdata/by-example for those more specific tests to
// ease maintenence.
func TestByExample(t *testing.T) {
	baseDir := "testdata/by-example"
	exampleDirs, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range exampleDirs {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			dir := filepath.Join(baseDir, entry.Name())
			wantDiags := collectExpectedDiagnostics(t, dir)
			_, gotDiags := LoadModuleFromDir(dir)
			for _, got := range gotDiags {
				want, ok := wantDiags.ExpectedDiagnostic(got)
				if !ok {
					t.Errorf("unexpected diagnostic: %s", spew.Sdump(got))
					continue
				}
				if want.Severity != got.Severity() {
					t.Errorf("wrong severity in diagnostic\nwant: %s\ngot: %s", want.Severity, spew.Sdump(got))
				}
				if want.Summary != got.Description().Summary {
					t.Errorf("wrong summary in diagnostic\nwant: %s\ngot: %s", want.Summary, spew.Sdump(got))
				}
				// We remove our wantDiags entry for each match, so that
				// any leftovers for the end are missing expected diagostics.
				wantDiags.ForgetMatchingExpected(got)
			}
			for key, diag := range wantDiags {
				t.Errorf("missing expected diagnostic\nwant: %s %s at %s:%d", diag.Severity, diag.Summary, key.Filename, key.Line)
			}
		})
	}
}

type expectedDiagnostic struct {
	Severity tfdiags.Severity
	Summary  string
}

type expectedDiagnosticKey struct {
	Filename string
	Line     int
}

type expectedDiagnostics map[expectedDiagnosticKey]expectedDiagnostic

func collectExpectedDiagnostics(t *testing.T, dir string) expectedDiagnostics {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("can't read %s: %s", dir, err)
	}
	ret := make(expectedDiagnostics)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := filepath.Join(dir, entry.Name())
		src, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("can't read %s: %s", filename, err)
		}
		sc := hcl.NewRangeScanner(src, filename, bufio.ScanLines)
		for sc.Scan() {
			const errorPrefix = "ERROR:"
			const warningPrefix = "WARNING:"

			line := string(sc.Bytes())
			rng := sc.Range()
			if idx := strings.Index(line, errorPrefix); idx != -1 {
				wantSummary := strings.TrimSpace(line[idx+len(errorPrefix):])
				key := expectedDiagnosticKey{rng.Filename, rng.Start.Line}
				ret[key] = expectedDiagnostic{tfdiags.Error, wantSummary}
			}
			if idx := strings.Index(line, warningPrefix); idx != -1 {
				wantSummary := strings.TrimSpace(line[idx+len(warningPrefix):])
				key := expectedDiagnosticKey{rng.Filename, rng.Start.Line}
				ret[key] = expectedDiagnostic{tfdiags.Warning, wantSummary}
			}
		}
		if err := sc.Err(); err != nil {
			t.Fatalf("can't read %s: %s", filename, err)
		}
	}
	return ret
}

func (s expectedDiagnostics) ExpectedDiagnostic(diag tfdiags.Diagnostic) (expectedDiagnostic, bool) {
	rng := diag.Source().Subject
	if rng == nil {
		return expectedDiagnostic{}, false // Sourceless diagnostics cannot possibly match
	}
	key := expectedDiagnosticKey{rng.Filename, rng.Start.Line}
	expected, ok := s[key]
	return expected, ok
}

func (s expectedDiagnostics) ForgetMatchingExpected(diag tfdiags.Diagnostic) {
	rng := diag.Source().Subject
	if rng == nil {
		return
	}
	key := expectedDiagnosticKey{rng.Filename, rng.Start.Line}
	delete(s, key)
}
