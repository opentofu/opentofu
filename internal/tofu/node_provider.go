// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// NodeApplyableProvider represents a provider during an apply.
type NodeApplyableProvider struct {
	*NodeAbstractProvider
}

var (
	_ GraphNodeExecutable = (*NodeApplyableProvider)(nil)
)

// GraphNodeExecutable
func (n *NodeApplyableProvider) Execute(_ context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	instances, diags := n.initInstances(evalCtx, op)

	for key, provider := range instances {
		diags = diags.Append(n.executeInstance(evalCtx, op, key, provider))
	}

	return diags
}
func (n *NodeApplyableProvider) initInstances(ctx EvalContext, op walkOperation) (map[addrs.InstanceKey]providers.Interface, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var initKeys []addrs.InstanceKey
	// config -> init (different due to validate skipping most for_each logic)
	instanceKeys := make(map[addrs.InstanceKey]addrs.InstanceKey)
	if n.Config == nil || n.Config.Instances == nil {
		initKeys = append(initKeys, addrs.NoKey)
		instanceKeys[addrs.NoKey] = addrs.NoKey
	} else if op == walkValidate {
		// Instances are set AND we are validating
		initKeys = append(initKeys, addrs.NoKey)
		for key := range n.Config.Instances {
			instanceKeys[key] = addrs.NoKey
		}
	} else {
		// Instances are set AND we are not validating
		for key := range n.Config.Instances {
			initKeys = append(initKeys, key)
			instanceKeys[key] = key
		}
	}

	for _, key := range initKeys {
		_, err := ctx.InitProvider(n.Addr, key)
		diags = diags.Append(err)
	}
	if diags.HasErrors() {
		return nil, diags
	}

	instances := make(map[addrs.InstanceKey]providers.Interface)
	for configKey, initKey := range instanceKeys {
		provider, _, err := getProvider(ctx, n.Addr, initKey)
		diags = diags.Append(err)
		instances[configKey] = provider
	}
	if diags.HasErrors() {
		return nil, diags
	}

	return instances, diags
}
func (n *NodeApplyableProvider) executeInstance(ctx EvalContext, op walkOperation, providerKey addrs.InstanceKey, provider providers.Interface) tfdiags.Diagnostics {
	switch op {
	case walkValidate:
		log.Printf("[TRACE] NodeApplyableProvider: validating configuration for %s", n.Addr)
		return n.ValidateProvider(ctx, providerKey, provider)
	case walkPlan, walkPlanDestroy, walkApply, walkDestroy:
		log.Printf("[TRACE] NodeApplyableProvider: configuring %s", n.Addr)
		return n.ConfigureProvider(ctx, providerKey, provider, false)
	case walkImport:
		log.Printf("[TRACE] NodeApplyableProvider: configuring %s (requiring that configuration is wholly known)", n.Addr)
		return n.ConfigureProvider(ctx, providerKey, provider, true)
	}
	return nil
}

func (n *NodeApplyableProvider) ValidateProvider(ctx EvalContext, providerKey addrs.InstanceKey, provider providers.Interface) tfdiags.Diagnostics {
	configBody := buildProviderConfig(ctx, n.Addr, n.ProviderConfig())

	// if a provider config is empty (only an alias), return early and don't continue
	// validation. validate doesn't need to fully configure the provider itself, so
	// skipping a provider with an implied configuration won't prevent other validation from completing.
	_, noConfigDiags := configBody.Content(&hcl.BodySchema{})
	if !noConfigDiags.HasErrors() {
		return nil
	}

	schemaResp := provider.GetProviderSchema()
	diags := schemaResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
	if diags.HasErrors() {
		return diags
	}

	configSchema := schemaResp.Provider.Block
	if configSchema == nil {
		// Should never happen in real code, but often comes up in tests where
		// mock schemas are being used that tend to be incomplete.
		log.Printf("[WARN] ValidateProvider: no config schema is available for %s, so using empty schema", n.Addr)
		configSchema = &configschema.Block{}
	}

	data := EvalDataForNoInstanceKey
	if n.Config != nil && n.Config.Instances != nil {
		data = n.Config.Instances[providerKey]
	}

	configVal, _, evalDiags := ctx.EvaluateBlock(configBody, configSchema, nil, data)
	if evalDiags.HasErrors() {
		return diags.Append(evalDiags)
	}
	diags = diags.Append(evalDiags)

	// If our config value contains any marked values, ensure those are
	// stripped out before sending this to the provider
	unmarkedConfigVal, _ := configVal.UnmarkDeep()

	req := providers.ValidateProviderConfigRequest{
		Config: unmarkedConfigVal,
	}

	validateResp := provider.ValidateProviderConfig(req)
	diags = diags.Append(validateResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))

	return diags
}

// ConfigureProvider configures a provider that is already initialized and retrieved.
// If verifyConfigIsKnown is true, ConfigureProvider will return an error if the
// provider configVal is not wholly known and is meant only for use during import.
func (n *NodeApplyableProvider) ConfigureProvider(ctx EvalContext, providerKey addrs.InstanceKey, provider providers.Interface, verifyConfigIsKnown bool) tfdiags.Diagnostics {
	config := n.ProviderConfig()

	configBody := buildProviderConfig(ctx, n.Addr, config)

	resp := provider.GetProviderSchema()
	diags := resp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
	if diags.HasErrors() {
		return diags
	}

	configSchema := resp.Provider.Block
	data := EvalDataForNoInstanceKey
	if n.Config != nil && n.Config.Instances != nil {
		data = n.Config.Instances[providerKey]
	}

	configVal, configBody, evalDiags := ctx.EvaluateBlock(configBody, configSchema, nil, data)
	diags = diags.Append(evalDiags)
	if evalDiags.HasErrors() {
		return diags
	}

	if verifyConfigIsKnown && !configVal.IsWhollyKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration",
			Detail:   fmt.Sprintf("The configuration for %s depends on values that cannot be determined until apply.", n.Addr),
			Subject:  &config.DeclRange,
		})
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
	validateResp := provider.ValidateProviderConfig(req)
	diags = diags.Append(validateResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))
	if diags.HasErrors() && config == nil {
		// If there isn't an explicit "provider" block in the configuration,
		// this error message won't be very clear. Add some detail to the error
		// message in this case.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid provider configuration",
			fmt.Sprintf(providerConfigErr, n.Addr.Provider),
		))
	}

	if diags.HasErrors() {
		return diags
	}

	// If the provider returns something different, log a warning to help
	// indicate to provider developers that the value is not used.
	preparedCfg := validateResp.PreparedConfig
	if preparedCfg != cty.NilVal && !preparedCfg.IsNull() && !preparedCfg.RawEquals(unmarkedConfigVal) {
		log.Printf("[WARN] ValidateProviderConfig from %q changed the config value, but that value is unused", n.Addr)
	}

	configDiags := ctx.ConfigureProvider(n.Addr, providerKey, unmarkedConfigVal)
	diags = diags.Append(configDiags.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))
	if diags.HasErrors() && config == nil {
		// If there isn't an explicit "provider" block in the configuration,
		// this error message won't be very clear. Add some detail to the error
		// message in this case.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid provider configuration",
			fmt.Sprintf(providerConfigErr, n.Addr.Provider),
		))
	}
	return diags
}

const providerConfigErr = `Provider %q requires explicit configuration. Add a provider block to the root module and configure the provider's required arguments as described in the provider documentation.
`
