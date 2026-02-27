// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/hashicorp/go-getter"
	"github.com/ulikunitz/xz"
)

func extractModulePackage(tempF *os.File, targetDir string) error {
	// We reuse go-getter's decompressors for the actual extraction, because
	// they are already hardened against a number of previously-discovered
	// attacks involving crafted archives, and if any similar problems are
	// discovered later then upgrading go-getter will patch both this and
	// the go-getter-based module installation path.
	decompressor, err := sniffPackageDecompressor(tempF)
	if err != nil {
		return err
	}

	// getter.Decompressor wants to work with filenames rather than open
	// files, so we need to pass it the path to our temporary file now.
	// Note that this could potentially race if another process modifies
	// or removes the file before the decompressor opens it, but the
	// decompressors should all be robust to malicious input anyway.
	return decompressor.Decompress(targetDir, tempF.Name(), true, 0 /*default umask*/)
}

var packageDecompressSniffers = []func(*os.File, int64) getter.Decompressor{
	func(f *os.File, size int64) getter.Decompressor {
		// zip.NewReader succeeds only if the file has a zip header
		_, err := zip.NewReader(f, size)
		if err != nil {
			return nil
		}
		return &getter.ZipDecompressor{}
	},
	func(f *os.File, _ int64) getter.Decompressor {
		// gzip.NewReader succeeds only if the file has a gzip header
		_, err := gzip.NewReader(f)
		if err != nil {
			return nil
		}
		// Tar archives don't have a header, so we just assume that any
		// gzip stream is intended to contain a tar stream.
		return &getter.TarGzipDecompressor{}
	},
	func(f *os.File, _ int64) getter.Decompressor {
		buf := make([]byte, xz.HeaderLen)
		n, err := f.Read(buf)
		if err != nil || n != len(buf) {
			return nil // not able to read an xz header
		}
		if !xz.ValidHeader(buf) {
			return nil
		}
		// Tar archives don't have a header, so we just assume that any
		// xz stream is intended to contain a tar stream.
		return &getter.TarXzDecompressor{}
	},
	func(f *os.File, _ int64) getter.Decompressor {
		// encoding/bzip2 doesn't offer a direct way to ask if a stream
		// has a valid bzip2 header, so we'll check it manually by looking
		// for the "BZ" magic number at the very start. This sniffer is
		// intentionally last because it's doing the least checking and
		// so is most likely to generate false positives.
		// (The go-getter decompressor we return will check this more
		// thoroughly; our job here is just to decide if it seems likely
		// that this was intended to be a bzip2 stream.)

		// We're reading four bytes here because a file smaller than that
		// cannot possibly be a valid bzip2 stream. The last two bytes here
		// are real header fields though, not part of the magic number.
		buf := make([]byte, 4)
		n, err := f.Read(buf)
		if err != nil || n != len(buf) {
			return nil // not able to read a magic number
		}
		if buf[0] != 'B' || buf[1] != 'Z' {
			return nil // not the magic number we were looking for
		}
		// Tar archives don't have a header, so we just assume that any
		// bzip2 stream is intended to contain a tar stream.
		return &getter.TarBzip2Decompressor{}
	},
}

func sniffPackageDecompressor(tempF *os.File) (getter.Decompressor, error) {
	// Our approach here is just to try opening the file in a few different
	// ways where success implies a file was probably intended to be of
	// a particular format, but once we've decided we'll just let the real
	// decompressor do the actual validation of the package.

	info, err := tempF.Stat()
	if err != nil {
		return nil, err // Error message already mentions it was trying to stat
	}
	fileSize := info.Size()
	for _, sniffer := range packageDecompressSniffers {
		_, err := tempF.Seek(0, io.SeekStart)
		if err != nil {
			// Should not get here because the caller should always give us
			// a regular file. The error message from stdlib already mentions that
			// it was trying to seek.
			return nil, err
		}
		if ret := sniffer(tempF, fileSize); ret != nil {
			return ret, nil
		}
	}
	// If we fall out here then we weren't able to detect a supported format.
	return nil, fmt.Errorf("module package is not zip archive, or tar archive with gz, xz, or bzip2 compression")
}
