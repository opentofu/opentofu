// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"
)

func TestStopHook_impl(t *testing.T) {
	var _ Hook = new(stopHook)
}
