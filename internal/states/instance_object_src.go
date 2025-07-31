// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package states

import (
	"bytes"
	"reflect"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/legacy/hcl2shim"
)

// ResourceInstanceObjectSrc is a not-fully-decoded version of
// ResourceInstanceObject. Decoding of it can be completed by first handling
// any schema migration steps to get to the latest schema version and then
// calling method Decode with the implied type of the latest schema.
type ResourceInstanceObjectSrc struct {
	// SchemaVersion is the resource-type-specific schema version number that
	// was current when either AttrsJSON or AttrsFlat was encoded. Migration
	// steps are required if this is less than the current version number
	// reported by the corresponding provider.
	SchemaVersion uint64

	// AttrsJSON is a JSON-encoded representation of the object attributes,
	// encoding the value (of the object type implied by the associated resource
	// type schema) that represents this remote object in OpenTofu Language
	// expressions, and is compared with configuration when producing a diff.
	//
	// This is retained in JSON format here because it may require preprocessing
	// before decoding if, for example, the stored attributes are for an older
	// schema version which the provider must upgrade before use. If the
	// version is current, it is valid to simply decode this using the
	// type implied by the current schema, without the need for the provider
	// to perform an upgrade first.
	//
	// When writing a ResourceInstanceObject into the state, AttrsJSON should
	// always be conformant to the current schema version and the current
	// schema version should be recorded in the SchemaVersion field.
	AttrsJSON []byte

	// AttrsFlat is a legacy form of attributes used in older state file
	// formats, and in the new state format for objects that haven't yet been
	// upgraded. This attribute is mutually exclusive with Attrs: for any
	// ResourceInstanceObject, only one of these attributes may be populated
	// and the other must be nil.
	//
	// An instance object with this field populated should be upgraded to use
	// Attrs at the earliest opportunity, since this legacy flatmap-based
	// format will be phased out over time. AttrsFlat should not be used when
	// writing new or updated objects to state; instead, callers must follow
	// the recommendations in the AttrsJSON documentation above.
	AttrsFlat map[string]string

	// AttrSensitivePaths is an array of paths to mark as sensitive coming out of
	// state, or to save as sensitive paths when saving state
	AttrSensitivePaths []cty.PathValueMarks

	// TransientPathValueMarks helps propagate all the marks (including
	// non-sensitive ones) through the internal representation of a state,
	// without being serialized into the final state file.
	TransientPathValueMarks []cty.PathValueMarks

	// These fields all correspond to the fields of the same name on
	// ResourceInstanceObject.
	Private             []byte
	Status              ObjectStatus
	Dependencies        []addrs.ConfigResource
	CreateBeforeDestroy bool
}

// Compare two lists using an given element equal function, ignoring order and duplicates
func equalSlicesIgnoreOrder[S ~[]E, E any](a, b S, fn func(E, E) bool) bool {
	if len(a) != len(b) {
		return false
	}

	// Not sure if this is the most efficient approach, but it works
	// First check if all elements in a existing in b
	for _, v := range a {
		found := false
		for _, o := range b {
			if fn(v, o) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Now check if all elements in b exist in a
	// This is necessary just in case there are duplicate entries (there should not be).
	for _, v := range b {
		found := false
		for _, o := range a {
			if fn(v, o) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (os *ResourceInstanceObjectSrc) Equal(other *ResourceInstanceObjectSrc) bool {
	if os == other {
		return true
	}
	if os == nil || other == nil {
		return false
	}

	if os.SchemaVersion != other.SchemaVersion {
		return false
	}

	if !bytes.Equal(os.AttrsJSON, other.AttrsJSON) {
		return false
	}

	if !reflect.DeepEqual(os.AttrsFlat, other.AttrsFlat) {
		return false
	}

	// Ignore order/duplicates as that is the assumption in the rest of the codebase.
	// Given that these are generated from maps, it is known that the order is not consistent.
	if !equalSlicesIgnoreOrder(os.AttrSensitivePaths, other.AttrSensitivePaths, cty.PathValueMarks.Equal) {
		return false
	}
	// Ignore order/duplicates as that is the assumption in the rest of the codebase.
	// Given that these are generated from maps, it is known that the order is not consistent.
	if !equalSlicesIgnoreOrder(os.TransientPathValueMarks, other.TransientPathValueMarks, cty.PathValueMarks.Equal) {
		return false
	}

	if !bytes.Equal(os.Private, other.Private) {
		return false
	}

	if os.Status != other.Status {
		return false
	}

	// This represents a set of dependencies.  They must all be resolved before executing and therefore the order does not matter.
	if !equalSlicesIgnoreOrder(os.Dependencies, other.Dependencies, addrs.ConfigResource.Equal) {
		return false
	}

	if os.CreateBeforeDestroy != other.CreateBeforeDestroy {
		return false
	}

	return true
}

// Decode unmarshals the raw representation of the object attributes. Pass the
// implied type of the corresponding resource type schema for correct operation.
//
// Before calling Decode, the caller must check that the SchemaVersion field
// exactly equals the version number of the schema whose implied type is being
// passed, or else the result is undefined.
//
// The returned object may share internal references with the receiver and
// so the caller must not mutate the receiver any further once once this
// method is called.
func (os *ResourceInstanceObjectSrc) Decode(ty cty.Type) (*ResourceInstanceObject, error) {
	var val cty.Value
	var err error
	if os.AttrsFlat != nil {
		// Legacy mode. We'll do our best to unpick this from the flatmap.
		//
		// Note that we can only get here in unusual cases like when running
		// "tofu show" or "tofu console" against a very old state snapshot
		// created with Terraform v0.11 or earlier; in the normal plan/apply
		// path we use the provider function "UpgradeResourceState" to ask
		// the _provider_ to translate from flatmap to JSON, which can therefore
		// give better results because the provider can have awareness of its
		// own legacy encoding quirks.
		val, err = hcl2shim.HCL2ValueFromFlatmap(os.AttrsFlat, ty)
		if err != nil {
			return nil, err
		}
	} else {
		val, err = ctyjson.Unmarshal(os.AttrsJSON, ty)
		if os.TransientPathValueMarks != nil {
			val = val.MarkWithPaths(os.TransientPathValueMarks)
		}
		// Mark the value with paths if applicable
		if os.AttrSensitivePaths != nil {
			val = val.MarkWithPaths(os.AttrSensitivePaths)
		}
		if err != nil {
			return nil, err
		}
	}

	return &ResourceInstanceObject{
		Value:               val,
		Status:              os.Status,
		Dependencies:        os.Dependencies,
		Private:             os.Private,
		CreateBeforeDestroy: os.CreateBeforeDestroy,
	}, nil
}

// CompleteUpgrade creates a new ResourceInstanceObjectSrc by copying the
// metadata from the receiver and writing in the given new schema version
// and attribute value that are presumed to have resulted from upgrading
// from an older schema version.
func (os *ResourceInstanceObjectSrc) CompleteUpgrade(newAttrs cty.Value, newType cty.Type, newSchemaVersion uint64) (*ResourceInstanceObjectSrc, error) {
	new := os.DeepCopy()
	new.AttrsFlat = nil // We always use JSON after an upgrade, even if the source used flatmap

	// This is the same principle as ResourceInstanceObject.Encode, but
	// avoiding a decode/re-encode cycle because we don't have type info
	// available for the "old" attributes.
	newAttrs = cty.UnknownAsNull(newAttrs)
	src, err := ctyjson.Marshal(newAttrs, newType)
	if err != nil {
		return nil, err
	}

	new.AttrsJSON = src
	new.SchemaVersion = newSchemaVersion
	return new, nil
}
