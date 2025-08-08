package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NodeUnconfiguredProvider represents a provider during an "eval" walk.
// This special provider node type just initializes a provider and
// fetches its schema, without configuring it or otherwise interacting
// with it.
type NodeUnconfiguredProvider struct {
	*NodeAbstractProvider
}

var _ GraphNodeExecutable = (*NodeUnconfiguredProvider)(nil)

// // GraphNodeExecutable
func (n *NodeUnconfiguredProvider) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	_, err := evalCtx.InitProvider(ctx, n.Addr, addrs.NoKey)
	diags = diags.Append(err)
	provider, _, err := getProvider(ctx, evalCtx, n.Addr, addrs.NoKey)

	diags = diags.Append(err)
	return diags.Append(n.InitUnconfiguredProvider(ctx, evalCtx, provider))
}

func (n *NodeUnconfiguredProvider) InitUnconfiguredProvider(ctx context.Context, evalCtx EvalContext, provider providers.Interface) tfdiags.Diagnostics {
	providerKey := addrs.NoKey
	config := n.ProviderConfig()
	configBody := buildProviderConfig(ctx, evalCtx, n.Addr, config)

	schemaResp := provider.GetProviderSchema(ctx)
	diags := schemaResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey))

	configSchema := schemaResp.Provider.Block
	data := EvalDataForNoInstanceKey
	configVal, configBody, evalDiags := evalCtx.EvaluateBlock(ctx, configBody, configSchema, nil, data)
	diags = diags.Append(evalDiags)
	if evalDiags.HasErrors() {
		return diags
	}

	// If our config value contains any marked values, ensure those are
	// stripped out before sending this to the provider
	unmarkedConfigVal, _ := configVal.UnmarkDeep()

	// Allow the provider to validate and insert any defaults into the full
	// configuration.
	req := providers.ValidateProviderConfigRequest{
		Config: unmarkedConfigVal,
	}

	// ValidateProviderConfig is only used for validation. We are intentionally
	// ignoring the PreparedConfig field to maintain existing behavior.
	validateResp := provider.ValidateProviderConfig(ctx, req)
	diags = diags.Append(validateResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))

	return diags
}
