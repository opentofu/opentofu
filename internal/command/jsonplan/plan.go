// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tofu"
)

// FormatVersion represents the version of the json format and will be
// incremented for any change to this format that requires changes to a
// consuming parser.
const (
	FormatVersion = "1.2"

	ResourceInstanceReplaceBecauseCannotUpdate    = "replace_because_cannot_update"
	ResourceInstanceReplaceBecauseTainted         = "replace_because_tainted"
	ResourceInstanceReplaceByRequest              = "replace_by_request"
	ResourceInstanceReplaceByTriggers             = "replace_by_triggers"
	ResourceInstanceDeleteBecauseNoResourceConfig = "delete_because_no_resource_config"
	ResourceInstanceDeleteBecauseWrongRepetition  = "delete_because_wrong_repetition"
	ResourceInstanceDeleteBecauseCountIndex       = "delete_because_count_index"
	ResourceInstanceDeleteBecauseEachKey          = "delete_because_each_key"
	ResourceInstanceDeleteBecauseNoModule         = "delete_because_no_module"
	ResourceInstanceDeleteBecauseNoMoveTarget     = "delete_because_no_move_target"
	ResourceInstanceReadBecauseConfigUnknown      = "read_because_config_unknown"
	ResourceInstanceReadBecauseDependencyPending  = "read_because_dependency_pending"
	ResourceInstanceReadBecauseCheckNested        = "read_because_check_nested"
)

// Plan is the top-level representation of the json format of a plan. It includes
// the complete config and current state.
type Plan struct {
	FormatVersion    string      `json:"format_version,omitempty"`
	TerraformVersion string      `json:"terraform_version,omitempty"`
	Variables        Variables   `json:"variables,omitempty"`
	PlannedValues    StateValues `json:"planned_values,omitempty"`
	// ResourceDrift and ResourceChanges are sorted in a user-friendly order
	// that is undefined at this time, but consistent.
	ResourceDrift      []ResourceChange  `json:"resource_drift,omitempty"`
	ResourceChanges    []ResourceChange  `json:"resource_changes,omitempty"`
	OutputChanges      map[string]Change `json:"output_changes,omitempty"`
	PriorState         json.RawMessage   `json:"prior_state,omitempty"`
	Config             json.RawMessage   `json:"configuration,omitempty"`
	RelevantAttributes []ResourceAttr    `json:"relevant_attributes,omitempty"`
	Checks             json.RawMessage   `json:"checks,omitempty"`
	Timestamp          string            `json:"timestamp,omitempty"`
	Errored            bool              `json:"errored"`
}

func newPlan() *Plan {
	return &Plan{
		FormatVersion: FormatVersion,
	}
}

// ResourceAttr contains the address and attribute of an external for the
// RelevantAttributes in the plan.
type ResourceAttr struct {
	Resource string          `json:"resource"`
	Attr     json.RawMessage `json:"attribute"`
}

// Change is the representation of a proposed change for an object.
type Change struct {
	// Actions are the actions that will be taken on the object selected by the
	// properties below. Valid actions values are:
	//    ["no-op"]
	//    ["create"]
	//    ["read"]
	//    ["update"]
	//    ["delete", "create"]
	//    ["create", "delete"]
	//    ["delete"]
	//    ["forget"]
	// The two "replace" actions are represented in this way to allow callers to
	// e.g. just scan the list for "delete" to recognize all three situations
	// where the object will be deleted, allowing for any new deletion
	// combinations that might be added in future.
	Actions []string `json:"actions,omitempty"`

	// Before and After are representations of the object value both before and
	// after the action. For ["delete"] and ["forget"] actions, the "after"
	// value is unset. For ["create"] the "before" is unset. For ["no-op"], the
	// before and after values are identical. The "after" value will be
	// incomplete if there are values within it that won't be known until after
	// apply.
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`

	// AfterUnknown is an object value with similar structure to After, but
	// with all unknown leaf values replaced with true, and all known leaf
	// values omitted.  This can be combined with After to reconstruct a full
	// value after the action, including values which will only be known after
	// apply.
	AfterUnknown json.RawMessage `json:"after_unknown,omitempty"`

	// BeforeSensitive and AfterSensitive are object values with similar
	// structure to Before and After, but with all sensitive leaf values
	// replaced with true, and all non-sensitive leaf values omitted. These
	// objects should be combined with Before and After to prevent accidental
	// display of sensitive values in user interfaces.
	BeforeSensitive json.RawMessage `json:"before_sensitive,omitempty"`
	AfterSensitive  json.RawMessage `json:"after_sensitive,omitempty"`

	// ReplacePaths is an array of arrays representing a set of paths into the
	// object value which resulted in the action being "replace". This will be
	// omitted if the action is not replace, or if no paths caused the
	// replacement (for example, if the resource was tainted). Each path
	// consists of one or more steps, each of which will be a number or a
	// string.
	ReplacePaths json.RawMessage `json:"replace_paths,omitempty"`

	// Importing contains the import metadata about this operation. If importing
	// is present (ie. not null) then the change is an import operation in
	// addition to anything mentioned in the actions field. The actual contents
	// of the Importing struct is subject to change, so downstream consumers
	// should treat any values in here as strictly optional.
	Importing *Importing `json:"importing,omitempty"`

	// GeneratedConfig contains any HCL config generated for this resource
	// during planning as a string.
	//
	// If this is populated, then Importing should also be populated but this
	// might change in the future. However, nNot all Importing changes will
	// contain generated config.
	GeneratedConfig string `json:"generated_config,omitempty"`
}

// Importing is a nested object for the resource import metadata.
type Importing struct {
	// The original ID of this resource used to target it as part of planned
	// import operation.
	ID string `json:"id,omitempty"`
}

type Output struct {
	Sensitive bool            `json:"sensitive"`
	Type      json.RawMessage `json:"type,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

// Variables is the JSON representation of the variables provided to the current
// plan.
type Variables map[string]*Variable

type Variable struct {
	Value json.RawMessage `json:"value,omitempty"`
}

// MarshalForRenderer returns the pre-json encoding changes of the requested
// plan, in a format available to the structured renderer.
//
// This function does a small part of the Marshal function, as it only returns
// the part of the plan required by the jsonformat.Plan renderer.
func MarshalForRenderer(
	p *plans.Plan,
	schemas *tofu.Schemas,
) (map[string]Change, []ResourceChange, []ResourceChange, []ResourceAttr, error) {
	output := newPlan()

	var err error
	if output.OutputChanges, err = MarshalOutputChanges(p.Changes); err != nil {
		return nil, nil, nil, nil, err
	}

	if output.ResourceChanges, err = MarshalResourceChanges(p.Changes.Resources, schemas); err != nil {
		return nil, nil, nil, nil, err
	}

	if len(p.DriftedResources) > 0 {
		// In refresh-only mode, we render all resources marked as drifted,
		// including those which have moved without other changes. In other plan
		// modes, move-only changes will be included in the planned changes, so
		// we skip them here.
		var driftedResources []*plans.ResourceInstanceChangeSrc
		if p.UIMode == plans.RefreshOnlyMode {
			driftedResources = p.DriftedResources
		} else {
			for _, dr := range p.DriftedResources {
				if dr.Action != plans.NoOp {
					driftedResources = append(driftedResources, dr)
				}
			}
		}
		output.ResourceDrift, err = MarshalResourceChanges(driftedResources, schemas)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	if err := output.marshalRelevantAttrs(p); err != nil {
		return nil, nil, nil, nil, err
	}

	return output.OutputChanges, output.ResourceChanges, output.ResourceDrift, output.RelevantAttributes, nil
}

// MarshalForLog returns the original JSON compatible plan, ready for a logging
// package to marshal further.
func MarshalForLog(
	config *configs.Config,
	p *plans.Plan,
	sf *statefile.File,
	schemas *tofu.Schemas,
) (*Plan, error) {
	// Delegate to the plans package implementation
	jsonPlan, err := plans.MarshalForLog(config, p, sf, schemas)
	if err != nil {
		return nil, err
	}
	
	// Convert from plans.JSONPlan to our local Plan type
	// For hackathon, we'll just marshal and unmarshal
	jsonBytes, err := json.Marshal(jsonPlan)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal intermediate JSON plan: %w", err)
	}
	
	var output Plan
	if err := json.Unmarshal(jsonBytes, &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to jsonplan.Plan: %w", err)
	}
	
	return &output, nil
}

func (p *Plan) marshalRelevantAttrs(plan *plans.Plan) error {
	for _, ra := range plan.RelevantAttributes {
		addr := ra.Resource.String()
		path, err := encodePath(ra.Attr)
		if err != nil {
			return err
		}

		p.RelevantAttributes = append(p.RelevantAttributes, ResourceAttr{addr, path})
	}

	// Sort for consistency
	sort.Slice(p.RelevantAttributes, func(i, j int) bool {
		return p.RelevantAttributes[i].Resource < p.RelevantAttributes[j].Resource
	})

	return nil
}

// Marshal returns the json encoding of a tofu plan.
func Marshal(
	config *configs.Config,
	p *plans.Plan,
	sf *statefile.File,
	schemas *tofu.Schemas,
) ([]byte, error) {
	output, err := MarshalForLog(config, p, sf, schemas)
	if err != nil {
		return nil, err
	}

	return json.Marshal(output)
}

func (p *Plan) marshalPlanVariables(vars map[string]plans.DynamicValue, decls map[string]*configs.Variable) error {
	p.Variables = make(Variables, len(vars))

	for k, v := range vars {
		val, err := v.Decode(cty.DynamicPseudoType)
		if err != nil {
			return err
		}
		valJSON, err := ctyjson.Marshal(val, val.Type())
		if err != nil {
			return err
		}
		p.Variables[k] = &Variable{
			Value: valJSON,
		}
	}

	// In Terraform v1.1 and earlier we had some confusion about which subsystem
	// of Terraform was the one responsible for substituting in default values
	// for unset module variables, with root module variables being handled in
	// three different places while child module variables were only handled
	// during the Terraform Core graph walk.
	//
	// For Terraform v1.2 and later we rationalized that by having the Terraform
	// Core graph walk always be responsible for selecting defaults regardless
	// of root vs. child module, but unfortunately our earlier accidental
	// misbehavior bled out into the public interface by making the defaults
	// show up in the "vars" map to this function. Those are now correctly
	// omitted (so that the plan file only records the variables _actually_
	// set by the caller) but consumers of the JSON plan format may be depending
	// on our old behavior and so we'll fake it here just in time so that
	// outside consumers won't see a behavior change.
	for name, decl := range decls {
		if _, ok := p.Variables[name]; ok {
			continue
		}
		if val := decl.Default; val != cty.NilVal {
			valJSON, err := ctyjson.Marshal(val, val.Type())
			if err != nil {
				return err
			}
			p.Variables[name] = &Variable{
				Value: valJSON,
			}
		}
	}

	if len(p.Variables) == 0 {
		p.Variables = nil // omit this property if there are no variables to describe
	}

	return nil
}

// MarshalResourceChanges converts the provided internal representation of
// ResourceInstanceChangeSrc objects into the public structured JSON changes.
//
// This function is referenced directly from the structured renderer tests, to
// ensure parity between the renderers. It probably shouldn't be used anywhere
// else.
func MarshalResourceChanges(resources []*plans.ResourceInstanceChangeSrc, schemas *tofu.Schemas) ([]ResourceChange, error) {
	// Delegate to plans package
	jsonChanges, err := plans.MarshalResourceChanges(resources, schemas)
	if err != nil {
		return nil, err
	}
	
	// Convert to our local type
	jsonBytes, err := json.Marshal(jsonChanges)
	if err != nil {
		return nil, err
	}
	
	var result []ResourceChange
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}
	
	return result, nil
}

// MarshalOutputChanges converts the provided internal representation of
// Changes objects into the structured JSON representation.
//
// This function is referenced directly from the structured renderer tests, to
// ensure parity between the renderers. It probably shouldn't be used anywhere
// else.
func MarshalOutputChanges(changes *plans.Changes) (map[string]Change, error) {
	// Delegate to plans package
	jsonChanges, err := plans.MarshalOutputChanges(changes)
	if err != nil {
		return nil, err
	}
	
	// Convert to our local type
	jsonBytes, err := json.Marshal(jsonChanges)
	if err != nil {
		return nil, err
	}
	
	var result map[string]Change
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}
	
	return result, nil
}


func actionString(action string) []string {
	switch action {
	case "NoOp":
		return []string{"no-op"}
	case "Create":
		return []string{"create"}
	case "Delete":
		return []string{"delete"}
	case "Update":
		return []string{"update"}
	case "CreateThenDelete":
		return []string{"create", "delete"}
	case "Read":
		return []string{"read"}
	case "DeleteThenCreate":
		return []string{"delete", "create"}
	case "Forget":
		return []string{"forget"}
	default:
		return []string{action}
	}
}

// UnmarshalActions reverses the actionString function.
func UnmarshalActions(actions []string) plans.Action {
	if len(actions) == 2 {
		if actions[0] == "create" && actions[1] == "delete" {
			return plans.CreateThenDelete
		}

		if actions[0] == "delete" && actions[1] == "create" {
			return plans.DeleteThenCreate
		}
	}

	if len(actions) == 1 {
		switch actions[0] {
		case "create":
			return plans.Create
		case "delete":
			return plans.Delete
		case "update":
			return plans.Update
		case "read":
			return plans.Read
		case "no-op":
			return plans.NoOp
		case "forget":
			return plans.Forget
		}
	}

	panic("unrecognized action slice: " + strings.Join(actions, ", "))
}

// encodePaths lossily encodes a cty.PathSet into an array of arrays of step
// values, such as:
//
//	[["length"],["triggers",0,"value"]]
//
// The lossiness is that we cannot distinguish between an IndexStep with string
// key and a GetAttr step. This is fine with JSON output, because JSON's type
// system means that those two steps are equivalent anyway: both are object
// indexes.
//
// JavaScript (or similar dynamic language) consumers of these values can
// iterate over the steps starting from the root object to reach the
// value that each path is describing.
func encodePaths(pathSet cty.PathSet) (json.RawMessage, error) {
	if pathSet.Empty() {
		return nil, nil
	}

	pathList := pathSet.List()
	jsonPaths := make([]json.RawMessage, 0, len(pathList))

	for _, path := range pathList {
		jsonPath, err := encodePath(path)
		if err != nil {
			return nil, err
		}
		jsonPaths = append(jsonPaths, jsonPath)
	}

	return json.Marshal(jsonPaths)
}

func encodePath(path cty.Path) (json.RawMessage, error) {
	steps := make([]json.RawMessage, 0, len(path))
	for _, step := range path {
		switch s := step.(type) {
		case cty.IndexStep:
			key, err := ctyjson.Marshal(s.Key, s.Key.Type())
			if err != nil {
				return nil, fmt.Errorf("Failed to marshal index step key %#v: %w", s.Key, err)
			}
			steps = append(steps, key)
		case cty.GetAttrStep:
			name, err := json.Marshal(s.Name)
			if err != nil {
				return nil, fmt.Errorf("Failed to marshal get attr step name %#v: %w", s.Name, err)
			}
			steps = append(steps, name)
		default:
			return nil, fmt.Errorf("Unsupported path step %#v (%t)", step, step)
		}
	}
	return json.Marshal(steps)
}
