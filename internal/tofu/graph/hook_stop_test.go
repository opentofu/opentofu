// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"testing"

	"github.com/opentofu/opentofu/internal/tofu/hooks"
)

func TestStopHook_impl(t *testing.T) {
	var _ hooks.Hook = new(stopHook)
}
