// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"fmt"
	"os"
	"path/filepath"
)

type SourceRange struct {
	Filename   string
	Start, End SourcePos
}

func (r *SourceRange) Equal(other *SourceRange) bool {
	if r == nil || other == nil {
		return r == other
	}

	return r.Filename == other.Filename && r.Start.Equal(other.Start) && r.End.Equal(other.End)
}

type SourcePos struct {
	Line, Column, Byte int
}

func (p SourcePos) Equal(other SourcePos) bool {
	return p.Line == other.Line && p.Column == other.Column && p.Byte == other.Byte
}

// StartString returns a string representation of the start of the range,
// including the filename and the line and column numbers.
func (r SourceRange) StartString() string {
	filename := r.Filename

	// We'll try to relative-ize our filename here so it's less verbose
	// in the common case of being in the current working directory. If not,
	// we'll just show the full path.
	wd, err := os.Getwd()
	if err == nil {
		relFn, err := filepath.Rel(wd, filename)
		if err == nil {
			filename = relFn
		}
	}

	return fmt.Sprintf("%s:%d,%d", filename, r.Start.Line, r.Start.Column)
}
