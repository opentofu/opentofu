// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"fmt"
	"sort"
	"time"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/version"
)

// MarshalForLog returns the original JSON compatible plan, ready for a logging
// package to marshal further. The schemas parameter must be provided from
// outside since we can't import tofu package here.
func MarshalForLog(
	config *configs.Config,
	p *Plan,
	sf *statefile.File,
	schemas interface{}, // This is actually *tofu.Schemas but we can't import that
) (*JSONPlan, error) {
	output := newJSONPlan()
	output.TerraformVersion = version.String()
	output.Timestamp = p.Timestamp.Format(time.RFC3339)
	output.Errored = p.Errored
	output.MiddlewareMetadata = p.MiddlewareMetadata

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
		var driftedResources []*ResourceInstanceChangeSrc
		if p.UIMode == RefreshOnlyMode {
			driftedResources = p.DriftedResources
		} else {
			for _, dr := range p.DriftedResources {
				if dr.Action != NoOp {
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
		// Import jsonchecks functionality here
		output.Checks = MarshalCheckStates(p.Checks)
	}

	// output.PriorState
	if sf != nil && !sf.State.Empty() {
		output.PriorState, err = MarshalState(sf, schemas)
		if err != nil {
			return nil, fmt.Errorf("error marshaling prior state: %w", err)
		}
	}

	// output.Config
	output.Config, err = MarshalConfig(config, schemas)
	if err != nil {
		return nil, fmt.Errorf("error marshaling config: %w", err)
	}

	return output, nil
}

func newJSONPlan() *JSONPlan {
	return &JSONPlan{
		FormatVersion: "1.2",
	}
}

func (p *JSONPlan) marshalPlanVariables(vars map[string]DynamicValue, decls map[string]*configs.Variable) error {
	p.Variables = make(map[string]*JSONVariable, len(vars))

	for k, v := range vars {
		val, err := v.Decode(cty.DynamicPseudoType)
		if err != nil {
			return err
		}
		valJSON, err := ctyjson.Marshal(val, val.Type())
		if err != nil {
			return err
		}
		p.Variables[k] = &JSONVariable{
			Value: valJSON,
		}
	}

	// Handle default values for backwards compatibility
	for name, decl := range decls {
		if _, ok := p.Variables[name]; ok {
			continue
		}
		if val := decl.Default; val != cty.NilVal {
			valJSON, err := ctyjson.Marshal(val, val.Type())
			if err != nil {
				return err
			}
			p.Variables[name] = &JSONVariable{
				Value: valJSON,
			}
		}
	}

	if len(p.Variables) == 0 {
		p.Variables = nil // omit this property if there are no variables to describe
	}

	return nil
}

func (p *JSONPlan) marshalRelevantAttrs(plan *Plan) error {
	for _, ra := range plan.RelevantAttributes {
		addr := ra.Resource.String()
		path, err := encodePath(ra.Attr)
		if err != nil {
			return err
		}

		p.RelevantAttributes = append(p.RelevantAttributes, JSONResourceAttr{addr, path})
	}

	// We sort the relevant attributes by resource address to make the output
	// deterministic. Our own equivalence tests rely on it.
	sort.Slice(p.RelevantAttributes, func(i, j int) bool {
		return p.RelevantAttributes[i].Resource < p.RelevantAttributes[j].Resource
	})

	return nil
}

// Constants for JSON marshaling - map internal constants to JSON strings
const (
	JSONResourceInstanceReplaceBecauseCannotUpdate    = "replace_because_cannot_update"
	JSONResourceInstanceReplaceBecauseTainted         = "replace_because_tainted"
	JSONResourceInstanceReplaceByRequest              = "replace_by_request"
	JSONResourceInstanceReplaceByTriggers             = "replace_by_triggers"
	JSONResourceInstanceDeleteBecauseNoResourceConfig = "delete_because_no_resource_config"
	JSONResourceInstanceDeleteBecauseWrongRepetition  = "delete_because_wrong_repetition"
	JSONResourceInstanceDeleteBecauseCountIndex       = "delete_because_count_index"
	JSONResourceInstanceDeleteBecauseEachKey          = "delete_because_each_key"
	JSONResourceInstanceDeleteBecauseNoModule         = "delete_because_no_module"
	JSONResourceInstanceDeleteBecauseNoMoveTarget     = "delete_because_no_move_target"
	JSONResourceInstanceReadBecauseConfigUnknown      = "read_because_config_unknown"
	JSONResourceInstanceReadBecauseDependencyPending  = "read_because_dependency_pending"
	JSONResourceInstanceReadBecauseCheckNested        = "read_because_check_nested"
)