// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"encoding/json"
)

// JSONPlan is the top-level representation of the json format of a plan. It includes
// the complete config and current state.
type JSONPlan struct {
	FormatVersion    string                 `json:"format_version,omitempty"`
	TerraformVersion string                 `json:"terraform_version,omitempty"`
	Variables        map[string]*JSONVariable   `json:"variables,omitempty"`
	PlannedValues    JSONStateValues        `json:"planned_values,omitempty"`
	// ResourceDrift and ResourceChanges are sorted in a user-friendly order
	// that is undefined at this time, but consistent.
	ResourceDrift      []JSONResourceChange  `json:"resource_drift,omitempty"`
	ResourceChanges    []JSONResourceChange  `json:"resource_changes,omitempty"`
	OutputChanges      map[string]JSONChange `json:"output_changes,omitempty"`
	PriorState         json.RawMessage   `json:"prior_state,omitempty"`
	Config             json.RawMessage   `json:"configuration,omitempty"`
	RelevantAttributes []JSONResourceAttr    `json:"relevant_attributes,omitempty"`
	Checks             json.RawMessage   `json:"checks,omitempty"`
	Timestamp          string            `json:"timestamp,omitempty"`
	Errored            bool              `json:"errored"`
	MiddlewareMetadata map[string]map[string]interface{} `json:"middleware_metadata,omitempty"`
}

// JSONResourceAttr contains the address and attribute of an external for the
// RelevantAttributes in the plan.
type JSONResourceAttr struct {
	Resource string          `json:"resource"`
	Attr     json.RawMessage `json:"attribute"`
}

// JSONChange is the representation of a proposed change for an object.
type JSONChange struct {
	// Actions are the actions that will be taken on the object selected by the
	// properties below.
	Actions []string `json:"actions,omitempty"`

	// Before and After are representations of the object value both before and
	// after the action.
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`

	// AfterUnknown is an object value with similar structure to After, but
	// with all unknown leaf values replaced with true, and all known leaf
	// values omitted.
	AfterUnknown json.RawMessage `json:"after_unknown,omitempty"`

	// BeforeSensitive and AfterSensitive are object values with similar
	// structure to Before and After, but with all sensitive leaf values
	// replaced with true, and all non-sensitive leaf values omitted.
	BeforeSensitive json.RawMessage `json:"before_sensitive,omitempty"`
	AfterSensitive  json.RawMessage `json:"after_sensitive,omitempty"`

	// ReplacePaths is an array of arrays representing a set of paths into the
	// object value which resulted in the action being "replace".
	ReplacePaths json.RawMessage `json:"replace_paths,omitempty"`

	// Importing contains the import metadata about this operation.
	Importing *JSONImporting `json:"importing,omitempty"`

	// GeneratedConfig contains any HCL config generated for this resource
	// during planning as a string.
	GeneratedConfig string `json:"generated_config,omitempty"`
}

// JSONImporting is a nested object for the resource import metadata.
type JSONImporting struct {
	// The original ID of this resource used to target it as part of planned
	// import operation.
	ID string `json:"id,omitempty"`
}

type JSONVariable struct {
	Value json.RawMessage `json:"value,omitempty"`
}

// JSONResourceChange is the representation of a resource change in the plan
type JSONResourceChange struct {
	// Address is the absolute resource address
	Address string `json:"address,omitempty"`

	// PreviousAddress is the absolute address that this resource instance had
	// at the conclusion of a previous run.
	PreviousAddress string `json:"previous_address,omitempty"`

	// ModuleAddress is the module portion of the above address. Omitted if the
	// instance is in the root module.
	ModuleAddress string `json:"module_address,omitempty"`

	// "managed" or "data"
	Mode string `json:"mode,omitempty"`

	Type         string          `json:"type,omitempty"`
	Name         string          `json:"name,omitempty"`
	Index        json.RawMessage `json:"index,omitempty"`
	ProviderName string          `json:"provider_name,omitempty"`

	// "deposed", if set, indicates that this action applies to a "deposed"
	// object of the given instance rather than to its "current" object.
	Deposed string `json:"deposed,omitempty"`

	// Change describes the change that will be made to this object
	Change JSONChange `json:"change,omitempty"`

	// ActionReason is a keyword representing some optional extra context
	// for why the actions given inside Change.Actions were chosen.
	ActionReason string `json:"action_reason,omitempty" `
}

// JSONStateValues is the JSON representation of the values of a state
type JSONStateValues struct {
	Outputs    map[string]JSONOutput `json:"outputs,omitempty"`
	RootModule *JSONModule          `json:"root_module,omitempty"`
}

type JSONOutput struct {
	Sensitive  bool            `json:"sensitive"`
	Type       json.RawMessage `json:"type,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Deprecated string          `json:"deprecated,omitempty"`
}

type JSONModule struct {
	Resources    []JSONResource `json:"resources,omitempty"`
	ChildModules []JSONModule   `json:"child_modules,omitempty"`

	// Address is the absolute module address, omitted for the root module
	Address string `json:"address,omitempty"`
}

type JSONResource struct {
	// Address is the absolute address for this resource instance
	Address string `json:"address,omitempty"`

	// Mode can be "managed" or "data"
	Mode string `json:"mode,omitempty"`

	Type          string          `json:"type,omitempty"`
	Name          string          `json:"name,omitempty"`
	Index         json.RawMessage `json:"index,omitempty"`
	ProviderName  string          `json:"provider_name,omitempty"`
	SchemaVersion uint64          `json:"schema_version,omitempty"`

	// AttributeValues is the JSON representation of the attribute values
	AttributeValues json.RawMessage `json:"values,omitempty"`

	// SensitiveValues is a similar object to AttributeValues, but with all
	// sensitive values replaced with true
	SensitiveValues json.RawMessage `json:"sensitive_values,omitempty"`

	// DependsOn contains the absolute resource addresses of any resources
	// that this resource instance depends on
	DependsOn []string `json:"depends_on,omitempty"`

	// Tainted is true if this resource is tainted
	Tainted bool `json:"tainted,omitempty"`

	// DeprecatedTainted is true if this resource is tainted. This is
	// deprecated and will be removed.
	DeprecatedTainted bool `json:"tainted,omitempty"`

	// DeposedKey is set if this resource instance is deposed
	DeposedKey string `json:"deposed_key,omitempty"`
}