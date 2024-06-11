// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"testing"
)

type testContainersLogger struct {
	t *testing.T
}

func (t testContainersLogger) Printf(format string, v ...interface{}) {
	t.t.Helper()
	t.t.Logf(format, v...)
}
