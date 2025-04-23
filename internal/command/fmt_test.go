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
	"github.com/mitchellh/cli"
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

			ui := cli.NewMockUi()
			c := &FmtCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(testProvider()),
					Ui:               ui,
				},
			}
			args := []string{gotFile}
			if code := c.Run(args); code != 0 {
				t.Fatalf("fmt command was unsuccessful:\n%s", ui.ErrorWriter.String())
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

			ui := cli.NewMockUi()
			c := &FmtCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(testProvider()),
					Ui:               ui,
				},
			}
			args := []string{gotFile}
			if code := c.Run(args); code != 0 {
				t.Fatalf("fmt command was unsuccessful:\n%s", ui.ErrorWriter.String())
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	missingDir := filepath.Join(tempDir, "doesnotexist")
	args := []string{missingDir}
	if code := c.Run(args); code != 2 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	expected := "No file or directory at"
	if actual := ui.ErrorWriter.String(); !strings.Contains(actual, expected) {
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{tempDir}
	if code := c.Run(args); code != 2 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	expected := "Invalid expression"
	if actual := ui.ErrorWriter.String(); !strings.Contains(actual, expected) {
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{tempDir}
	if code := c.Run(args); code != 2 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	substrings := []string{
		"Argument definition required",
		"line 1, in terraform",
		`1: terraform {backend "s3" {}}`,
	}
	for _, substring := range substrings {
		if actual := ui.ErrorWriter.String(); !strings.Contains(actual, substring) {
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{
		filepath.Join(tempDir, "main.tf"),
		filepath.Join(tempDir, "second.tf"),
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	got, err := filepath.Abs(strings.TrimSpace(ui.OutputWriter.String()))
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{}
	if code := c.Run(args); code != 0 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	output := strings.Split(strings.TrimSpace(ui.OutputWriter.String()), "\n")

	// Consistent order
	sort.Strings(output)

	for i, expected := range []string{fmtFixture.filename, fmtFixture.altFilename} {
		actual := output[i]
		if actual != expected {
			t.Fatalf("got: %q\nexpected: %q", actual, expected)
		}
	}
}

func TestFmt_directoryArg(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{tempDir}
	if code := c.Run(args); code != 0 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	output := strings.Split(strings.TrimSpace(ui.OutputWriter.String()), "\n")

	// Consistent order
	sort.Strings(output)

	for i, check := range []string{fmtFixture.filename, fmtFixture.altFilename} {
		got, err := filepath.Abs(output[i])
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{filepath.Join(tempDir, fmtFixture.filename)}
	if code := c.Run(args); code != 0 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	got, err := filepath.Abs(strings.TrimSpace(ui.OutputWriter.String()))
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

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
		input: input,
	}

	args := []string{"-"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	expected := fmtFixture.golden
	if actual := ui.OutputWriter.Bytes(); !bytes.Equal(actual, expected) {
		t.Fatalf("got: %q\nexpected: %q", actual, expected)
	}
}

func TestFmt_nonDefaultOptions(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{
		"-list=false",
		"-write=false",
		"-diff",
		tempDir,
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("wrong exit code. errors: \n%s", ui.ErrorWriter.String())
	}

	expected := fmt.Sprintf("-%s+%s", fmtFixture.input, fmtFixture.golden)
	if actual := ui.OutputWriter.String(); !strings.Contains(actual, expected) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, expected)
	}
}

func TestFmt_check(t *testing.T) {
	tempDir := fmtFixtureWriteDir(t)

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
	}

	args := []string{
		"-check",
		tempDir,
	}
	if code := c.Run(args); code != 3 {
		t.Fatalf("wrong exit code. expected 3")
	}

	// Given that we give relative paths back to the user, normalize this temp
	// dir so that we're comparing against a relative-ized (normalized) path
	tempDir = c.normalizePath(tempDir)

	if actual := ui.OutputWriter.String(); !strings.Contains(actual, tempDir) {
		t.Fatalf("expected:\n%s\n\nto include: %q", actual, tempDir)
	}
}

func TestFmt_checkStdin(t *testing.T) {
	input := new(bytes.Buffer)
	input.Write(fmtFixture.input)

	ui := new(cli.MockUi)
	c := &FmtCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(testProvider()),
			Ui:               ui,
		},
		input: input,
	}

	args := []string{
		"-check",
		"-",
	}
	if code := c.Run(args); code != 3 {
		t.Fatalf("wrong exit code. expected 3, got %d", code)
	}

	if ui.OutputWriter != nil {
		t.Fatalf("expected no output, got: %q", ui.OutputWriter.String())
	}
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
