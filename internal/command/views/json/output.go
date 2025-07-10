// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package json

import (
	"encoding/json"
	"fmt"

	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/command/jsondiffer/structured"
	"github.com/opentofu/opentofu/internal/command/jsondiffer/structured/attribute_path"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Output struct {
	Sensitive  bool            `json:"sensitive"`
	Deprecated string          `json:"deprecated,omitempty"`
	Type       json.RawMessage `json:"type,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Action     ChangeAction    `json:"action,omitempty"`
}

type Outputs map[string]Output

func OutputsFromMap(outputValues map[string]*states.OutputValue) (Outputs, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	outputs := make(map[string]Output, len(outputValues))

	for name, ov := range outputValues {
		unmarked, _ := ov.Value.UnmarkDeep()
		value, err := ctyjson.Marshal(unmarked, unmarked.Type())
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Error serializing output %q", name),
				fmt.Sprintf("Error: %s", err),
			))
			return nil, diags
		}
		valueType, err := ctyjson.MarshalType(unmarked.Type())
		if err != nil {
			diags = diags.Append(err)
			return nil, diags
		}

		var redactedValue json.RawMessage
		if !ov.Sensitive {
			redactedValue = json.RawMessage(value)
		}

		outputs[name] = Output{
			Sensitive:  ov.Sensitive,
			Deprecated: ov.Deprecated,
			Type:       json.RawMessage(valueType),
			Value:      redactedValue,
		}
	}

	return outputs, nil
}

// FromJsonViewsOutput unmarshals the raw values in the viewsjson.Output structs into
// generic interface{} types that can be reasoned about.
func FromJsonViewsOutput(output Output) structured.Change {
	return structured.Change{
		// We model resource formatting as NoOps.
		Before: structured.UnmarshalGeneric(output.Value),
		After:  structured.UnmarshalGeneric(output.Value),

		// We have some sensitive values, but we don't have any unknown values.
		Unknown:         false,
		BeforeSensitive: output.Sensitive,
		AfterSensitive:  output.Sensitive,

		// We don't display replacement data for resources, and all attributes
		// are relevant.
		ReplacePaths:       attribute_path.Empty(false),
		RelevantAttributes: attribute_path.AlwaysMatcher(),
	}
}

func OutputsFromChanges(changes []*plans.OutputChangeSrc) Outputs {
	outputs := make(map[string]Output, len(changes))

	for _, change := range changes {
		outputs[change.Addr.OutputValue.Name] = Output{
			Sensitive: change.Sensitive,
			Action:    changeAction(change.Action),
		}
	}

	return outputs
}

func (o Outputs) String() string {
	return fmt.Sprintf("Outputs: %d", len(o))
}
