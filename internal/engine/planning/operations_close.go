package planning

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type closeOperations struct {
	providerInstances  *providerInstances
	ephemeralInstances *ephemeralInstances
}

var _ exec.Operations = (*closeOperations)(nil)

func (h *closeOperations) ProviderInstanceConfig(ctx context.Context, instAddr addrs.AbsProviderInstanceCorrect) (*exec.ProviderInstanceConfig, tfdiags.Diagnostics) {
	return &exec.ProviderInstanceConfig{InstanceAddr: instAddr}, nil
}

func (h *closeOperations) ProviderInstanceOpen(ctx context.Context, config *exec.ProviderInstanceConfig) (*exec.ProviderClient, tfdiags.Diagnostics) {
	return &exec.ProviderClient{InstanceAddr: config.InstanceAddr}, nil
}

func (h *closeOperations) ProviderInstanceClose(ctx context.Context, client *exec.ProviderClient) tfdiags.Diagnostics {
	err := h.providerInstances.callClose(ctx, client.InstanceAddr)
	return tfdiags.Diagnostics{}.Append(err)
}

func (h *closeOperations) ResourceInstanceDesired(ctx context.Context, instAddr addrs.AbsResourceInstance) (*eval.DesiredResourceInstance, tfdiags.Diagnostics) {
	if instAddr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
		return &eval.DesiredResourceInstance{
			Addr: instAddr,
		}, nil
	}
	return nil, nil
}

func (h *closeOperations) ResourceInstancePrior(ctx context.Context, instAddr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return nil, nil
}

func (h *closeOperations) ResourceInstancePostconditions(ctx context.Context, result *exec.ResourceInstanceObject) tfdiags.Diagnostics {
	return nil
}

func (h *closeOperations) ManagedFinalPlan(ctx context.Context, desired *eval.DesiredResourceInstance, prior *exec.ResourceInstanceObject, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ManagedResourceObjectFinalPlan, tfdiags.Diagnostics) {
	return nil, nil
}

func (h *closeOperations) ManagedApply(ctx context.Context, plan *exec.ManagedResourceObjectFinalPlan, fallback *exec.ResourceInstanceObject, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return nil, nil
}

func (h *closeOperations) ManagedDepose(ctx context.Context, instAddr addrs.AbsResourceInstance) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return nil, nil
}

func (h *closeOperations) ManagedAlreadyDeposed(ctx context.Context, instAddr addrs.AbsResourceInstance, deposedKey states.DeposedKey) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return nil, nil
}

func (h *closeOperations) DataRead(ctx context.Context, desired *eval.DesiredResourceInstance, plannedVal cty.Value, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return nil, nil
}

func (h *closeOperations) EphemeralOpen(ctx context.Context, desired *eval.DesiredResourceInstance, providerClient *exec.ProviderClient) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return &exec.ResourceInstanceObject{InstanceAddr: desired.Addr}, nil
}

func (h *closeOperations) EphemeralClose(ctx context.Context, object *exec.ResourceInstanceObject, providerClient *exec.ProviderClient) tfdiags.Diagnostics {
	return h.ephemeralInstances.callClose(ctx, object.InstanceAddr)
}
