// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DataRead implements [exec.Operations].
func (ops *execOperations) DataRead(
	ctx context.Context,
	desired *eval.DesiredResourceInstance,
	plannedVal cty.Value,
	providerClient *exec.ProviderClient,
) (*states.ResourceInstanceObjectFull, tfdiags.Diagnostics) {
	log.Printf("[TRACE] applying: DataRead %s", desired.Addr)
	panic("unimplemented")
}
