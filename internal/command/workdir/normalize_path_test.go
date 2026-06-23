// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNormalizePathSymlinkedMainDir checks that an absolute path is normalized
// correctly when the main directory is reached through a symlink. Previously
// the relative result was computed against the symlinked path, producing a
// path that no longer resolved to the intended file.
// See https://github.com/opentofu/opentofu/issues/3879.
func TestNormalizePathSymlinkedMainDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior under test is specific to Unix-like systems")
	}

	// Canonicalize the temp dir up front so the expectations don't depend on
	// the platform's own symlinking of the temp location (e.g. /var on macOS).
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("failed to resolve temp dir: %s", err)
	}

	realDir := filepath.Join(base, "a", "b")
	if err := os.MkdirAll(filepath.Join(realDir, "c"), 0o755); err != nil {
		t.Fatalf("failed to create real dir: %s", err)
	}
	target := filepath.Join(realDir, "c", "test.tf")
	if err := os.WriteFile(target, []byte("\n"), 0o644); err != nil {
		t.Fatalf("failed to create target file: %s", err)
	}

	// linkDir is a symlink that points at realDir but lives at a different
	// depth, mirroring the reproduction in the issue.
	linkDir := filepath.Join(base, "z")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("failed to create symlink: %s", err)
	}

	d := NewDir(linkDir)

	got := d.NormalizePath(target)
	if want := filepath.Join("c", "test.tf"); got != want {
		t.Fatalf("NormalizePath(%q) = %q; want %q", target, got, want)
	}

	// The normalized path must resolve back to the original file when joined
	// with the canonicalized main directory.
	if resolved := filepath.Join(realDir, got); resolved != target {
		t.Fatalf("normalized path resolved to %q; want %q", resolved, target)
	}
}
