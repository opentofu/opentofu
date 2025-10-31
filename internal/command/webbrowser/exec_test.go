// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package webbrowser

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const fakeBrowserLaunchCmdOutputEnvName = "OPENTOFU_WEBBROWSER_EXEC_TEST_OUTPUT"

// TestMain overrides the test entrypoint so that we can reuse the test
// executable as a fake browser launcher command when testing
// [NewExecLauncher].
func TestMain(m *testing.M) {
	if f := os.Getenv(fakeBrowserLaunchCmdOutputEnvName); f != "" {
		err := fakeBrowserLauncherCommand(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fake browser launcher failed: %s", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeBrowserLauncherCommand(outputFilename string) error {
	// The "exec" browser launcher must pass the URL to open in the
	// first argument to the executable it launches.
	url := os.Args[1]
	return os.WriteFile(outputFilename, []byte(url), os.ModePerm)
}

func TestExecLauncher(t *testing.T) {
	// For this test we re-use the text executable as a fake browser-launching
	// program, through the special logic in [TestMain].
	fakeExec := os.Args[0]

	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "browser-exec-launcher-test")
	t.Setenv(fakeBrowserLaunchCmdOutputEnvName, outputFile)

	launcher := NewExecLauncher(fakeExec)
	err := launcher.OpenURL("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result), "http://example.com/"; got != want {
		t.Errorf("wrong URL written to output file\ngot:  %s\nwant: %s", got, want)
	}
}

func TestParseBrowserEnv_success(t *testing.T) {
	// ParseBrowserEnv only actually needs to work on Unix-like systems, so
	// the test scenario below is not written to be portable.
	// There's no runtime equivalent of the "unix" build tag, so we just
	// explicitly test the two main Unix OSes we support here.
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("ParseBrowserEnv is only for unix systems")
	}

	tmpDir := t.TempDir()
	fakeExec := filepath.Join(tmpDir, "fake-launch-browser")
	err := os.WriteFile(fakeExec, []byte(`not a real program`), 0755)
	if err != nil {
		// NOTE: This test requires the temp directory to be somewhere that
		// allows executables, so this won't work if the temp directory is
		// on a "noexec" mount on a Unix-style system.
		t.Fatal(err)
	}
	// Temporarily we'll reset the search path to just our temp directory
	t.Setenv("PATH", tmpDir)
	result := ParseBrowserEnv("fake-launch-browser")
	if result == "" {
		t.Fatal("failed to find fake executable")
	}
	if got, want := filepath.Base(result), "fake-launch-browser"; got != want {
		t.Fatalf("returned path %q has wrong basename %q; want %q", result, got, want)
	}
}

func TestParseBrowserEnv_empty(t *testing.T) {
	result := ParseBrowserEnv("")
	if result != "" {
		t.Errorf("returned %q, but wanted empty string", result)
	}
}

func TestParseBrowserEnv_esrComplexSpec(t *testing.T) {
	// The following tests with a strings following the more complex
	// interpretation of BROWSER from http://www.catb.org/~esr/BROWSER/ , which
	// OpenTofu intentionally doesn't support and so should be treated as
	// if the environment variable isn't set at all.
	t.Run(`with %s`, func(t *testing.T) {
		// The esr proposal calls for checking whether there's a %s sequence
		// in the value and then, if so, substituting the URL there and then
		// passing the entire result to a shell. This is the main thing that
		// different implementations did inconsistently, because it's
		// unspecified whether the %s should be placed in quotes in the
		// environment variable, if those quotes should be inserted by the
		// program acting on the variable, or if some other shell escaping
		// strategy should be used instead. We just ignore this form entirely
		// because it's apparently not commonly used and it's unclear how
		// to implement it without causing security problems.
		result := ParseBrowserEnv("example %s")
		if result != "" {
			t.Errorf("returned %q, but wanted empty string", result)
		}
	})
	t.Run("multiple commands", func(t *testing.T) {
		// The esr proposal calls for splitting the string on semicolon
		// and trying one command at a time until one succeeds. That's
		// ambiguous with there being a single command whose path contains
		// a semicolon, so we just try to treat it as a single command and
		// ignore the value if that doesn't work. In practice the need for
		// multiple options to try tends to be met instead by setting BROWSER
		// to refer to a wrapper script that deals with the selection policy,
		// which is the pattern OpenTofu supports.
		result := ParseBrowserEnv("example1;example2")
		if result != "" {
			t.Errorf("returned %q, but wanted empty string", result)
		}
	})
}
