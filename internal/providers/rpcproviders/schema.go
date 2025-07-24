// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log"
	"maps"
	"math"
	"sync"

	"github.com/apparentlymart/opentofu-providers/tofuprovider"
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerops"
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerschema"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// GetProviderSchema implements providers.Interface.
func (r rpcProvider) GetProviderSchema(ctx context.Context) providers.GetProviderSchemaResponse {
	// Whenever someone calls this directly we'll ignore our existing cache
	// and re-fetch everything, but we'll use the result to update the
	// cache so other future calls can benefit.
	diags := r.schema.fetchEverything(ctx)
	if diags.HasErrors() {
		var resp providers.GetProviderSchemaResponse
		resp.Diagnostics = resp.Diagnostics.Append(diags)
		return resp
	}

	return r.schema.AsGetProviderSchemaResponse()
}

// GetFunctions implements providers.Interface.
func (r rpcProvider) GetFunctions(ctx context.Context) providers.GetFunctionsResponse {
	// Whenever someone calls this directly we'll ignore our existing cache
	// and re-fetch everything, but we'll use the result to update the
	// cache so other future calls can benefit.
	diags := r.schema.fetchFunctions(ctx)
	if diags.HasErrors() {
		var resp providers.GetFunctionsResponse
		resp.Diagnostics = resp.Diagnostics.Append(diags)
		return resp
	}

	return r.schema.AsGetFunctionsResponse()
}

// schemaCache retains whatever schema information we have obtained from the
// provider so far, as a read-through cache so we can avoid requesting the
// same information multiple times.
type schemaCache struct {
	// client is the provider client we'll use to fetch schema information
	// on request if that information is not already in the cache.
	client tofuprovider.Provider

	// mu must be locked whenever accessing any of the fields below
	mu sync.Mutex

	// serverCaps are the capabilities reported by the server
	serverCaps *providers.ServerCapabilities

	// Don't directly access the internals of this type. Use the methods
	// instead because we'll probably change the inner details of our
	// caching strategy over time as we extend the protocol to support
	// finer-grain requests.
	//
	// Currently our behavior is actually pretty coarse, with most request
	// causing us to fetch the entire provider schema at once and then
	// cache it all for future use. We hope to make this more granular
	// over time using extensions of the underlying protocol.

	// providerConfig is the schema for the provider configuration itself.
	// nil means that the cache has not yet been populated.
	providerConfig *providers.Schema

	// providerMeta is the schema to use for the "provider_meta" block in
	// conjunction with this provider.
	// nil means that the cache has not yet been populated.
	providerMeta *providers.Schema

	// These maps contain the schema for each resource type that the provider
	// offers for each resource mode.
	//
	// Due to the design of the underlying protocol each of these fields
	// is currently either entirely populated (any non-nil value) or nil
	// to represent that we've not made a request yet. If future protocol
	// versions allow fetching data in a more granular way then the rules
	// might change, so callers should always access these only indirectly
	// through the methods of schemaCache.
	managedResourceTypes, dataResourceTypes, ephemeralResourceTypes map[string]*providers.Schema

	// functions holds the signature for each function the provider offers.
	// If the map as a whole is nil that means we haven't yet asked the
	// provider which functions it supports. Once the map is non-nil the
	// keys represent the complete set of available functions, but individual
	// elements might be nil to represent that we've not yet retrieved the
	// signature information.
	functions map[string]*providers.FunctionSpec
}

// HasManagedResourceType returns true if the provider supports a managed
// resource type of the given name.
//
// Callers should prefer this over GetManagedResourceType if they only need
// to know whether the type is supported at all, because it might be answerable
// without fetching quite so much data from the underlying provider.
func (c *schemaCache) HasManagedResourceType(ctx context.Context, name string) (bool, tfdiags.Diagnostics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	diags := c.ensureManagedResourceTypes(ctx)
	_, ok := c.managedResourceTypes[name]
	return ok, diags
}

// HasDataResourceType returns true if the provider supports a data resource
// type of the given name.
//
// Callers should prefer this over GetDataResourceType if they only need
// to know whether the type is supported at all, because it might be answerable
// without fetching quite so much data from the underlying provider.
func (c *schemaCache) HasDataResourceType(ctx context.Context, name string) (bool, tfdiags.Diagnostics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	diags := c.ensureDataResourceTypes(ctx)
	_, ok := c.dataResourceTypes[name]
	return ok, diags
}

// HasEphemeralResourceType returns true if the provider supports an ephemeral
// resource type of the given name.
//
// Callers should prefer this over GetEphemeralResourceType if they only need
// to know whether the type is supported at all, because it might be answerable
// without fetching quite so much data from the underlying provider.
func (c *schemaCache) HasEphemeralResourceType(ctx context.Context, name string) (bool, tfdiags.Diagnostics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	diags := c.ensureEphemeralResourceTypes(ctx)
	_, ok := c.ephemeralResourceTypes[name]
	return ok, diags
}

// HasManagedResourceType returns true if the provider supports a function
// the given name.
//
// Callers should prefer this over GetFunction if they only need to know whether
// the function is supported at all, because it might be answerable without
// fetching quite so much data from the underlying provider.
func (c *schemaCache) HasFunction(ctx context.Context, name string) (bool, tfdiags.Diagnostics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	diags := c.ensureFunctions(ctx)
	_, ok := c.functions[name]
	return ok, diags
}

func (c *schemaCache) GetProviderConfig(ctx context.Context) (*providers.Schema, tfdiags.Diagnostics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var diags tfdiags.Diagnostics
	if c.providerConfig == nil {
		diags = diags.Append(c.fetchEverything(ctx))
	}
	return c.providerConfig, diags
}

func (c *schemaCache) GetFunction(ctx context.Context, name string) (*providers.FunctionSpec, tfdiags.Diagnostics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	diags := c.ensureFunctions(ctx)
	return c.functions[name], diags
}

func (c *schemaCache) ensureManagedResourceTypes(ctx context.Context) tfdiags.Diagnostics {
	if c.managedResourceTypes != nil {
		return nil // already cached
	}
	return c.fetchEverything(ctx)
}

func (c *schemaCache) ensureDataResourceTypes(ctx context.Context) tfdiags.Diagnostics {
	if c.dataResourceTypes != nil {
		return nil // already cached
	}
	return c.fetchEverything(ctx)
}

func (c *schemaCache) ensureEphemeralResourceTypes(ctx context.Context) tfdiags.Diagnostics {
	if c.ephemeralResourceTypes != nil {
		return nil // already cached
	}
	return c.fetchEverything(ctx)
}

func (c *schemaCache) ensureFunctions(ctx context.Context) tfdiags.Diagnostics {
	if c.functions != nil {
		return nil // already cached
	}
	// There is a separate granular method for fetching functions, so we
	// can avoid fetching anything else for now.
	return c.fetchFunctions(ctx)
}

// fetchEverything calls GetProviderSchema and then fully populates everything
// in our cache fields.
//
// Callers should call this only after checking whether the information they
// need is already cached, to avoid making redundant requests.
//
// Currently most methods end up here because the underlying protocol does
// not offer more granular schema request methods. We hope to use this less
// over time.
func (c *schemaCache) fetchEverything(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	resp, err := c.client.GetProviderSchema(ctx, &providerops.GetProviderSchemaRequest{})
	diags = appendDiags(diags, resp, err)
	if diags.HasErrors() {
		return diags
	}

	c.serverCaps = convertServerCapabililties(resp.ServerCapabilities())

	allSchema := resp.ProviderSchema()
	c.providerConfig, err = convertSchema(allSchema.ProviderConfigSchema())
	diags = appendConvertSchemaDiags(diags, err)
	c.providerMeta, err = convertSchema(allSchema.ProviderMetaSchema())
	diags = appendConvertSchemaDiags(diags, err)

	convertNamedSchema := func(name string, schema providerschema.Schema) (string, *providers.Schema) {
		ret, err := convertSchema(schema)
		diags = appendConvertSchemaDiags(diags, err)
		return name, ret
	}
	c.managedResourceTypes = maps.Collect(mapSeq2(allSchema.ManagedResourceTypeSchemas(), convertNamedSchema))
	c.dataResourceTypes = maps.Collect(mapSeq2(allSchema.DataResourceTypeSchemas(), convertNamedSchema))
	c.ephemeralResourceTypes = maps.Collect(mapSeq2(allSchema.EphemeralResourceTypeSchemas(), convertNamedSchema))

	c.functions, err = convertFunctionSignatures(allSchema.FunctionSignatures())
	diags = appendConvertSchemaDiags(diags, err)

	return diags
}

// fetchFunctions calls GetFunctions and uses the result to populate the
// "functions" field.
//
// Callers should call this only after checking whether the information they
// need is already cached, to avoid making redundant requests.
func (c *schemaCache) fetchFunctions(ctx context.Context) tfdiags.Diagnostics {
	// FIXME: OpenTofu chose to slightly tweak the provider protocol (as
	// compared to Terraform's reference implementation) by asking the
	// provider for its functions only after it's already configured so
	// that the provider configuration can potentially cause different
	// functions to be defined. That means it isn't actually valid to treat
	// the set of functions as part of the cache and so maybe we should
	// drop this altogether and just always fetch functions over again
	// whenever they are requested.
	//
	// For now we hack around this by forcing any existing cache to be
	// purged each time ConfigureProvider is called, but that's not
	// compatible with sharing the same cache object across multiple
	// instances of the same provider, so we'll need a different strategy
	// if we ever choose to do that.

	var diags tfdiags.Diagnostics
	resp, err := c.client.GetFunctions(ctx, &providerops.GetFunctionsRequest{})
	diags = appendDiags(diags, resp, err)
	if diags.HasErrors() {
		return diags
	}

	c.functions, err = convertFunctionSignatures(resp.FunctionSignatures())
	diags = appendConvertSchemaDiags(diags, err)

	return diags
}

func convertSchema(got providerschema.Schema) (*providers.Schema, error) {
	if got == nil {
		return nil, nil
	}
	ret := providers.Schema{
		Version: got.SchemaVersion(),
		Block:   convertSchemaBlockType(got),
	}
	if err := ret.Block.InternalValidate(); err != nil {
		return nil, err
	}
	return &ret, nil
}

func convertSchemaBlockType(got providerschema.BlockType) *configschema.Block {
	return &configschema.Block{
		Attributes: convertSchemaAttributes(got.Attributes()),
		BlockTypes: convertSchemaNestedBlockTypes(got.NestedBlockTypes()),
	}
}

func convertSchemaAttributes(attrs iter.Seq2[string, providerschema.Attribute]) map[string]*configschema.Attribute {
	if attrs == nil {
		return nil
	}
	return maps.Collect(mapSeq2(attrs, func(k string, v providerschema.Attribute) (string, *configschema.Attribute) {
		return k, convertSchemaAttribute(v)
	}))
}

func convertSchemaAttribute(attr providerschema.Attribute) *configschema.Attribute {
	ret := &configschema.Attribute{}
	if nTy := attr.NestedType(); nTy != nil {
		ret.NestedType = convertSchemaObjectType(nTy)
	} else if ty := attr.Type(); ty != nil {
		realTy, err := ty.AsCtyType()
		if err != nil {
			// An error here means that the provider returned something invalid,
			// so we'll just log it for now and then stub it out so that a
			// later call to InternalValidate can flag it as invalid.
			log.Printf("[ERROR] provider returned unparsable attribute type: %s", err)
			ret.Type = cty.NilType
		} else {
			ret.Type = realTy
		}
	} else {
		// If we get here then the provider's response is invalid.
		log.Printf("[ERROR] provider returned attribute with no type")
	}

	switch attr.Usage() {
	case providerschema.AttributeRequired:
		ret.Required = true
	case providerschema.AttributeOptional:
		ret.Optional = true
	case providerschema.AttributeOptionalComputed:
		ret.Optional = true
		ret.Computed = true
	case providerschema.AttributeComputed:
		ret.Computed = true
	default:
		// This'll get caught upstream by InternalValidate noticing that
		// the flag fields are not set in a valid combination.
		log.Printf("[ERROR] provider has unsupported attribute usage")
	}

	ret.Deprecated = attr.IsDeprecated()
	ret.Sensitive = attr.IsSensitive()
	// TODO: ret.WriteOnly, once it exists

	desc, descFormat := attr.DocDescription()
	ret.Description = desc
	ret.DescriptionKind = convertDocStringFormatToStringKind(descFormat)

	return ret
}

func convertSchemaObjectType(oTy providerschema.ObjectType) *configschema.Object {
	return &configschema.Object{
		Attributes: convertSchemaAttributes(oTy.Attributes()),
		Nesting:    convertSchemaNestingMode(oTy.Nesting()),
	}
}

func convertSchemaNestedBlockTypes(blockTypes iter.Seq2[string, providerschema.NestedBlockType]) map[string]*configschema.NestedBlock {
	if blockTypes == nil {
		return nil
	}
	return maps.Collect(mapSeq2(blockTypes, func(k string, v providerschema.NestedBlockType) (string, *configschema.NestedBlock) {
		return k, convertSchemaNestedBlockType(v)
	}))
}

func convertSchemaNestedBlockType(blockType providerschema.NestedBlockType) *configschema.NestedBlock {
	innerBlock := convertSchemaBlockType(blockType)
	minItems, maxItems := blockType.ItemLimits()
	if minItems > int64(math.MaxInt) {
		minItems = math.MaxInt // saturate (minItems being anything other than zero or one is meaningless in practice)
	}
	if maxItems > int64(math.MaxInt) {
		maxItems = math.MaxInt // saturate (okay because we can't produce a collection with more than MaxInt elements)
	}
	return &configschema.NestedBlock{
		Nesting:  convertSchemaNestingMode(blockType.Nesting()),
		Block:    *innerBlock,
		MinItems: int(minItems),
		MaxItems: int(maxItems),
	}
}

func convertSchemaNestingMode(mode providerschema.NestingMode) configschema.NestingMode {
	switch mode {
	case providerschema.NestingSingle:
		return configschema.NestingSingle
	case providerschema.NestingGroup:
		return configschema.NestingGroup
	case providerschema.NestingList:
		return configschema.NestingList
	case providerschema.NestingSet:
		return configschema.NestingSet
	case providerschema.NestingMap:
		return configschema.NestingMap
	default:
		// The zero value of configschema.NestingMode represents an invalid
		// nesting mode, but it doesn't have an exported symbol.
		var zero configschema.NestingMode
		return zero
	}
}

func convertFunctionSignatures(sigs iter.Seq2[string, providerschema.FunctionSignature]) (map[string]*providers.FunctionSpec, error) {
	if sigs == nil {
		return nil, nil
	}
	ret := make(map[string]*providers.FunctionSpec)
	var err error
	for name, sig := range sigs {
		retSig, sigErr := convertFunctionSignature(sig)
		err = errors.Join(err, sigErr)
		ret[name] = retSig
	}
	return ret, err
}

func convertFunctionSignature(sig providerschema.FunctionSignature) (*providers.FunctionSpec, error) {
	retType, err := sig.ResultType().AsCtyType()
	if err != nil {
		return nil, fmt.Errorf("invalid function return type: %w", err)
	}

	docDesc, docDescFormat := sig.DocDescription()

	var posParams []providers.FunctionParameterSpec
	if params := sig.Parameters(); params != nil {
		for param := range params {
			posParam, err := convertFunctionParameter(param)
			if err != nil {
				return nil, fmt.Errorf("invalid function parameter: %w", err)
			}
			posParams = append(posParams, posParam)
		}
	}

	var varParam *providers.FunctionParameterSpec
	if param := sig.VariadicParameter(); param != nil {
		varParamV, err := convertFunctionParameter(param)
		if err != nil {
			return nil, fmt.Errorf("invalid function variadic parameter: %w", err)
		}
		varParam = &varParamV
	}

	return &providers.FunctionSpec{
		Parameters:        posParams,
		VariadicParameter: varParam,
		Return:            retType,

		Summary:            sig.DocSummary(),
		Description:        docDesc,
		DescriptionFormat:  convertDocStringFormatToTextFormatting(docDescFormat),
		DeprecationMessage: sig.DeprecationMessage(),
	}, nil
}

func convertFunctionParameter(param providerschema.FunctionParameter) (providers.FunctionParameterSpec, error) {
	ty, err := param.Type().AsCtyType()
	if err != nil {
		return providers.FunctionParameterSpec{}, fmt.Errorf("invalid parameter type: %w", err)
	}

	docDesc, docDescFormat := param.DocDescription()

	return providers.FunctionParameterSpec{
		Name:               param.Name(),
		Type:               ty,
		AllowNullValue:     param.NullValueAllowed(),
		AllowUnknownValues: param.UnknownValuesAllowed(),
		Description:        docDesc,
		DescriptionFormat:  convertDocStringFormatToTextFormatting(docDescFormat),
	}, nil
}

func convertDocStringFormatToTextFormatting(docDescFormat providerschema.DocStringFormat) providers.TextFormatting {
	switch docDescFormat {
	case providerschema.DocStringPlain:
		return providers.TextFormattingPlain
	case providerschema.DocStringMarkdown:
		return providers.TextFormattingMarkdown
	default:
		var zero providers.TextFormatting
		return zero
	}
}

func convertDocStringFormatToStringKind(docDescFormat providerschema.DocStringFormat) configschema.StringKind {
	switch docDescFormat {
	case providerschema.DocStringPlain:
		return configschema.StringPlain
	case providerschema.DocStringMarkdown:
		return configschema.StringMarkdown
	default:
		var zero configschema.StringKind
		return zero
	}
}

func (c *schemaCache) InvalidateCachedFunctions() {
	c.mu.Lock()
	c.functions = nil
	c.mu.Unlock()
}

func (c *schemaCache) AsGetProviderSchemaResponse() providers.GetProviderSchemaResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	ret := providers.GetProviderSchemaResponse{}
	if c.serverCaps != nil {
		ret.ServerCapabilities = *c.serverCaps
	}
	if c.providerConfig != nil {
		ret.Provider = *c.providerConfig
	}
	if c.providerMeta != nil {
		ret.ProviderMeta = *c.providerMeta
	}
	ret.ResourceTypes = make(map[string]providers.Schema, len(c.managedResourceTypes))
	for name, schema := range c.managedResourceTypes {
		if schema != nil {
			ret.ResourceTypes[name] = *schema
		}
	}
	ret.DataSources = make(map[string]providers.Schema, len(c.dataResourceTypes))
	for name, schema := range c.dataResourceTypes {
		if schema != nil {
			ret.DataSources[name] = *schema
		}
	}
	ret.Functions = make(map[string]providers.FunctionSpec, len(c.functions))
	for name, spec := range c.functions {
		if spec != nil {
			ret.Functions[name] = *spec
		}
	}
	return ret
}

func (c *schemaCache) AsGetFunctionsResponse() providers.GetFunctionsResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	ret := providers.GetFunctionsResponse{}
	ret.Functions = make(map[string]providers.FunctionSpec, len(c.functions))
	for name, spec := range c.functions {
		if spec != nil {
			ret.Functions[name] = *spec
		}
	}
	return ret
}
