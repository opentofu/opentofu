// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"
)

func TestMockUIOutput(t *testing.T) {
	var _ UIOutput = new(MockUIOutput)
}
