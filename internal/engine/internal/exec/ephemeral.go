package exec

import (
	"context"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type OpenEphemeralResourceInstance struct {
	State *ResourceInstanceObject
	Close func(context.Context) tfdiags.Diagnostics
}
