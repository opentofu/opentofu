// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

// e2eTestingApplyHooks simulates particularly nasty
// scenarios within OpenTofu's apply engine, such
// as panics due to programming errors
type e2eTestingApplyHook struct {
	tofu.NilHook
}

func (e *e2eTestingApplyHook) PreApply(addr addrs.AbsResourceInstance, gen states.Generation, action plans.Action, priorState, plannedNewState cty.Value) (tofu.HookAction, error) {
	if resourceString := os.Getenv("TOFU_E2E_APPLY_RESOURCE_PANIC"); resourceString == addr.String() {
		panic("Crash simulating a critical programming error in the apply process, this should produce an errored.tfstate file")
	}
	return tofu.HookActionContinue, nil
}
