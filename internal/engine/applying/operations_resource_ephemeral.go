// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// EphemeralClose implements [exec.Operations].
func (ops *execOperations) EphemeralClose(
	ctx context.Context,
	object *states.ResourceInstanceObjectFull,
	providerClient *exec.ProviderClient,
) tfdiags.Diagnostics {
	// FIXME: Track instance address on resource instance objects
	//log.Printf("[TRACE] applying: EphemeralClose %s", object.Addr)
	panic("unimplemented")
}

// EphemeralOpen implements [exec.Operations].
func (ops *execOperations) EphemeralOpen(
	ctx context.Context,
	desired *eval.DesiredResourceInstance,
	providerClient *exec.ProviderClient,
) (*states.ResourceInstanceObjectFull, tfdiags.Diagnostics) {
	log.Printf("[TRACE] applying: EphemeralOpen %s", desired.Addr)
	panic("unimplemented")
}
