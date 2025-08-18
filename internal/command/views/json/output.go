// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package json

import (
	"encoding/json"
	"fmt"

	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/command/jsonentities"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func OutputsFromMap(outputValues map[string]*states.OutputValue) (jsonentities.Outputs, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	outputs := make(map[string]jsonentities.Output, len(outputValues))

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

		outputs[name] = jsonentities.Output{
			Sensitive:  ov.Sensitive,
			Deprecated: ov.Deprecated,
			Type:       json.RawMessage(valueType),
			Value:      redactedValue,
		}
	}

	return outputs, nil
}

func OutputsFromChanges(changes []*plans.OutputChangeSrc) jsonentities.Outputs {
	outputs := make(map[string]jsonentities.Output, len(changes))

	for _, change := range changes {
		outputs[change.Addr.OutputValue.Name] = jsonentities.Output{
			Sensitive: change.Sensitive,
			Action:    jsonentities.ParseChangeAction(change.Action),
		}
	}

	return outputs
}
