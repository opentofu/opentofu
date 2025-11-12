// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"context"
	"time"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Unconfigured represents a provider plugin that has not yet been configured. It has
// limited functionality that must not depend on ConfigureProvider having been called.
type Unconfigured interface {
	// GetMetadata is not yet implemented or used at this time. It may
	// be used in the future to avoid loading a provider's full schema
	// for initial validation. This could result in some potential
	// memory savings.

	// GetSchema returns the complete schema for the provider.
	GetProviderSchema(context.Context) GetProviderSchemaResponse

	// ValidateProviderConfig allows the provider to validate the configuration.
	// The ValidateProviderConfigResponse.PreparedConfig field is unused. The
	// final configuration is not stored in the state, and any modifications
	// that need to be made must be made during the Configure method call.
	ValidateProviderConfig(context.Context, ValidateProviderConfigRequest) ValidateProviderConfigResponse

	// ValidateResourceConfig allows the provider to validate the resource
	// configuration values.
	ValidateResourceConfig(context.Context, ValidateResourceConfigRequest) ValidateResourceConfigResponse

	// ValidateDataResourceConfig allows the provider to validate the data source
	// configuration values.
	ValidateDataResourceConfig(context.Context, ValidateDataResourceConfigRequest) ValidateDataResourceConfigResponse

	// ValidateEphemeralConfig allows the provider to validate the ephemeral resource
	// configuration values.
	ValidateEphemeralConfig(context.Context, ValidateEphemeralConfigRequest) ValidateEphemeralConfigResponse

	// MoveResourceState requests that the given resource data be moved from one
	// type to another, potentially between providers as well.
	MoveResourceState(context.Context, MoveResourceStateRequest) MoveResourceStateResponse

	// CallFunction requests that the given function is called and response returned.
	// There is a bit of a quirk in OpenTofu-land.  We allow providers to supply
	// additional functions via GetFunctions() after configuration.  Those functions
	// will only be available via CallFunction after ConfigureProvider is called.
	CallFunction(context.Context, CallFunctionRequest) CallFunctionResponse

	// Configure configures and initialized the provider.
	ConfigureProvider(context.Context, ConfigureProviderRequest) ConfigureProviderResponse

	// Close shuts down the plugin process if applicable.
	Close(context.Context) error

	// Stop is called when the provider should halt any in-flight actions.
	//
	// Stop should not block waiting for in-flight actions to complete. It
	// should take any action it wants and return immediately acknowledging it
	// has received the stop request. OpenTofu will not make any further API
	// calls to the provider after Stop is called.
	//
	// The given context is guaranteed not to be cancelled and to have no
	// deadline, but the contexts visible to other provider methods
	// running concurrently might be cancelled either before or after
	// Stop call. Any provider other operations that need to be able to continue
	// when reacting to Stop must use [context.WithoutCancel], or equivalent,
	// to insulate themselves from any incoming cancellation/deadline signals.
	//
	// The error returned, if non-nil, is assumed to mean that signaling the
	// stop somehow failed and that the user should expect potentially waiting
	// a longer period of time.
	Stop(context.Context) error
}

// Configured represents a provider plugin that has been configured. It has additional
// functionallity on top of the Unconfigured interface that depends on ConfigureProvider
// having been called.
type Configured interface {
	// A configured provider can do anything a unconfigured provider can.
	Unconfigured

	// UpgradeResourceState is called when the state loader encounters an
	// instance state whose schema version is less than the one reported by the
	// currently-used version of the corresponding provider, and the upgraded
	// result is used for any further processing.
	UpgradeResourceState(context.Context, UpgradeResourceStateRequest) UpgradeResourceStateResponse

	// ReadResource refreshes a resource and returns its current state.
	ReadResource(context.Context, ReadResourceRequest) ReadResourceResponse

	// PlanResourceChange takes the current state and proposed state of a
	// resource, and returns the planned final state.
	PlanResourceChange(context.Context, PlanResourceChangeRequest) PlanResourceChangeResponse

	// ApplyResourceChange takes the planned state for a resource, which may
	// yet contain unknown computed values, and applies the changes returning
	// the final state.
	//
	// NOTE: the context passed to this method can potentially be cancelled,
	// and so any cancel-sensitive operation that needs to be able to complete
	// gracefully should use [context.WithoutCancel] to create a new context
	// disconnected from the incoming cancellation chain. The caller doesn't
	// do this automatically to give implementations flexibility to use a
	// mixture of both cancelable and non-cancelable requests.
	ApplyResourceChange(context.Context, ApplyResourceChangeRequest) ApplyResourceChangeResponse

	// ImportResourceState requests that the given resource be imported.
	ImportResourceState(context.Context, ImportResourceStateRequest) ImportResourceStateResponse

	// ReadDataSource returns the data source's current state.
	ReadDataSource(context.Context, ReadDataSourceRequest) ReadDataSourceResponse

	// OpenEphemeralResource opens the provided ephemeral resource.
	// This is meant to return the following:
	// * the ephemeral information that will be used in other ephemeral contexts.
	//   The OpenEphemeralResourceResponse.Result is meant to be used all the time it's requested
	//   but this information will not be changed if the Renew will be called.
	//   Renew is meant to be supported only by a limited number of providers where the actual
	//   information from Result renewed by updating a remote state (eg: Vault/OpenBao)
	// * internal private information that needs to be used for future Renew/Close calls.
	// * a timestamp that will be used to determine if and when Renew call will be performed.
	// * deferred information containing a reason returned by the provider. This will be used to
	//   determine if the resource needs to be deferred or not.
	OpenEphemeralResource(context.Context, OpenEphemeralResourceRequest) OpenEphemeralResourceResponse

	// RenewEphemeralResource is renewing the information related to the OpenEphemeralResourceResponse.Result returned by
	// the OpenEphemeralResource.
	// The request is using the private information from the OpenEphemeralResourceResponse.Private
	// to enable the provider to perform this action.
	// The information returned in RenewEphemeralResourceResponse.Private needs to be used in any future call to
	// Renew/Close.
	RenewEphemeralResource(context.Context, RenewEphemeralResourceRequest) (resp RenewEphemeralResourceResponse)

	// CloseEphemeralResource closes the provided ephemeral resource.
	// This requires the information from OpenEphemeralResourceResponse.Private or RenewEphemeralResourceResponse.Private
	// to succeed.
	CloseEphemeralResource(context.Context, CloseEphemeralResourceRequest) CloseEphemeralResourceResponse

	// GetFunctions returns a full list of functions defined in this provider. It should be a super
	// set of the functions returned in GetProviderSchema()
	GetFunctions(context.Context) GetFunctionsResponse
}

// Interface represents the set of methods required for a complete resource
// provider plugin. Longer term, we could remove this interface in favor of it's
// component parts (Unconfigured, Configured) for added safety and clarity.
type Interface interface {
	Configured
}

// GetProviderSchemaResponse is the return type for GetProviderSchema, and
// should only be used when handling a value for that method. The handling of
// of schemas in any other context should always use ProviderSchema, so that
// the in-memory representation can be more easily changed separately from the
// RCP protocol.
type GetProviderSchemaResponse struct {
	// Provider is the schema for the provider itself.
	Provider Schema

	// ProviderMeta is the schema for the provider's meta info in a module
	ProviderMeta Schema

	// ResourceTypes map the resource type name to that type's schema.
	ResourceTypes map[string]Schema

	// DataSources maps the data source name to that data source's schema.
	DataSources map[string]Schema

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics

	// ServerCapabilities lists optional features supported by the provider.
	ServerCapabilities ServerCapabilities

	// Functions lists all functions supported by this provider.
	Functions map[string]FunctionSpec

	// EphemeralResources maps the ephemeral type name to that type's schema.
	EphemeralResources map[string]Schema
}

// Schema pairs a provider or resource schema with that schema's version.
// This is used to be able to upgrade the schema in UpgradeResourceState.
//
// This describes the schema for a single object within a provider. Type
// "Schemas" (plural) instead represents the overall collection of schemas
// for everything within a particular provider.
type Schema struct {
	Version int64
	Block   *configschema.Block
}

// ServerCapabilities allows providers to communicate extra information
// regarding supported protocol features. This is used to indicate availability
// of certain forward-compatible changes which may be optional in a major
// protocol version, but cannot be tested for directly.
type ServerCapabilities struct {
	// PlanDestroy signals that this provider expects to receive a
	// PlanResourceChange call for resources that are to be destroyed.
	PlanDestroy bool

	// The GetProviderSchemaOptional capability indicates that this
	// provider does not require calling GetProviderSchema to operate
	// normally, and the caller can used a cached copy of the provider's
	// schema.
	// In other words, the providers for which GetProviderSchemaOptional is false
	// require their schema to be read after EVERY instantiation to function normally.
	GetProviderSchemaOptional bool
}

type FunctionSpec struct {
	// List of parameters required to call the function
	Parameters []FunctionParameterSpec
	// Optional Spec for variadic parameters
	VariadicParameter *FunctionParameterSpec
	// Type which the function will return
	Return cty.Type
	// Human-readable shortened documentation for the function
	Summary string
	// Human-readable documentation for the function
	Description string
	// Formatting type of the Description field
	DescriptionFormat TextFormatting
	// Human-readable message present if the function is deprecated
	DeprecationMessage string
}

type FunctionParameterSpec struct {
	// Human-readable display name for the parameter
	Name string
	// Type constraint for the parameter
	Type cty.Type
	// Null values allowed for the parameter
	AllowNullValue bool
	// Unknown Values allowed for the parameter
	// Individual provider implementations may interpret this as a
	// check using IsWhollyKnown instead cty's default of IsKnown.
	// If the input is not wholly known, the result should be
	// cty.UnknownVal(spec.returnType)
	AllowUnknownValues bool
	// Human-readable documentation for the parameter
	Description string
	// Formatting type of the Description field
	DescriptionFormat TextFormatting
}

type TextFormatting string

const TextFormattingPlain = TextFormatting("Plain")
const TextFormattingMarkdown = TextFormatting("Markdown")

type ValidateProviderConfigRequest struct {
	// Config is the raw configuration value for the provider.
	Config cty.Value
}

type ValidateProviderConfigResponse struct {
	// PreparedConfig is unused and will be removed with support for plugin protocol v5.
	PreparedConfig cty.Value
	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type ValidateResourceConfigRequest struct {
	// TypeName is the name of the resource type to validate.
	TypeName string

	// Config is the configuration value to validate, which may contain unknown
	// values.
	Config cty.Value
}

type ValidateResourceConfigResponse struct {
	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type ValidateDataResourceConfigRequest struct {
	// TypeName is the name of the data source type to validate.
	TypeName string

	// Config is the configuration value to validate, which may contain unknown
	// values.
	Config cty.Value
}

type ValidateDataResourceConfigResponse struct {
	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type ValidateEphemeralConfigRequest struct {
	// TypeName is the name of the ephemeral resource type to validate.
	TypeName string

	// Config is the configuration value to validate, which may contain unknown
	// values.
	Config cty.Value
}

type ValidateEphemeralConfigResponse struct {
	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type UpgradeResourceStateRequest struct {
	// TypeName is the name of the resource type being upgraded
	TypeName string

	// Version is version of the schema that created the current state.
	Version int64

	// RawStateJSON and RawStateFlatmap contain the state that needs to be
	// upgraded to match the current schema version. Because the schema is
	// unknown, this contains only the raw data as stored in the state.
	// RawStateJSON is the current json state encoding.
	// RawStateFlatmap is the legacy flatmap encoding.
	// Only on of these fields may be set for the upgrade request.
	RawStateJSON    []byte
	RawStateFlatmap map[string]string
}

type UpgradeResourceStateResponse struct {
	// UpgradedState is the newly upgraded resource state.
	UpgradedState cty.Value

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type ConfigureProviderRequest struct {
	// OpenTofu version is the version string from the running instance of
	// tofu. Providers can use TerraformVersion to verify compatibility,
	// and to store for informational purposes.
	TerraformVersion string

	// Config is the complete configuration value for the provider.
	Config cty.Value
}

type ConfigureProviderResponse struct {
	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type ReadResourceRequest struct {
	// TypeName is the name of the resource type being read.
	TypeName string

	// PriorState contains the previously saved state value for this resource.
	PriorState cty.Value

	// Private is an opaque blob that will be stored in state along with the
	// resource. It is intended only for interpretation by the provider itself.
	Private []byte

	// ProviderMeta is the configuration for the provider_meta block for the
	// module and provider this resource belongs to. Its use is defined by
	// each provider, and it should not be used without coordination with
	// HashiCorp. It is considered experimental and subject to change.
	ProviderMeta cty.Value
}

type ReadResourceResponse struct {
	// NewState contains the current state of the resource.
	NewState cty.Value

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics

	// Private is an opaque blob that will be stored in state along with the
	// resource. It is intended only for interpretation by the provider itself.
	Private []byte
}

type PlanResourceChangeRequest struct {
	// TypeName is the name of the resource type to plan.
	TypeName string

	// PriorState is the previously saved state value for this resource.
	PriorState cty.Value

	// ProposedNewState is the expected state after the new configuration is
	// applied. This is created by directly applying the configuration to the
	// PriorState. The provider is then responsible for applying any further
	// changes required to create the proposed final state.
	ProposedNewState cty.Value

	// Config is the resource configuration, before being merged with the
	// PriorState. Any value not explicitly set in the configuration will be
	// null. Config is supplied for reference, but Provider implementations
	// should prefer the ProposedNewState in most circumstances.
	Config cty.Value

	// PriorPrivate is the previously saved private data returned from the
	// provider during the last apply.
	PriorPrivate []byte

	// ProviderMeta is the configuration for the provider_meta block for the
	// module and provider this resource belongs to. Its use is defined by
	// each provider, and it should not be used without coordination with
	// HashiCorp. It is considered experimental and subject to change.
	ProviderMeta cty.Value
}

type PlanResourceChangeResponse struct {
	// PlannedState is the expected state of the resource once the current
	// configuration is applied.
	PlannedState cty.Value

	// RequiresReplace is the list of the attributes that are requiring
	// resource replacement.
	RequiresReplace []cty.Path

	// PlannedPrivate is an opaque blob that is not interpreted by tofu
	// core. This will be saved and relayed back to the provider during
	// ApplyResourceChange.
	PlannedPrivate []byte

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics

	// LegacyTypeSystem is set only if the provider is using the legacy SDK
	// whose type system cannot be precisely mapped into the OpenTofu type
	// system. We use this to bypass certain consistency checks that would
	// otherwise fail due to this imprecise mapping. No other provider or SDK
	// implementation is permitted to set this.
	LegacyTypeSystem bool
}

type ApplyResourceChangeRequest struct {
	// TypeName is the name of the resource type being applied.
	TypeName string

	// PriorState is the current state of resource.
	PriorState cty.Value

	// Planned state is the state returned from PlanResourceChange, and should
	// represent the new state, minus any remaining computed attributes.
	PlannedState cty.Value

	// Config is the resource configuration, before being merged with the
	// PriorState. Any value not explicitly set in the configuration will be
	// null. Config is supplied for reference, but Provider implementations
	// should prefer the PlannedState in most circumstances.
	Config cty.Value

	// PlannedPrivate is the same value as returned by PlanResourceChange.
	PlannedPrivate []byte

	// ProviderMeta is the configuration for the provider_meta block for the
	// module and provider this resource belongs to. Its use is defined by
	// each provider, and it should not be used without coordination with
	// HashiCorp. It is considered experimental and subject to change.
	ProviderMeta cty.Value
}

type ApplyResourceChangeResponse struct {
	// NewState is the new complete state after applying the planned change.
	// In the event of an error, NewState should represent the most recent
	// known state of the resource, if it exists.
	NewState cty.Value

	// Private is an opaque blob that will be stored in state along with the
	// resource. It is intended only for interpretation by the provider itself.
	Private []byte

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics

	// LegacyTypeSystem is set only if the provider is using the legacy SDK
	// whose type system cannot be precisely mapped into the OpenTofu type
	// system. We use this to bypass certain consistency checks that would
	// otherwise fail due to this imprecise mapping. No other provider or SDK
	// implementation is permitted to set this.
	LegacyTypeSystem bool
}

type ImportResourceStateRequest struct {
	// TypeName is the name of the resource type to be imported.
	TypeName string

	// ID is a string with which the provider can identify the resource to be
	// imported.
	ID string
}

type ImportResourceStateResponse struct {
	// ImportedResources contains one or more state values related to the
	// imported resource. It is not required that these be complete, only that
	// there is enough identifying information for the provider to successfully
	// update the states in ReadResource.
	ImportedResources []ImportedResource

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

// ImportedResource represents an object being imported into OpenTofu with the
// help of a provider. An ImportedObject is a RemoteObject that has been read
// by the provider's import handler but hasn't yet been committed to state.
type ImportedResource struct {
	// TypeName is the name of the resource type associated with the
	// returned state. It's possible for providers to import multiple related
	// types with a single import request.
	TypeName string

	// State is the state of the remote object being imported. This may not be
	// complete, but must contain enough information to uniquely identify the
	// resource.
	State cty.Value

	// Private is an opaque blob that will be stored in state along with the
	// resource. It is intended only for interpretation by the provider itself.
	Private []byte
}

// AsInstanceObject converts the receiving ImportedObject into a
// ResourceInstanceObject that has status ObjectReady.
//
// The returned object does not know its own resource type, so the caller must
// retain the ResourceType value from the source object if this information is
// needed.
//
// The returned object also has no dependency addresses, but the caller may
// freely modify the direct fields of the returned object without affecting
// the receiver.
func (ir ImportedResource) AsInstanceObject() *states.ResourceInstanceObject {
	return &states.ResourceInstanceObject{
		Status:  states.ObjectReady,
		Value:   ir.State,
		Private: ir.Private,
	}
}

type MoveResourceStateRequest struct {
	// The address of the provider the resource is being moved from.
	SourceProviderAddress string
	// The resource type that the resource is being moved from.
	SourceTypeName string
	// The schema version of the resource type that the resource is being
	// moved from.
	SourceSchemaVersion uint64
	// The raw state of the resource being moved. Only the json field is
	// populated, as there should be no legacy providers using the flatmap
	// format that support newly introduced RPCs.
	SourceStateJSON    []byte
	SourceStateFlatmap map[string]string // Unused

	// The private state of the resource being moved.
	SourcePrivate []byte

	// The resource type that the resource is being moved to.
	TargetTypeName string
}

type MoveResourceStateResponse struct {
	// The state of the resource after it has been moved.
	TargetState cty.Value
	// The private state of the resource after it has been moved.
	TargetPrivate []byte

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type ReadDataSourceRequest struct {
	// TypeName is the name of the data source type to Read.
	TypeName string

	// Config is the complete configuration for the requested data source.
	Config cty.Value

	// ProviderMeta is the configuration for the provider_meta block for the
	// module and provider this resource belongs to. Its use is defined by
	// each provider, and it should not be used without coordination with
	// HashiCorp. It is considered experimental and subject to change.
	ProviderMeta cty.Value
}

type ReadDataSourceResponse struct {
	// State is the current state of the requested data source.
	State cty.Value

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type OpenEphemeralResourceRequest struct {
	// TypeName is the name of the ephemeral resource type to Open.
	TypeName string

	// Config is the complete configuration for the requested ephemeral resource.
	Config cty.Value
}

type OpenEphemeralResourceResponse struct {
	// Result will contain the ephemeral information returned by the ephemeral resource.
	Result cty.Value
	// Private is the provider information that needs to be used later on Renew/Close call.
	Private []byte
	// Deferred returns only a reason of why the provider is asking deferring the opening.
	Deferred *EphemeralResourceDeferred
	// RenewAt indicates if(!=nil) and when(<=time.Now()) the Renew call needs to be performed.
	RenewAt *time.Time

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type EphemeralResourceDeferred struct {
	DeferralReason DeferralReason
}

type RenewEphemeralResourceRequest struct {
	// TypeName is the name of the ephemeral resource to Renew.
	TypeName string

	// Private should be the same with the one from the last call on Open/Renew call.
	Private []byte
}

type RenewEphemeralResourceResponse struct {
	// Private needs to be used for the next call on Renew/Close
	Private []byte
	// RenewAt indicates if(!=nil) and when(<=time.Now()) the Renew call needs to be performed.
	RenewAt *time.Time

	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type CloseEphemeralResourceRequest struct {
	// TypeName is the name of the ephemeral resource to Close.
	TypeName string

	// Private should be the same with the one from the last call on Open/Renew call.
	Private []byte
}

type CloseEphemeralResourceResponse struct {
	// Diagnostics contains any warnings or errors from the method call.
	Diagnostics tfdiags.Diagnostics
}

type GetFunctionsResponse struct {
	Functions map[string]FunctionSpec

	Diagnostics tfdiags.Diagnostics
}

type CallFunctionRequest struct {
	Name      string
	Arguments []cty.Value
}

type CallFunctionResponse struct {
	Result cty.Value
	Error  error
}

type CallFunctionArgumentError struct {
	Text             string
	FunctionArgument int
}

func (err *CallFunctionArgumentError) Error() string {
	return err.Text
}
