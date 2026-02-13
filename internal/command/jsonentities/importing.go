// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonentities

import "encoding/json"

// Importing contains metadata about a resource change that includes an import
// action.
//
// Every field in here should be treated as optional as future versions do not
// make a guarantee that they will retain the format of this change.
//
// Consumers should be capable of rendering/parsing the Importing struct even
// if it does not have the ID or Identity fields set.
type Importing struct {
	// TODO: Ensure usages of this ID also handle Identity
	ID string `json:"id,omitempty"`

	Identity json.RawMessage `json:"identity,omitempty"`
}
