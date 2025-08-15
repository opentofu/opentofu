// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package backend

import (
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"testing"

	"github.com/mitchellh/go-homedir"
)

func TestReadPathOrContents_Path(t *testing.T) {
	f := testTempFile(t)

	if _, err := io.WriteString(f, "foobar"); err != nil {
		t.Fatalf("err: %s", err)
	}
	f.Close()

	contents, err := ReadPathOrContents(f.Name())

	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if contents != "foobar" {
		t.Fatalf("expected contents %s, got %s", "foobar", contents)
	}
}

func TestReadPathOrContents_TildePath(t *testing.T) {
	home, err := homedir.Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	f := testTempFile(t, home)

	if _, err := io.WriteString(f, "foobar"); err != nil {
		t.Fatalf("err: %s", err)
	}
	f.Close()

	r := strings.NewReplacer(home, "~")
	homePath := r.Replace(f.Name())
	contents, err := ReadPathOrContents(homePath)

	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if contents != "foobar" {
		t.Fatalf("expected contents %s, got %s", "foobar", contents)
	}
}

func TestRead_PathNoPermission(t *testing.T) {
	// This skip condition is intended to get this test out of the way of users
	// who are building and testing OpenTofu from within a Linux-based Docker
	// container, where it is common for processes to be running as effectively
	// root within the container.
	if u, err := user.Current(); err == nil && u.Uid == "0" {
		t.Skip("This test is invalid when running as root, since root can read every file")
	}

	f := testTempFile(t)

	if _, err := io.WriteString(f, "foobar"); err != nil {
		t.Fatalf("err: %s", err)
	}
	f.Close()

	if runtime.GOOS == "windows" {
		currentUser, err := user.Current()
		if err != nil {
			t.Fatalf("failed to get current user: %s", err)
		}
		acl, _ := exec.Command("icacls", f.Name()).CombinedOutput()
		t.Logf("Before /deny %s", acl)

		cmd := exec.Command("icacls", f.Name(), "/deny", currentUser.Username+":(R)")
		if err := cmd.Run(); err != nil {
			t.Fatalf("err: %s", err)
		}

		acl, _ = exec.Command("icacls", f.Name()).CombinedOutput()
		t.Logf("After /deny %s", acl)
		defer func() {
			acl, _ := exec.Command("icacls", f.Name(), "/remove:d", currentUser.Username).CombinedOutput()
			t.Logf("After remove:d %s", acl)
		}()
	} else {
		if err := os.Chmod(f.Name(), 0); err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	contents, err := ReadPathOrContents(f.Name())

	if err == nil {
		t.Fatal("Expected error, got none!")
	}
	if contents != "" {
		t.Fatalf("expected contents %s, got %s", "", contents)
	}
}

func TestReadPathOrContents_Contents(t *testing.T) {
	input := "hello"

	contents, err := ReadPathOrContents(input)

	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if contents != input {
		t.Fatalf("expected contents %s, got %s", input, contents)
	}
}

func TestReadPathOrContents_TildeContents(t *testing.T) {
	input := "~/hello/notafile"

	contents, err := ReadPathOrContents(input)

	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if contents != input {
		t.Fatalf("expected contents %s, got %s", input, contents)
	}
}

// Returns an open tempfile based at baseDir.
//
// The temporary file is cleaned up automatically when the calling
// test is complete.
func testTempFile(t testing.TB, baseDir ...string) *os.File {
	t.Helper()

	base := ""
	if len(baseDir) == 1 {
		base = baseDir[0]
	}
	f, err := os.CreateTemp(base, "tf")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	t.Cleanup(func() {
		os.Remove(f.Name())
	})
	return f
}
