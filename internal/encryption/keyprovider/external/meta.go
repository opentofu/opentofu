// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import "encoding/json"

// MetadataV1 describes the metadata structure of the external provider.
type MetadataV1 struct {
	ExternalData map[string]any `hcl:"external_data" json:"external_data"`
}

// Encode encodes the current [MetadataV1] to work properly with external key providers.
// When the [MetadataV1.ExternalData] is nil, it returns "null".
// The [MetadataV1.ExternalData] can be an empty map, which is a valid case meaning that it should
// return that encoded accordingly.
func (m *MetadataV1) Encode() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	if m.ExternalData == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m)
}
