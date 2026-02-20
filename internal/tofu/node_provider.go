// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/instances"
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

	instances map[addrs.InstanceKey]providers.Configured
}

var (
	_ GraphNodeExecutable = (*NodeApplyableProvider)(nil)
	_ GraphNodeProvider   = (*NodeApplyableProvider)(nil) // Partial, see NodeAbstractProvider
)

// GraphNodeProvider
func (n *NodeApplyableProvider) Instance(key addrs.InstanceKey) (providers.Configured, error) {
	if n.instances == nil {
		// Should never happen
		return nil, fmt.Errorf("bug: NodeApplyableProvider.Instance() called before Execute()")
	}
	instance, ok := n.instances[key]
	if !ok {
		return nil, fmt.Errorf("provider %s not initialized with key %s", n.Addr, key)
	}

	return instance, nil
}

// GraphNodeProvider
func (n *NodeApplyableProvider) Close(ctx context.Context) error {
	if n.instances == nil {
		// Should never happen
		return fmt.Errorf("bug: NodeApplyableProvider.Close() called before Execute()")
	}
	var errs []error
	for _, instance := range n.instances {
		errs = append(errs, instance.Close(ctx))
	}
	return errors.Join(errs...)
}

// GraphNodeExecutable
func (n *NodeApplyableProvider) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	n.instances = map[addrs.InstanceKey]providers.Configured{}

	var instanceData map[addrs.InstanceKey]instances.RepetitionData
	if n.Config == nil || n.Config.Instances == nil {
		// Stub out uninstanced
		instanceData = map[addrs.InstanceKey]instances.RepetitionData{addrs.NoKey: EvalDataForNoInstanceKey}
	} else {
		instanceData = n.Config.Instances
	}

	if op == walkValidate {
		log.Printf("[TRACE] NodeApplyableProvider: validating configuration for %s", n.Addr)
		instance, newDiags := evalCtx.Providers().NewProvider(ctx, n.Addr.Provider)
		n.instances[addrs.NoKey] = instance
		for key, data := range instanceData {
			diags = diags.Append(n.ValidateProvider(ctx, evalCtx, instance, key, data))
		}
		return diags.Append(newDiags)
	}

	verifyConfigIsKnown := op == walkImport
	if verifyConfigIsKnown {
		log.Printf("[TRACE] NodeApplyableProvider: configuring %s (requiring that configuration is wholly known)", n.Addr)
	} else {
		log.Printf("[TRACE] NodeApplyableProvider: configuring %s", n.Addr)
	}

	for key, data := range instanceData {
		diags = diags.Append(n.ConfigureProvider(ctx, evalCtx, key, data, verifyConfigIsKnown))
	}

	return diags
}

func (n *NodeApplyableProvider) ValidateProvider(ctx context.Context, evalCtx EvalContext, instance providers.Unconfigured, providerKey addrs.InstanceKey, data InstanceKeyEvalData) tfdiags.Diagnostics {
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

	schema, schemaDiags := evalCtx.Providers().GetProviderSchema(ctx, n.Addr.Provider)
	diags := schemaDiags.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
	if diags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return diags
	}

	configSchema := schema.Provider.Block
	if configSchema == nil {
		// Should never happen in real code, but often comes up in tests where
		// mock schemas are being used that tend to be incomplete.
		log.Printf("[WARN] ValidateProvider: no config schema is available for %s, so using empty schema", n.Addr)
		configSchema = &configschema.Block{}
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

	validateResp := instance.ValidateProviderConfig(ctx, req)
	diags = diags.Append(validateResp.Diagnostics.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))

	tracing.SetSpanError(span, diags)
	return diags
}

// ConfigureProvider configures a provider that is already initialized and retrieved.
// If verifyConfigIsKnown is true, ConfigureProvider will return an error if the
// provider configVal is not wholly known and is meant only for use during import.
func (n *NodeApplyableProvider) ConfigureProvider(ctx context.Context, evalCtx EvalContext, providerKey addrs.InstanceKey, data InstanceKeyEvalData, verifyConfigIsKnown bool) tfdiags.Diagnostics {
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
		instance, diags := evalCtx.Providers().NewProvider(ctx, n.Addr.Provider)
		n.instances[providerKey] = instance
		return diags
	}

	config := n.ProviderConfig()

	configBody := buildProviderConfig(ctx, evalCtx, n.Addr, config)

	schema, schemaDiags := evalCtx.Providers().GetProviderSchema(ctx, n.Addr.Provider)
	diags := schemaDiags.InConfigBody(configBody, n.Addr.InstanceString(providerKey))
	if diags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return diags
	}

	configSchema := schema.Provider.Block

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

	provider, newDiags := evalCtx.Providers().NewConfiguredProvider(ctx, n.Addr.Provider, configVal)
	diags = diags.Append(newDiags.InConfigBody(configBody, n.Addr.InstanceString(providerKey)))

	n.instances[providerKey] = provider

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
