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
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
)

// traceAttrProviderAddress is a standardized trace span attribute name that we
// use for recording the address of the main provider that a span is associated
// with.
//
// The value of this should be populated by calling the String method on
// a value of type [addrs.Provider].
const traceAttrProviderAddr = "opentofu.provider.source"

// traceAttrProviderConfigAddress is a standardized trace span attribute
// name that we use for recording the address of the main provider configuration
// block that a span is associated with.
//
// The value of this should be populated by calling the String method on
// a value of type [addrs.AbsProviderConfig].
const traceAttrProviderConfigAddr = "opentofu.provider_config.address"

// traceAttrProviderInstanceAddr is a standardized trace span attribute
// name that we use for recording the address of the main provider instance
// that a span is associated with.
//
// The value of this should be populated by calling traceProviderInstanceAddr
// with the [addrs.AbsProviderConfig] and [addrs.InstanceKey] value that
// together uniquely identify the provider instance.
const traceAttrProviderInstanceAddr = "opentofu.provider_instance.address"

// NodeApplyableProvider represents a configured provider.
type NodeApplyableProvider struct {
	*NodeAbstractProvider
}

var (
	_ GraphNodeExecutable = (*NodeApplyableProvider)(nil)
)

// GraphNodeExecutable
func (n *NodeApplyableProvider) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	instances, diags := n.initInstances(ctx, evalCtx, op)

	for key, provider := range instances {
		diags = diags.Append(n.executeInstance(ctx, evalCtx, op, key, provider))
	}

	return diags
}
func (n *NodeApplyableProvider) initInstances(ctx context.Context, evalCtx EvalContext, op walkOperation) (map[addrs.InstanceKey]providers.Interface, tfdiags.Diagnostics) {
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
		_, err := evalCtx.InitProvider(ctx, n.Addr, key)
		diags = diags.Append(err)
	}
	if diags.HasErrors() {
		return nil, diags
	}

	instances := make(map[addrs.InstanceKey]providers.Interface)
	for configKey, initKey := range instanceKeys {
		provider, _, err := getProvider(ctx, evalCtx, n.Addr, initKey)
		diags = diags.Append(err)
		instances[configKey] = provider
	}
	if diags.HasErrors() {
		return nil, diags
	}

	return instances, diags
}
func (n *NodeApplyableProvider) executeInstance(ctx context.Context, evalCtx EvalContext, op walkOperation, providerKey addrs.InstanceKey, provider providers.Interface) tfdiags.Diagnostics {
	switch op {
	case walkValidate:
		log.Printf("[TRACE] NodeApplyableProvider: validating configuration for %s", n.Addr)
		return n.ValidateProvider(ctx, evalCtx, providerKey, provider)
	case walkPlan, walkPlanDestroy, walkApply, walkDestroy:
		log.Printf("[TRACE] NodeApplyableProvider: configuring %s", n.Addr)
		return n.ConfigureProvider(ctx, evalCtx, providerKey, provider, false)
	case walkImport:
		log.Printf("[TRACE] NodeApplyableProvider: configuring %s (requiring that configuration is wholly known)", n.Addr)
		return n.ConfigureProvider(ctx, evalCtx, providerKey, provider, true)
	}
	return nil
}

func (n *NodeApplyableProvider) ValidateProvider(ctx context.Context, evalCtx EvalContext, providerKey addrs.InstanceKey, provider providers.Interface) tfdiags.Diagnostics {
	_, span := tracing.Tracer().Start(
		ctx, "Validate provider configuration",
		tracing.SpanAttributes(
			traceattrs.String(traceAttrProviderAddr, n.Addr.Provider.String()),
			traceattrs.String(traceAttrProviderConfigAddr, n.Addr.String()),
			traceattrs.String(traceAttrProviderInstanceAddr, traceProviderInstanceAddr(n.Addr, providerKey)),
		),
	)
	defer span.End()

	if n.Config != nil && n.Config.IsMocked {
		// Mocked for testing
		return nil
	}

	configBody := buildProviderConfig(ctx, evalCtx, n.Addr, n.ProviderConfig())

	// if a provider config is empty (only an alias), return early and don't continue
	// validation. validate doesn't need to fully configure the provider itself, so
	// skipping a provider with an implied configuration won't prevent other validation from completing.
	_, noConfigDiags := configBody.Content(&hcl.BodySchema{})
	if !noConfigDiags.HasErrors() {
		return nil
	}

	schemaResp := provider.GetProviderSchema(ctx)
	diags := schemaResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
	if diags.HasErrors() {
		tracing.SetSpanError(span, diags)
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

	configVal, _, evalDiags := evalCtx.EvaluateBlock(ctx, configBody, configSchema, nil, data)
	if evalDiags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return diags.Append(evalDiags)
	}
	diags = diags.Append(evalDiags)

	// If our config value contains any marked values, ensure those are
	// stripped out before sending this to the provider
	unmarkedConfigVal, _ := configVal.UnmarkDeep()

	req := providers.ValidateProviderConfigRequest{
		Config: unmarkedConfigVal,
	}

	validateResp := provider.ValidateProviderConfig(ctx, req)
	diags = diags.Append(validateResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))

	tracing.SetSpanError(span, diags)
	return diags
}

// ConfigureProvider configures a provider that is already initialized and retrieved.
// If verifyConfigIsKnown is true, ConfigureProvider will return an error if the
// provider configVal is not wholly known and is meant only for use during import.
func (n *NodeApplyableProvider) ConfigureProvider(ctx context.Context, evalCtx EvalContext, providerKey addrs.InstanceKey, provider providers.Interface, verifyConfigIsKnown bool) tfdiags.Diagnostics {
	_, span := tracing.Tracer().Start(
		ctx, "Configure provider",
		tracing.SpanAttributes(
			traceattrs.String(traceAttrProviderAddr, n.Addr.Provider.String()),
			traceattrs.String(traceAttrProviderConfigAddr, n.Addr.String()),
			traceattrs.String(traceAttrProviderInstanceAddr, traceProviderInstanceAddr(n.Addr, providerKey)),
		),
	)
	defer span.End()

	if n.Config != nil && n.Config.IsMocked {
		// Mocked for testing
		return nil
	}

	config := n.ProviderConfig()

	configBody := buildProviderConfig(ctx, evalCtx, n.Addr, config)

	resp := provider.GetProviderSchema(ctx)
	diags := resp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
	if diags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return diags
	}

	configSchema := resp.Provider.Block
	data := EvalDataForNoInstanceKey
	if n.Config != nil && n.Config.Instances != nil {
		data = n.Config.Instances[providerKey]
	}

	configVal, configBody, evalDiags := evalCtx.EvaluateBlock(ctx, configBody, configSchema, nil, data)
	diags = diags.Append(evalDiags)
	if evalDiags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return diags
	}

	if verifyConfigIsKnown && !configVal.IsWhollyKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration",
			Detail:   fmt.Sprintf("The configuration for %s depends on values that cannot be determined until apply.", n.Addr),
			Subject:  &config.DeclRange,
		})
		tracing.SetSpanError(span, diags)
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
		tracing.SetSpanError(span, diags)
		return diags
	}

	// If the provider returns something different, log a warning to help
	// indicate to provider developers that the value is not used.
	preparedCfg := validateResp.PreparedConfig
	if preparedCfg != cty.NilVal && !preparedCfg.IsNull() && !preparedCfg.RawEquals(unmarkedConfigVal) {
		log.Printf("[WARN] ValidateProviderConfig from %q changed the config value, but that value is unused", n.Addr)
	}

	configDiags := evalCtx.ConfigureProvider(ctx, n.Addr, providerKey, unmarkedConfigVal)
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
	tracing.SetSpanError(span, diags)
	return diags
}

const providerConfigErr = `Provider %q requires explicit configuration. Add a provider block to the root module and configure the provider's required arguments as described in the provider documentation.
`

// traceProviderInstanceAddr generates a value to be used for tracing attributes
// that refer to a specific instance of a provider configuration.
//
// This is here to compensate for the fact that we don't currently have an
// address type for provider instance addresses in package addrs, and instead
// just pass around loose config address and instance key values as separate
// arguments. If we do eventually have a suitable address type then this
// function should be removed and all uses of it replaced by calling the
// String method on that address type.
func traceProviderInstanceAddr(configAddr addrs.AbsProviderConfig, instKey addrs.InstanceKey) string {
	if instKey == addrs.NoKey {
		return configAddr.String()
	}
	return configAddr.String() + instKey.String()
}
