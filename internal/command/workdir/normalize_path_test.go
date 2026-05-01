// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNormalizePathSymlink verifies that NormalizePath correctly handles an
// absolute path when the working directory was reached via a symlink.
//
// Regression test for https://github.com/opentofu/opentofu/issues/3879:
// `tofu fmt` would fail with "No file or directory at <relative-path>" when
// given an absolute path from a symlinked working directory, because
// filepath.Abs resolved the symlinked cwd to its real path, but the given
// absolute path retained the symlink components, causing filepath.Rel to
// produce a nonsensical result.
func TestNormalizePathSymlink(t *testing.T) {
	// Create a real directory: <tmpdir>/a/b/c
	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("failed to create real dir: %s", err)
	}

	// Create a file inside the real directory
	testFile := filepath.Join(realDir, "test.tf")
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %s", err)
	}

	// Create a symlink: <tmpdir>/z -> <tmpdir>/a/b
	symlinkDir := filepath.Join(tmpDir, "z")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skipf("cannot create symlink (skipping on platforms without symlink support): %s", err)
	}

	// Simulate running from the symlinked directory by creating a Dir
	// anchored at the symlink path.
	d := NewDir(symlinkDir)

	// The absolute path to the file uses the real path (as constructed by $PWD/file).
	absFilePath := testFile // /tmp/.../a/b/test.tf

	result := d.NormalizePath(absFilePath)

	// The result must be a valid relative path that, when joined with the
	// symlink dir, points to the actual file.
	joined := filepath.Join(symlinkDir, result)
	if _, err := os.Stat(joined); err != nil {
		t.Errorf("NormalizePath returned %q which does not resolve to an existing file (joined=%q): %s", result, joined, err)
	}
}
