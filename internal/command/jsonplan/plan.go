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
	"time"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/jsonchecks"
	"github.com/opentofu/opentofu/internal/command/jsonconfig"
	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/opentofu/opentofu/version"
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
	// might change in the future. However, not all Importing changes will
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
	output := newPlan()
	output.TerraformVersion = version.String()
	output.Timestamp = p.Timestamp.Format(time.RFC3339)
	output.Errored = p.Errored

	err := output.marshalPlanVariables(p.VariableValues, config.Module.Variables)
	if err != nil {
		return nil, fmt.Errorf("error in marshalPlanVariables: %w", err)
	}

	// output.PlannedValues
	err = output.marshalPlannedValues(p.Changes, schemas)
	if err != nil {
		return nil, fmt.Errorf("error in marshalPlannedValues: %w", err)
	}

	// output.ResourceDrift
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
			return nil, fmt.Errorf("error in marshaling resource drift: %w", err)
		}
	}

	if err := output.marshalRelevantAttrs(p); err != nil {
		return nil, fmt.Errorf("error marshaling relevant attributes for external changes: %w", err)
	}

	// output.ResourceChanges
	if p.Changes != nil {
		output.ResourceChanges, err = MarshalResourceChanges(p.Changes.Resources, schemas)
		if err != nil {
			return nil, fmt.Errorf("error in marshaling resource changes: %w", err)
		}
	}

	// output.OutputChanges
	if output.OutputChanges, err = MarshalOutputChanges(p.Changes); err != nil {
		return nil, fmt.Errorf("error in marshaling output changes: %w", err)
	}

	// output.Checks
	if p.Checks != nil && p.Checks.ConfigResults.Len() > 0 {
		output.Checks = jsonchecks.MarshalCheckStates(p.Checks)
	}

	// output.PriorState
	if sf != nil && !sf.State.Empty() {
		output.PriorState, err = jsonstate.Marshal(sf, schemas)
		if err != nil {
			return nil, fmt.Errorf("error marshaling prior state: %w", err)
		}
	}

	// output.Config
	output.Config, err = jsonconfig.Marshal(config, schemas)
	if err != nil {
		return nil, fmt.Errorf("error marshaling config: %w", err)
	}

	return output, nil
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
	var ret []ResourceChange

	var sortedResources []*plans.ResourceInstanceChangeSrc
	sortedResources = append(sortedResources, resources...)
	sort.Slice(sortedResources, func(i, j int) bool {
		if !sortedResources[i].Addr.Equal(sortedResources[j].Addr) {
			return sortedResources[i].Addr.Less(sortedResources[j].Addr)
		}
		return sortedResources[i].DeposedKey < sortedResources[j].DeposedKey
	})

	for _, rc := range sortedResources {
		var r ResourceChange
		addr := rc.Addr
		r.Address = addr.String()
		if !addr.Equal(rc.PrevRunAddr) {
			r.PreviousAddress = rc.PrevRunAddr.String()
		}

		if addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
			// We need to write ephemeral resources to the plan file to be able to build
			// the apply graph on `tofu apply <planfile>`.
			// The DiffTransformer needs the changes from the plan to be able to generate
			// executable resource instance graph nodes, so we are adding the ephemeral resources too.
			// Even though we are writing these, the actual values of the ephemeral *must not*
			// be written to the plan so nullify these.
			rc.ChangeSrc.Before = nil
			rc.ChangeSrc.After = nil
		}
		dataSource := addr.Resource.Resource.Mode == addrs.DataResourceMode
		// We create "delete" actions for data resources so we can clean up
		// their entries in state, but this is an implementation detail that
		// users shouldn't see.
		if dataSource && rc.Action == plans.Delete {
			continue
		}

		schema, _ := schemas.ResourceTypeConfig(
			rc.ProviderAddr.Provider,
			addr.Resource.Resource.Mode,
			addr.Resource.Resource.Type,
		)
		if schema == nil {
			return nil, fmt.Errorf("no schema found for %s (in provider %s)", r.Address, rc.ProviderAddr.Provider)
		}

		changeV, err := rc.Decode(schema.ImpliedType())
		if err != nil {
			return nil, err
		}
		// We drop the marks from the change, as decoding is only an
		// intermediate step to re-encode the values as json
		changeV.Before, _ = changeV.Before.UnmarkDeep()
		changeV.After, _ = changeV.After.UnmarkDeep()

		var before, after []byte
		var beforeSensitive, afterSensitive []byte
		var afterUnknown cty.Value

		if changeV.Before != cty.NilVal {
			before, err = ctyjson.Marshal(changeV.Before, changeV.Before.Type())
			if err != nil {
				return nil, err
			}
			valMarks := rc.BeforeValMarks
			if schema.ContainsMarks() {
				valMarks = append(valMarks, schema.ValueMarks(changeV.Before, nil)...)
			}
			if err := ensureEphemeralMarksAreValid(addr, valMarks); err != nil {
				return nil, err
			}
			bs := jsonstate.SensitiveAsBoolWithPathValueMarks(changeV.Before, valMarks)
			beforeSensitive, err = ctyjson.Marshal(bs, bs.Type())
			if err != nil {
				return nil, err
			}
		}
		if changeV.After != cty.NilVal {
			if changeV.After.IsWhollyKnown() {
				after, err = ctyjson.Marshal(changeV.After, changeV.After.Type())
				if err != nil {
					return nil, err
				}
				afterUnknown = cty.EmptyObjectVal
			} else {
				filteredAfter := omitUnknowns(changeV.After)
				if filteredAfter.IsNull() {
					after = nil
				} else {
					after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
					if err != nil {
						return nil, err
					}
				}
				afterUnknown = unknownAsBool(changeV.After)
			}
			valMarks := rc.AfterValMarks
			if schema.ContainsMarks() {
				valMarks = append(valMarks, schema.ValueMarks(changeV.After, nil)...)
			}
			if err := ensureEphemeralMarksAreValid(addr, valMarks); err != nil {
				return nil, err
			}
			as := jsonstate.SensitiveAsBoolWithPathValueMarks(changeV.After, valMarks)
			afterSensitive, err = ctyjson.Marshal(as, as.Type())
			if err != nil {
				return nil, err
			}
		}

		a, err := ctyjson.Marshal(afterUnknown, afterUnknown.Type())
		if err != nil {
			return nil, err
		}
		replacePaths, err := encodePaths(rc.RequiredReplace)
		if err != nil {
			return nil, err
		}

		var importing *Importing
		if rc.Importing != nil {
			importing = &Importing{ID: rc.Importing.ID}
		}

		r.Change = Change{
			Actions:         actionString(rc.Action.String()),
			Before:          json.RawMessage(before),
			After:           json.RawMessage(after),
			AfterUnknown:    a,
			BeforeSensitive: json.RawMessage(beforeSensitive),
			AfterSensitive:  json.RawMessage(afterSensitive),
			ReplacePaths:    replacePaths,
			Importing:       importing,
			GeneratedConfig: rc.GeneratedConfig,
		}

		if rc.DeposedKey != states.NotDeposed {
			r.Deposed = rc.DeposedKey.String()
		}

		key := addr.Resource.Key
		if key != nil {
			value := key.Value()
			if r.Index, err = ctyjson.Marshal(value, value.Type()); err != nil {
				return nil, err
			}
		}

		switch addr.Resource.Resource.Mode {
		case addrs.ManagedResourceMode:
			r.Mode = jsonstate.ManagedResourceMode
		case addrs.DataResourceMode:
			r.Mode = jsonstate.DataResourceMode
		case addrs.EphemeralResourceMode:
			r.Mode = jsonstate.EphemeralResourceMode
		default:
			return nil, fmt.Errorf("resource %s has an unsupported mode %s", r.Address, addr.Resource.Resource.Mode.String())
		}
		r.ModuleAddress = addr.Module.String()
		r.Name = addr.Resource.Resource.Name
		r.Type = addr.Resource.Resource.Type
		r.ProviderName = rc.ProviderAddr.Provider.String()

		switch rc.ActionReason {
		case plans.ResourceInstanceChangeNoReason:
			r.ActionReason = "" // will be omitted in output
		case plans.ResourceInstanceReplaceBecauseCannotUpdate:
			r.ActionReason = ResourceInstanceReplaceBecauseCannotUpdate
		case plans.ResourceInstanceReplaceBecauseTainted:
			r.ActionReason = ResourceInstanceReplaceBecauseTainted
		case plans.ResourceInstanceReplaceByRequest:
			r.ActionReason = ResourceInstanceReplaceByRequest
		case plans.ResourceInstanceReplaceByTriggers:
			r.ActionReason = ResourceInstanceReplaceByTriggers
		case plans.ResourceInstanceDeleteBecauseNoResourceConfig:
			r.ActionReason = ResourceInstanceDeleteBecauseNoResourceConfig
		case plans.ResourceInstanceDeleteBecauseWrongRepetition:
			r.ActionReason = ResourceInstanceDeleteBecauseWrongRepetition
		case plans.ResourceInstanceDeleteBecauseCountIndex:
			r.ActionReason = ResourceInstanceDeleteBecauseCountIndex
		case plans.ResourceInstanceDeleteBecauseEachKey:
			r.ActionReason = ResourceInstanceDeleteBecauseEachKey
		case plans.ResourceInstanceDeleteBecauseNoModule:
			r.ActionReason = ResourceInstanceDeleteBecauseNoModule
		case plans.ResourceInstanceDeleteBecauseNoMoveTarget:
			r.ActionReason = ResourceInstanceDeleteBecauseNoMoveTarget
		case plans.ResourceInstanceReadBecauseConfigUnknown:
			r.ActionReason = ResourceInstanceReadBecauseConfigUnknown
		case plans.ResourceInstanceReadBecauseDependencyPending:
			r.ActionReason = ResourceInstanceReadBecauseDependencyPending
		case plans.ResourceInstanceReadBecauseCheckNested:
			r.ActionReason = ResourceInstanceReadBecauseCheckNested
		default:
			return nil, fmt.Errorf("resource %s has an unsupported action reason %s", r.Address, rc.ActionReason)
		}

		ret = append(ret, r)

	}

	return ret, nil
}

func ensureEphemeralMarksAreValid(addr addrs.AbsResourceInstance, valMarks []cty.PathValueMarks) error {
	// ephemeral resources will have the ephemeral mark at the root of the value, got from schema.ValueMarks
	// so we don't want to error for those particular ones
	if addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
		return nil
	}
	if err := marks.EnsureNoEphemeralMarks(valMarks); err != nil {
		return fmt.Errorf("%s: %w", addr, err)
	}
	return nil
}

// GenerateChange is used to receive two values and calculate the difference
// between them in order to return a Change struct
func GenerateChange(beforeVal, afterVal cty.Value) (*Change, error) {
	var err error
	beforeVal, marks := beforeVal.UnmarkDeepWithPaths()
	bs := jsonstate.SensitiveAsBoolWithPathValueMarks(beforeVal, marks)
	beforeSensitive, err := ctyjson.Marshal(bs, bs.Type())
	if err != nil {
		return nil, err
	}

	afterVal, marks = afterVal.UnmarkDeepWithPaths()
	as := jsonstate.SensitiveAsBoolWithPathValueMarks(afterVal, marks)
	afterSensitive, err := ctyjson.Marshal(as, as.Type())
	if err != nil {
		return nil, err
	}

	var before, after []byte
	var afterUnknown cty.Value

	if beforeVal != cty.NilVal {
		before, err = ctyjson.Marshal(beforeVal, beforeVal.Type())
		if err != nil {
			return nil, err
		}
	}
	if afterVal != cty.NilVal {
		if afterVal.IsWhollyKnown() {
			after, err = ctyjson.Marshal(afterVal, afterVal.Type())
			if err != nil {
				return nil, err
			}
			afterUnknown = cty.False
		} else {
			filteredAfter := omitUnknowns(afterVal)
			if filteredAfter.IsNull() {
				after = nil
			} else {
				after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
				if err != nil {
					return nil, err
				}
			}
			afterUnknown = unknownAsBool(afterVal)
		}
	}

	a, _ := ctyjson.Marshal(afterUnknown, afterUnknown.Type())

	return &Change{
		Before:       json.RawMessage(before),
		After:        json.RawMessage(after),
		AfterUnknown: a,

		BeforeSensitive: json.RawMessage(beforeSensitive),
		AfterSensitive:  json.RawMessage(afterSensitive),
		// Just to be explicit, outputs cannot be imported so this is always
		// nil.
		Importing: nil,
	}, nil
}

// MarshalOutputChanges converts the provided internal representation of
// Changes objects into the structured JSON representation.
//
// This function is referenced directly from the structured renderer tests, to
// ensure parity between the renderers. It probably shouldn't be used anywhere
// else.
func MarshalOutputChanges(changes *plans.Changes) (map[string]Change, error) {
	if changes == nil {
		// Nothing to do!
		return nil, nil
	}

	outputChanges := make(map[string]Change, len(changes.Outputs))
	for _, oc := range changes.Outputs {

		// Skip output changes that are not from the root module.
		// These are automatically stripped from plans that are written to disk
		// elsewhere, we just need to duplicate the logic here in case anyone
		// is converting this plan directly from memory.
		if !oc.Addr.Module.IsRoot() {
			continue
		}

		changeV, err := oc.Decode()
		if err != nil {
			return nil, err
		}

		// The only information we have in the plan about output sensitivity is
		// a boolean which is true if the output was or is marked sensitive. As
		// a result, BeforeSensitive and AfterSensitive will be identical, and
		// either false or true.
		outputSensitive := cty.False
		if oc.Sensitive {
			outputSensitive = cty.True
		}
		sensitive, err := ctyjson.Marshal(outputSensitive, outputSensitive.Type())
		if err != nil {
			return nil, err
		}

		change, err := GenerateChange(changeV.Before, changeV.After)
		if err != nil {
			return nil, err
		}

		change.Actions = actionString(oc.Action.String())
		change.BeforeSensitive = json.RawMessage(sensitive)
		change.AfterSensitive = json.RawMessage(sensitive)

		outputChanges[oc.Addr.OutputValue.Name] = *change
	}

	return outputChanges, nil
}

func (p *Plan) marshalPlannedValues(changes *plans.Changes, schemas *tofu.Schemas) error {
	// marshal the planned changes into a module
	plan, err := marshalPlannedValues(changes, schemas)
	if err != nil {
		return err
	}
	p.PlannedValues.RootModule = plan

	// marshalPlannedOutputs
	outputs, err := marshalPlannedOutputs(changes)
	if err != nil {
		return err
	}
	p.PlannedValues.Outputs = outputs

	return nil
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

	// We sort the relevant attributes by resource address to make the output
	// deterministic. Our own equivalence tests rely on it.
	sort.Slice(p.RelevantAttributes, func(i, j int) bool {
		return p.RelevantAttributes[i].Resource < p.RelevantAttributes[j].Resource
	})

	return nil
}

// omitUnknowns recursively walks the src cty.Value and returns a new cty.Value,
// omitting any unknowns.
//
// The result also normalizes some types: all sequence types are turned into
// tuple types and all mapping types are converted to object types, since we
// assume the result of this is just going to be serialized as JSON (and thus
// lose those distinctions) anyway.
func omitUnknowns(val cty.Value) cty.Value {
	ty := val.Type()
	switch {
	case val.IsNull():
		return val
	case !val.IsKnown():
		return cty.NilVal
	case ty.IsPrimitiveType():
		return val
	case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
		var vals []cty.Value
		it := val.ElementIterator()
		for it.Next() {
			_, v := it.Element()
			newVal := omitUnknowns(v)
			if newVal != cty.NilVal {
				vals = append(vals, newVal)
			} else if newVal == cty.NilVal {
				// element order is how we correlate unknownness, so we must
				// replace unknowns with nulls
				vals = append(vals, cty.NullVal(v.Type()))
			}
		}
		// We use tuple types always here, because the work we did above
		// may have caused the individual elements to have different types,
		// and we're doing this work to produce JSON anyway and JSON marshalling
		// represents all of these sequence types as an array.
		return cty.TupleVal(vals)
	case ty.IsMapType() || ty.IsObjectType():
		vals := make(map[string]cty.Value)
		it := val.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			newVal := omitUnknowns(v)
			if newVal != cty.NilVal {
				vals[k.AsString()] = newVal
			}
		}
		// We use object types always here, because the work we did above
		// may have caused the individual elements to have different types,
		// and we're doing this work to produce JSON anyway and JSON marshalling
		// represents both of these mapping types as an object.
		return cty.ObjectVal(vals)
	default:
		// Should never happen, since the above should cover all types
		panic(fmt.Sprintf("omitUnknowns cannot handle %#v", val))
	}
}

// recursively iterate through a cty.Value, replacing unknown values (including
// null) with cty.True and known values with cty.False.
//
// The result also normalizes some types: all sequence types are turned into
// tuple types and all mapping types are converted to object types, since we
// assume the result of this is just going to be serialized as JSON (and thus
// lose those distinctions) anyway.
//
// For map/object values, all known attribute values will be omitted instead of
// returning false, as this results in a more compact serialization.
func unknownAsBool(val cty.Value) cty.Value {
	ty := val.Type()
	switch {
	case val.IsNull():
		return cty.False
	case !val.IsKnown():
		if ty.IsPrimitiveType() || ty.Equals(cty.DynamicPseudoType) {
			return cty.True
		}
		fallthrough
	case ty.IsPrimitiveType():
		return cty.BoolVal(!val.IsKnown())
	case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
		length := val.LengthInt()
		if length == 0 {
			// If there are no elements then we can't have unknowns
			return cty.EmptyTupleVal
		}
		vals := make([]cty.Value, 0, length)
		it := val.ElementIterator()
		for it.Next() {
			_, v := it.Element()
			vals = append(vals, unknownAsBool(v))
		}
		// The above transform may have changed the types of some of the
		// elements, so we'll always use a tuple here in case we've now made
		// different elements have different types. Our ultimate goal is to
		// marshal to JSON anyway, and all of these sequence types are
		// indistinguishable in JSON.
		return cty.TupleVal(vals)
	case ty.IsMapType() || ty.IsObjectType():
		var length int
		switch {
		case ty.IsMapType():
			length = val.LengthInt()
		default:
			length = len(val.Type().AttributeTypes())
		}
		if length == 0 {
			// If there are no elements then we can't have unknowns
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value)
		it := val.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			vAsBool := unknownAsBool(v)
			// Omit all of the "false"s for known values for more compact
			// serialization
			if !vAsBool.RawEquals(cty.False) {
				vals[k.AsString()] = vAsBool
			}
		}
		// The above transform may have changed the types of some of the
		// elements, so we'll always use an object here in case we've now made
		// different elements have different types. Our ultimate goal is to
		// marshal to JSON anyway, and all of these mapping types are
		// indistinguishable in JSON.
		return cty.ObjectVal(vals)
	default:
		// Should never happen, since the above should cover all types
		panic(fmt.Sprintf("unknownAsBool cannot handle %#v", val))
	}
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
	case "Open":
		return []string{"open"}
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
		case "open":
			return plans.Open
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
