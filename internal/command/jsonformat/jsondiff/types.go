// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsondiff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type Type string

const (
	Number Type = "number"
	Object Type = "object"
	Array  Type = "array"
	Bool   Type = "bool"
	String Type = "string"
	Null   Type = "null"
)

func GetType(val interface{}) Type {
	switch val.(type) {
	case []interface{}:
		return Array
	case float64:
		return Number
	case json.Number:
		return Number
	case string:
		return String
	case bool:
		return Bool
	case nil:
		return Null
	case map[string]interface{}:
		return Object
	default:
		panic(fmt.Sprintf("unrecognized json type %T", val))
	}
}

// Equal reports whether two generic JSON values are equal. Numbers compare
// lexically, matching how the JSON was decoded. Types that JSON decoding
// never produces are compared with reflect.DeepEqual as a fallback.
func Equal(a, b interface{}) bool {
	switch a := a.(type) {
	case nil:
		return b == nil
	case string:
		b, ok := b.(string)
		return ok && a == b
	case bool:
		b, ok := b.(bool)
		return ok && a == b
	case json.Number:
		b, ok := b.(json.Number)
		return ok && a == b
	case float64:
		b, ok := b.(float64)
		return ok && a == b
	case []interface{}:
		b, ok := b.([]interface{})
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !Equal(a[i], b[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		b, ok := b.(map[string]interface{})
		if !ok || len(a) != len(b) {
			return false
		}
		for key, val := range a {
			other, ok := b[key]
			if !ok || !Equal(val, other) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

// ShouldDiffMultilineStrings Checks if two values should be diffed as multiline strings.
// It checks if both values are strings, and if they have a common line, without checking for a common line, we might end up mistaking a deletion as an update.
// This function is used to determine if we should diff the elements in the slices instead of marking them as deleted and created.
// Primarily used for collections.ShouldDiffElement callback.
func ShouldDiffMultilineStrings(a, b any) bool {
	isMultilineString := false
	tA, tB := GetType(a), GetType(b)
	// If the values are both string, we check if one of them contains newlines to determine if we should diff them,
	if tA == String && tB == String {
		isMultilineString = strings.Contains(a.(string), "\n") || strings.Contains(b.(string), "\n")
	}
	// Only in case the strings have a common line we should diff them,
	if isMultilineString {
		linesA := strings.Split(a.(string), "\n")
		linesB := strings.Split(b.(string), "\n")
		for _, line := range linesA {
			for _, lineB := range linesB {
				if line == lineB {
					return true
				}
			}
		}
	}
	return false
}
