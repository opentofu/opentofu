// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonentities

import (
	"encoding/json"
	"fmt"
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
