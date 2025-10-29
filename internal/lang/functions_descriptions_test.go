// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/funcs"
)

func TestFunctionDescriptions(t *testing.T) {
	scope := &Scope{
		ConsoleMode: true,
	}
	// This will implicitly test the parameter description count since
	// WithNewDescriptions will panic if the number doesn't match.
	allFunctions := scope.Functions()

	// plantimestamp isn't available with ConsoleMode: true
	// THis also includes the core:: prefixed functions
	expectedFunctionCount := (len(funcs.DescriptionList) - 1) * 2

	if len(allFunctions) != expectedFunctionCount {
		t.Errorf("DescriptionList length expected: %d, got %d", len(allFunctions), expectedFunctionCount)
	}

	for name := range allFunctions {
		_, ok := funcs.DescriptionList[addrs.ParseFunction(name).Name]
		if !ok {
			t.Errorf("missing DescriptionList entry for function %q", name)
		}
	}
}
