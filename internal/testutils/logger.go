// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/opentofu/opentofu/internal/logging"
)

type testContainersLogger struct {
	t *testing.T
}

func (t testContainersLogger) Printf(format string, v ...interface{}) {
	t.t.Helper()
	t.t.Logf(format, v...)
}

type testHCLogAdapter struct {
	t *testing.T
}

func (t testHCLogAdapter) Accept(name string, level hclog.Level, msg string, args ...interface{}) {
	t.t.Helper()
	msg = fmt.Sprintf(msg, args...)
	t.t.Logf("%s\t%s\t%s", name, level.String(), msg)
}

func SetupTestLogger(t *testing.T) {
	logging.RegisterSinkAdapter(&testHCLogAdapter{t})
}
