// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"fmt"
	"testing"
)

// Log writes a log line for a compliance test.
func Log(t *testing.T, msg string, params ...any) {
	t.Helper()
	t.Logf("\033[32m%s\033[0m", fmt.Sprintf(msg, params...))
}

// Fail fails a compliance test.
func Fail(t *testing.T, msg string, params ...any) {
	t.Helper()
	t.Fatalf("\033[31m%s\033[0m", fmt.Sprintf(msg, params...))
}

// Skip skips a compliance test.
func Skip(t *testing.T, msg string, params ...any) {
	t.Helper()
	t.Skipf("\033[33m%s\033[0m", fmt.Sprintf(msg, params...))
}
