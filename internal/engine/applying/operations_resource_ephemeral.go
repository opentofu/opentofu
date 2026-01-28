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
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// EphemeralOpen implements [exec.Operations].
func (ops *execOperations) EphemeralOpen(
	ctx context.Context,
	desired *eval.DesiredResourceInstance,
	providerClient *exec.ProviderClient,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: EphemeralOpen %s using %s", desired.Addr, providerClient.InstanceAddr)
	panic("unimplemented")
}

// EphemeralClose implements [exec.Operations].
func (ops *execOperations) EphemeralClose(
	ctx context.Context,
	object *exec.ResourceInstanceObject,
	providerClient *exec.ProviderClient,
) tfdiags.Diagnostics {
	log.Printf("[TRACE] apply phase: EphemeralClose %s using %s", object.InstanceAddr, providerClient.InstanceAddr)
	panic("unimplemented")
}
