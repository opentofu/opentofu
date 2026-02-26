// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import "strings"

// writer implements [io.Writer] and redirects the content through the given function.
type writer struct {
	writeFn func(msg string)
}

func (w *writer) Write(p []byte) (n int, err error) {
	n = len(p)
	out := strings.TrimSuffix(string(p), "\n")
	w.writeFn(out)
	return n, nil
}
