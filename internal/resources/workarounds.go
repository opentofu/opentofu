// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"encoding/json"
	"log"

	"github.com/zclconf/go-cty/cty"
)

// This is ported from tofu/upgrade_resource_state.go. Eventually, it's tests should follow it here.

// stripRemovedStateAttributes deletes any attributes no longer present in the
// schema, so that the json can be correctly decoded.
func stripRemovedStateAttributes(state []byte, ty cty.Type) []byte {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal(state, &jsonMap)
	if err != nil {
		// we just log any errors here, and let the normal decode process catch
		// invalid JSON.
		log.Printf("[ERROR] UpgradeResourceState: stripRemovedStateAttributes: %s", err)
		return state
	}

	// if no changes were made, we return the original state to ensure nothing
	// was altered in the marshaling process.
	if !removeRemovedAttrs(jsonMap, ty) {
		return state
	}

	js, err := json.Marshal(jsonMap)
	if err != nil {
		// if the json map was somehow mangled enough to not marshal, something
		// went horribly wrong
		panic(err)
	}

	return js
}

// strip out the actual missing attributes, and return a bool indicating if any
// changes were made.
func removeRemovedAttrs(v interface{}, ty cty.Type) bool {
	modified := false
	// we're only concerned with finding maps that correspond to object
	// attributes
	switch v := v.(type) {
	case []interface{}:
		switch {
		// If these aren't blocks the next call will be a noop
		case ty.IsListType() || ty.IsSetType():
			eTy := ty.ElementType()
			for _, eV := range v {
				modified = removeRemovedAttrs(eV, eTy) || modified
			}
		}
		return modified
	case map[string]interface{}:
		switch {
		case ty.IsMapType():
			// map blocks aren't yet supported, but handle this just in case
			eTy := ty.ElementType()
			for _, eV := range v {
				modified = removeRemovedAttrs(eV, eTy) || modified
			}
			return modified

		case ty == cty.DynamicPseudoType:
			log.Printf("[DEBUG] UpgradeResourceState: ignoring dynamic block: %#v\n", v)
			return false

		case ty.IsObjectType():
			attrTypes := ty.AttributeTypes()
			for attr, attrV := range v {
				attrTy, ok := attrTypes[attr]
				if !ok {
					log.Printf("[DEBUG] UpgradeResourceState: attribute %q no longer present in schema", attr)
					delete(v, attr)
					modified = true
					continue
				}

				modified = removeRemovedAttrs(attrV, attrTy) || modified
			}
			return modified
		default:
			// This shouldn't happen, and will fail to decode further on, so
			// there's no need to handle it here.
			log.Printf("[WARN] UpgradeResourceState: unexpected type %#v for map in json state", ty)
			return false
		}
	}
	return modified
}
