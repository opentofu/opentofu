// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package copy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
)

// CopyDir recursively copies all of the files within the directory given in
// src to the directory given in dst.
//
// Both directories should already exist. If the destination directory is
// non-empty then the new files will merge in with the old, overwriting any
// files that have a relative path in common between source and destination.
//
// Recursive copying of directories is inevitably a rather opinionated sort of
// operation, so this function won't be appropriate for all use-cases. Some
// of the "opinions" it has are described in the following paragraphs:
//
// Symlinks in the source directory are recreated with the same target in the
// destination directory. If the symlink is to a directory itself, that
// directory is not recursively visited for further copying.
//
// File and directory modes are not preserved exactly, but the executable
// flag is preserved for files on operating systems where it is significant.
//
// Any "dot files" it encounters along the way are skipped, even on platforms
// that do not normally ascribe special meaning to files with names starting
// with dots.
//
// Callers may rely on the above details and other undocumented details of
// this function, so if you intend to change it be sure to review the callers
// first and make sure they are compatible with the change you intend to make.
func CopyDir(dst, src string) error {
	src, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("failed to evaluate symlinks for source %q: %w", src, err)
	}

	var errg errgroup.Group

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking the path %q: %w", path, err)
		}

		if path == src {
			return nil
		}

		if strings.HasPrefix(filepath.Base(path), ".") {
			// Skip any dot files
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// The "path" has the src prefixed to it. We need to join our
		// destination with the path without the src on it.
		dstPath := filepath.Join(dst, path[len(src):])

		// If we have a directory, make that subdirectory, then continue
		// the walk.
		if info.IsDir() {
			if path == filepath.Join(src, dst) {
				// dst is in src; don't walk it.
				return nil
			}

			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %q: %w", dstPath, err)
			}

			return nil
		}

		errg.Go(func() error {

			// we don't want to try and copy the same file over itself.
			if eq, err := SameFile(path, dstPath); err != nil {
				return fmt.Errorf("failed to check if files are the same: %w", err)
			} else if eq {
				return nil
			}

			// If the current path is a symlink, recreate the symlink relative to
			// the dst directory
			if info.Mode()&os.ModeSymlink == os.ModeSymlink {
				target, err := os.Readlink(path)
				if err != nil {
					return fmt.Errorf("failed to read symlink %q: %w", path, err)
				}

				if err := os.Symlink(target, dstPath); err != nil {
					return fmt.Errorf("failed to create symlink %q: %w", dstPath, err)
				}
				return nil
			}
			return copyFile(dstPath, path, info.Mode())
		})
		return nil
	}
	err = filepath.Walk(src, walkFn)
	waitErr := errg.Wait()
	return errors.Join(waitErr, err)
}

// copyFile copies the contents and mode of the file from src to dst.
func copyFile(dst, src string, mode os.FileMode) error {
	srcF, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %q: %w", src, err)
	}
	defer srcF.Close()

	dstF, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %q: %w", dst, err)
	}

	if _, err := io.Copy(dstF, srcF); err != nil {
		dstF.Close() // Ignore error from Close since io.Copy already failed
		return fmt.Errorf("failed to copy contents from %q to %q: %w", src, dst, err)
	}

	if err := dstF.Close(); err != nil {
		return fmt.Errorf("failed to close destination file %q: %w", dst, err)
	}

	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to set file mode for %q: %w", dst, err)
	}

	return nil
}

// SameFile returns true if the two given paths refer to the same physical
// file on disk, using the unique file identifiers from the underlying
// operating system. For example, on Unix systems this checks whether the
// two files are on the same device and have the same inode.
func SameFile(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}

	aInfo, err := os.Lstat(a)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	bInfo, err := os.Lstat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return os.SameFile(aInfo, bInfo), nil
}
