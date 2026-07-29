// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/workdir"
)

func TestFmt_TestFiles(t *testing.T) {
	const inSuffix = "_in.tftest.hcl"
	const outSuffix = "_out.tftest.hcl"
	const gotSuffix = "_got.tftest.hcl"
	entries, err := os.ReadDir("testdata/tftest-fmt")
	if err != nil {
		t.Fatal(err)
	}

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range entries {
		if info.IsDir() {
			continue
		}
		filename := info.Name()
		if !strings.HasSuffix(filename, inSuffix) {
			continue
		}
		testName := filename[:len(filename)-len(inSuffix)]
		t.Run(testName, func(t *testing.T) {
			inFile := filepath.Join("testdata", "tftest-fmt", testName+inSuffix)
			wantFile := filepath.Join("testdata", "tftest-fmt", testName+outSuffix)
			gotFile := filepath.Join(tmpDir, testName+gotSuffix)
			input, err := os.ReadFile(inFile)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(gotFile, input, 0700)
			if err != nil {
				t.Fatal(err)
			}

			view, done := testView(t)
			c := &FmtCommand{
				Meta: Meta{
					WorkingDir:       workdir.NewDir("."),
					testingOverrides: metaOverridesForProvider(testProvider()),
					View:             view,
				},
			}
			args := []string{gotFile}
			code := c.Run(args)
			output := done(t)
			if code != 0 {
				t.Fatalf("fmt command was unsuccessful:\n%s", output.Stderr())
			}

			got, err := os.ReadFile(gotFile)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(string(want), string(got)); diff != "" {
				t.Errorf("wrong result\n%s", diff)
			}
		})
	}
}

func TestFmt(t *testing.T) {
	const inSuffix = "_in.tf"
	const outSuffix = "_out.tf"
	const gotSuffix = "_got.tf"
	entries, err := os.ReadDir("testdata/fmt")
	if err != nil {
		t.Fatal(err)
	}

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range entries {
		if info.IsDir() {
			continue
		}
		filename := info.Name()
		if !strings.HasSuffix(filename, inSuffix) {
			continue
		}
		testName := filename[:len(filename)-len(inSuffix)]
		t.Run(testName, func(t *testing.T) {
			inFile := filepath.Join("testdata", "fmt", testName+inSuffix)
			wantFile := filepath.Join("testdata", "fmt", testName+outSuffix)
			gotFile := filepath.Join(tmpDir, testName+gotSuffix)
			input, err := os.ReadFile(inFile)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(gotFile, input, 0700)
			if err != nil {
				t.Fatal(err)
			}

			view, done := testView(t)
			c := &FmtCommand{
				Meta: Meta{
					WorkingDir:       workdir.NewDir("."),
					testingOverrides: metaOverridesForProvider(testProvider()),
					View:             view,
				},
			}
			args := []string{gotFile}
			code := c.Run(args)
			output := done(t)
			if code != 0 {
				t.Fatalf("fmt command was unsuccessful:\n%s", output.Stderr())
			}

			got, err := os.ReadFile(gotFile)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(string(want), string(got)); diff != "" {
				t.Errorf("wrong result\n%s", diff)
			}
		})
	}
}

func TestFmt_nonexist(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	missingDir := filepath.Join(tempDir, "doesnotexist")
	args := []string{missingDir}
	code := c.Run(args)
	output := done(t)
	if code != 2 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	expected := "No file or directory at"
	if actual := output.Stderr(); !strings.Contains(actual, expected) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, expected)
	}
}

func TestFmt_syntaxError(t *testing.T) {
	tempDir := testTempDirRealpath(t)

	invalidSrc := `
a = 1 +
`

	err := os.WriteFile(filepath.Join(tempDir, "invalid.tf"), []byte(invalidSrc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{tempDir}
	code := c.Run(args)
	output := done(t)
	if code != 2 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	expected := "Invalid expression"
	if actual := output.Stderr(); !strings.Contains(actual, expected) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, expected)
	}
}

func TestFmt_snippetInError(t *testing.T) {
	tempDir := testTempDirRealpath(t)

	backendSrc := `terraform {backend "s3" {}}`

	err := os.WriteFile(filepath.Join(tempDir, "backend.tf"), []byte(backendSrc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{"-no-color", tempDir}
	code := c.Run(args)
	output := done(t)
	if code != 2 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	substrings := []string{
		"Argument definition required",
		"line 1, in terraform",
		`1: terraform {backend "s3" {}}`,
	}
	for _, substring := range substrings {
		if actual := output.Stderr(); !strings.Contains(actual, substring) {
			t.Errorf("expected:\n%s\n\nto include: %q", actual, substring)
		}
	}
}

func TestFmt_manyArgs(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)
	// Add a second file
	secondSrc := `locals { x = 1 }`

	err := os.WriteFile(filepath.Join(tempDir, "second.tf"), []byte(secondSrc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{
		filepath.Join(tempDir, "main.tf"),
		filepath.Join(tempDir, "second.tf"),
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	got, err := filepath.Abs(strings.TrimSpace(output.Stdout()))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tempDir, fmtFixture.filename)

	if got != want {
		t.Fatalf("wrong output\ngot:  %s\nwant: %s", got, want)
	}
}

func TestFmt_workingDirectory(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)
	t.Chdir(tempDir)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	stdout := strings.Split(strings.TrimSpace(output.Stdout()), "\n")

	// Consistent order
	sort.Strings(stdout)

	for i, expected := range []string{fmtFixture.filename, fmtFixture.altFilename} {
		actual := stdout[i]
		if actual != expected {
			t.Fatalf("got: %q\nexpected: %q", actual, expected)
		}
	}
}

func TestFmt_directoryArg(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{tempDir}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	stdout := strings.Split(strings.TrimSpace(output.Stdout()), "\n")

	// Consistent order
	sort.Strings(stdout)

	for i, check := range []string{fmtFixture.filename, fmtFixture.altFilename} {
		got, err := filepath.Abs(stdout[i])
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(tempDir, check)

		if got != want {
			t.Fatalf("wrong output\ngot:  %s\nwant: %s", got, want)
		}
	}
}

func TestFmt_fileArg(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{filepath.Join(tempDir, fmtFixture.filename)}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	got, err := filepath.Abs(strings.TrimSpace(output.Stdout()))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tempDir, fmtFixture.filename)

	if got != want {
		t.Fatalf("wrong output\ngot:  %s\nwant: %s", got, want)
	}
}

func TestFmt_stdinArg(t *testing.T) {
	input := new(bytes.Buffer)
	input.Write(fmtFixture.input)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
		input: input,
	}

	args := []string{"-"}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	expected := fmtFixture.golden
	if actual := []byte(output.Stdout()); !bytes.Equal(actual, expected) {
		t.Fatalf("got: %q\nexpected: %q", actual, expected)
	}
}

func TestFmt_nonDefaultOptions(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{
		"-list=false",
		"-write=false",
		"-diff",
		tempDir,
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
	}

	expected := fmt.Sprintf("-%s+%s", fmtFixture.input, fmtFixture.golden)
	if actual := output.Stdout(); !strings.Contains(actual, expected) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, expected)
	}
}

func TestFmt_check(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
	}

	args := []string{
		"-check",
		tempDir,
	}
	code := c.Run(args)
	output := done(t)
	if code != 3 {
		t.Fatalf("wrong exit code. expected 3")
	}

	// The paths given back to the user are based on the path they gave us,
	// so the output here should include the temporary directory path as-is.
	if actual := output.Stdout(); !strings.Contains(actual, tempDir) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, tempDir)
	}
}

func TestFmt_checkStdin(t *testing.T) {
	input := new(bytes.Buffer)
	input.Write(fmtFixture.input)

	view, done := testView(t)
	c := &FmtCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(testProvider()),
			View:             view,
		},
		input: input,
	}

	args := []string{
		"-check",
		"-",
	}
	code := c.Run(args)
	output := done(t)
	if code != 3 {
		t.Fatalf("wrong exit code. expected 3, got %d", code)
	}

	stdout := output.Stdout()
	if len(stdout) > 0 {
		t.Fatalf("expected no output, got: %q", stdout)
	}
}

func TestFmt_symlinkedWorkingDir(t *testing.T) {
	// Regression test for the problem described in
	// https://github.com/opentofu/opentofu/issues/3879 : when the
	// working directory is reached through a symlink whose target is at
	// a different directory depth, normalizing an absolute path into a
	// working-directory-relative path produces a path that doesn't
	// resolve correctly, so "tofu fmt" must use the given path as-is.
	tempDir := testTempDirRealpath(t)

	// The symlink and its target are at different depths:
	//   tempDir/a/b/c/test.tf
	//   tempDir/z -> tempDir/a/b
	targetDir := filepath.Join(tempDir, "a", "b", "c")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(targetDir, "test.tf")
	symlinkDir := filepath.Join(tempDir, "z")
	if err := os.Symlink(filepath.Join(tempDir, "a", "b"), symlinkDir); err != nil {
		t.Skipf("cannot create symlink: %s", err)
	}

	t.Chdir(symlinkDir)

	run := func(t *testing.T, pathArg string) string {
		t.Helper()

		if err := os.WriteFile(targetFile, fmtFixture.input, 0600); err != nil {
			t.Fatal(err)
		}

		view, done := testView(t)
		c := &FmtCommand{
			Meta: Meta{
				WorkingDir:       workdir.NewDir("."),
				testingOverrides: metaOverridesForProvider(testProvider()),
				View:             view,
			},
		}

		code := c.Run([]string{pathArg})
		output := done(t)
		if code != 0 {
			t.Fatalf("wrong exit code. got %d. errors: \n%s", code, output.Stderr())
		}
		if stderr := output.Stderr(); strings.Contains(stderr, "No file or directory") {
			t.Fatalf("unexpected missing-path diagnostic:\n%s", stderr)
		}

		got, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, fmtFixture.golden) {
			t.Fatalf("target file was not formatted\ngot:  %q\nwant: %q", got, fmtFixture.golden)
		}

		return strings.TrimSpace(output.Stdout())
	}

	t.Run("absolute path", func(t *testing.T) {
		gotPath := run(t, targetFile)
		if gotPath != targetFile {
			t.Fatalf("wrong path in output\ngot:  %s\nwant: %s", gotPath, targetFile)
		}
	})

	t.Run("relative path", func(t *testing.T) {
		relPath := filepath.Join("c", "test.tf")
		gotPath := run(t, relPath)
		if gotPath != relPath {
			t.Fatalf("wrong path in output\ngot:  %s\nwant: %s", gotPath, relPath)
		}
	})
}

var fmtFixture = struct {
	filename      string
	altFilename   string
	input, golden []byte
}{
	"main.tf",
	"main.tofu",
	[]byte(`  foo  =  "bar"
`),
	[]byte(`foo = "bar"
`),
}

func fmtFixtureWriteDir(t *testing.T) string {
	dir := testTempDirRealpath(t)

	err := os.WriteFile(filepath.Join(dir, fmtFixture.filename), fmtFixture.input, 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dir, fmtFixture.altFilename), fmtFixture.input, 0600)
	if err != nil {
		t.Fatal(err)
	}

	return dir
}
