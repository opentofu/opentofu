// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonentities

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/command/jsonformat/structured"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured/attribute_path"
)

type Output struct {
	Sensitive  bool            `json:"sensitive"`
	Deprecated string          `json:"deprecated,omitempty"`
	Type       json.RawMessage `json:"type,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Action     ChangeAction    `json:"action,omitempty"`
}

type Outputs map[string]Output

func (o Outputs) String() string {
	return fmt.Sprintf("Outputs: %d", len(o))
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
