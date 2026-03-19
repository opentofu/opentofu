// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"maps"
	"slices"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func isValueMarkedUnusable(v cty.Value) bool {
	return v.HasMark(marks.Sensitive) || v.HasMark(marks.Ephemeral)
}

// ObjectValueID takes a value that is assumed to be an object representation
// of some resource instance object and attempts to heuristically find an
// attribute of it that is likely to be a unique identifier in the remote
// system that it belongs to which will be useful to the user.
//
// If such an attribute is found, its name and string value intended for
// display are returned. Both returned strings are empty if no such attribute
// exists, in which case the caller should assume that the resource instance
// address within the OpenTofu configuration is the best available identifier.
//
// This is only a best-effort sort of thing, relying on naming conventions in
// our resource type schemas. The result is not guaranteed to be unique, but
// should generally be suitable for display to an end-user anyway.
//
// This function will panic if the given value is not of an object type.
func ObjectValueID(obj cty.Value) (k, v string) {
	if obj.IsNull() || !obj.IsKnown() {
		return "", ""
	}

	atys := obj.Type().AttributeTypes()

	switch {

	case atys["id"] == cty.String:
		v := obj.GetAttr("id")
		if isValueMarkedUnusable(v) {
			break
		}
		v, _ = v.Unmark()

		if v.IsKnown() && !v.IsNull() {
			return "id", v.AsString()
		}

	case atys["name"] == cty.String:
		// "name" isn't always globally unique, but if there isn't also an
		// "id" then it _often_ is, in practice.
		v := obj.GetAttr("name")
		if isValueMarkedUnusable(v) {
			break
		}
		v, _ = v.Unmark()

		if v.IsKnown() && !v.IsNull() {
			return "name", v.AsString()
		}
	}

	return "", ""
}

// ObjectValueName takes a value that is assumed to be an object representation
// of some resource instance object and attempts to heuristically find an
// attribute of it that is likely to be a human-friendly name in the remote
// system that it belongs to which will be useful to the user.
//
// If such an attribute is found, its name and string value intended for
// display are returned. Both returned strings are empty if no such attribute
// exists, in which case the caller should assume that the resource instance
// address within the OpenTofu configuration is the best available identifier.
//
// This is only a best-effort sort of thing, relying on naming conventions in
// our resource type schemas. The result is not guaranteed to be unique, but
// should generally be suitable for display to an end-user anyway.
//
// Callers that use both ObjectValueName and ObjectValueID at the same time
// should be prepared to get the same attribute key and value from both in
// some cases, since there is overlap between the id-extraction and
// name-extraction heuristics.
//
// This function will panic if the given value is not of an object type.
func ObjectValueName(obj cty.Value) (k, v string) {
	if obj.IsNull() || !obj.IsKnown() {
		return "", ""
	}

	atys := obj.Type().AttributeTypes()

	switch {

	case atys["name"] == cty.String:
		v := obj.GetAttr("name")
		if isValueMarkedUnusable(v) {
			break
		}
		v, _ = v.Unmark()

		if v.IsKnown() && !v.IsNull() {
			return "name", v.AsString()
		}

	case atys["tags"].IsMapType() && atys["tags"].ElementType() == cty.String:
		tags := obj.GetAttr("tags")
		if tags.IsNull() || !tags.IsWhollyKnown() || isValueMarkedUnusable(tags) {
			break
		}
		tags, _ = tags.Unmark()

		switch {
		case tags.HasIndex(cty.StringVal("name")).RawEquals(cty.True):
			v := tags.Index(cty.StringVal("name"))
			if isValueMarkedUnusable(v) {
				break
			}
			v, _ = v.Unmark()

			if v.IsKnown() && !v.IsNull() {
				return "tags.name", v.AsString()
			}
		case tags.HasIndex(cty.StringVal("Name")).RawEquals(cty.True):
			// AWS-style naming convention
			v := tags.Index(cty.StringVal("Name"))
			if isValueMarkedUnusable(v) {
				break
			}
			v, _ = v.Unmark()

			if v.IsKnown() && !v.IsNull() {
				return "tags.Name", v.AsString()
			}
		}
	}

	return "", ""
}

// ObjectValueIDOrName is a convenience wrapper around both ObjectValueID
// and ObjectValueName (in that preference order) to try to extract some sort
// of human-friendly descriptive string value for an object as additional
// context about an object when it is being displayed in a compact way (where
// not all of the attributes are visible.)
//
// Just as with the two functions it wraps, it is a best-effort and may return
// two empty strings if no suitable attribute can be found for a given object.
func ObjectValueIDOrName(obj cty.Value) (k, v string) {
	k, v = ObjectValueID(obj)
	if k != "" {
		return
	}
	k, v = ObjectValueName(obj)
	return
}

// ObjectValueBestGuess is a wrapper around ObjectValueIDOrName that attempts to extract the best possible human-friendly
// string value for an object by first trying the ID and Name heuristics, and then falling back to any other string attribute if those fail.
// This is intended to be used in cases where we want to display some sort of human-friendly context about an object,
// but we don't have a specific expectation about which attribute will be the most useful for that purpose.
//
// Just as with the functions it wraps, it may return two empty strings if no suitable attribute can be found for a given object.
func ObjectValueBestGuess(obj cty.Value) (k, v string) {
	// First,attempt to fetch it from id or name attributes, which are the most likely to be unique and human-friendly, respectively.
	k, v = ObjectValueIDOrName(obj)
	if k != "" {
		return
	}

	// If we cant get anything, just return "", "" to indicate that we have no useful information about this object.
	if obj.IsNull() || !obj.IsKnown() {
		return "", ""
	}

	// Now we know we have _some_ information, we go through the rest of the attributes and
	// This is a bit of a Hail Mary, but it is better than nothing.
	attributeTypes := obj.Type().AttributeTypes()

	// Fetch whichever string attribute is first in the list that is not marked as unusable, and return it as the best effort description of this object.
	// We iterate over the attributes in sorted order to ensure that we get a deterministic result, even if the underlying map iteration order is not deterministic.
	for _, k := range slices.Sorted(maps.Keys(attributeTypes)) {
		// We specifically only want strings
		if attributeTypes[k] != cty.String {
			continue
		}

		v := obj.GetAttr(k)
		if v.IsNull() || !v.IsKnown() || isValueMarkedUnusable(v) {
			continue
		}
		v, _ = v.Unmark()

		if v.IsKnown() && !v.IsNull() {
			return k, v.AsString()
		}
	}

	// If we got here, then we have no useful information about this object, so we return "", "" to indicate that the
	// caller should skip printing this information, or attempt another way
	return "", ""
}
