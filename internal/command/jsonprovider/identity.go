// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonprovider

import (
	"encoding/json"

	"github.com/opentofu/opentofu/internal/providers"
)

type ResourceIdentitySchema struct {
	Version    uint64                        `json:"version"`
	Attributes map[string]*IdentityAttribute `json:"attributes,omitempty"`
}

type IdentityAttribute struct {
	IdentityType      json.RawMessage `json:"type,omitempty"`
	Description       string          `json:"description,omitempty"`
	RequiredForImport bool            `json:"required_for_import,omitempty"`
	OptionalForImport bool            `json:"optional_for_import,omitempty"`
}

// marshalIdentitySchemas converts the provider's identity schemas into a JSON-serializable format.
// It marshals the cty.Type of each attribute into JSON and includes the description and import requirements.
func marshalIdentitySchemas(resources map[string]providers.Schema) map[string]*ResourceIdentitySchema {
	var ret map[string]*ResourceIdentitySchema
	for name, schema := range resources {
		// We should only include resources that have an identity schema defined
		if schema.IdentitySchema == nil {
			continue
		}

		attrs := make(map[string]*IdentityAttribute, len(schema.IdentitySchema.Attributes))
		for k, attr := range schema.IdentitySchema.Attributes {
			attrTy, _ := attr.Type.MarshalJSON()
			attrs[k] = &IdentityAttribute{
				IdentityType:      attrTy,
				Description:       attr.Description,
				RequiredForImport: attr.Required,
				OptionalForImport: attr.Optional,
			}
		}

		// We construct the map here instead of above so that if there are no resources with identity schemas, we can return nil instead of an empty map.
		if ret == nil {
			ret = make(map[string]*ResourceIdentitySchema)
		}
		ret[name] = &ResourceIdentitySchema{
			Version:    uint64(schema.IdentitySchemaVersion),
			Attributes: attrs,
		}
	}
	return ret
}
