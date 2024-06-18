// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestContext(t *testing.T) {
	const checkTime = 20 * time.Second
	ctx := testutils.Context(t)
	if ctx == nil {
		t.Fatalf("No context returned from testutils.Context")
	}
	tDeadline, tOk := t.Deadline()
	ctxDeadline, ctxOk := ctx.Deadline()
	if tOk != ctxOk {
		t.Fatalf("The testutils.Context function does not correctly set up the deadline ('ok' value mismatch)")
	}
	if tOk {
		if !ctxDeadline.Before(tDeadline.Add(checkTime)) {
			t.Fatalf(
				"The testutils.Context function does not correctly set up the deadline (not enough time left for cleanup)",
			)
		}
	}
}
