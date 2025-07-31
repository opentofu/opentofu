// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package plugintofu

import (
	"encoding/json"
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/msgpack"
)

// convertCtyToInterface converts a cty.Value to interface{} for MessagePack transmission
func convertCtyToInterface(val cty.Value) (interface{}, error) {
	if val.IsNull() {
		return nil, nil
	}

	switch val.Type() {
	case cty.String:
		return val.AsString(), nil
	case cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i, _ := bf.Int64()
			return i, nil
		}
		f, _ := bf.Float64()
		return f, nil
	case cty.Bool:
		return val.True(), nil
	default:
		// For complex types, use MessagePack encoding
		bytes, err := msgpack.Marshal(val, val.Type())
		if err != nil {
			return nil, fmt.Errorf("failed to marshal cty value: %w", err)
		}

		// Convert to interface{} via JSON for MessagePack compatibility
		var result interface{}
		if err := json.Unmarshal(bytes, &result); err != nil {
			return nil, fmt.Errorf("failed to convert cty value: %w", err)
		}
		return result, nil
	}
}

// convertInterfaceToCty converts interface{} from MessagePack response back to cty.Value
func convertInterfaceToCty(data interface{}, expectedType cty.Type) (cty.Value, error) {
	if data == nil {
		return cty.NullVal(expectedType), nil
	}

	switch expectedType {
	case cty.String:
		if str, ok := data.(string); ok {
			return cty.StringVal(str), nil
		}
		return cty.StringVal(fmt.Sprintf("%v", data)), nil

	case cty.Number:
		switch v := data.(type) {
		case int64:
			return cty.NumberIntVal(v), nil
		case int:
			return cty.NumberIntVal(int64(v)), nil
		case float64:
			return cty.NumberFloatVal(v), nil
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return cty.NumberIntVal(i), nil
			}
			if f, err := v.Float64(); err == nil {
				return cty.NumberFloatVal(f), nil
			}
		}
		return cty.NumberIntVal(0), fmt.Errorf("cannot convert %T to number", data)

	case cty.Bool:
		if b, ok := data.(bool); ok {
			return cty.BoolVal(b), nil
		}
		return cty.BoolVal(false), fmt.Errorf("cannot convert %T to bool", data)

	default:
		// For complex types, try MessagePack decoding
		bytes, err := json.Marshal(data)
		if err != nil {
			return cty.NilVal, fmt.Errorf("failed to marshal interface: %w", err)
		}

		val, err := msgpack.Unmarshal(bytes, expectedType)
		if err != nil {
			return cty.NilVal, fmt.Errorf("failed to unmarshal to cty: %w", err)
		}
		return val, nil
	}
}

// convertCtyArgsToMap converts []cty.Value function arguments to map[string]interface{}
// for MessagePack transmission, using parameter names from the function spec
func convertCtyArgsToMap(args []cty.Value, paramNames []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for i, arg := range args {
		var paramName string
		if i < len(paramNames) {
			paramName = paramNames[i]
		} else {
			paramName = fmt.Sprintf("arg%d", i) // Fallback name
		}

		converted, err := convertCtyToInterface(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert argument %s: %w", paramName, err)
		}

		result[paramName] = converted
	}

	return result, nil
}

// getFromMap is a generic helper function to safely get a typed value from a map
//
// This exists because when we unmarshal MessagePack data using msgpack.Unmarshal,
// Go doesn't know the exact types of nested values, so everything becomes interface{}.
// Without this helper, we'd need verbose type assertions everywhere,
// With this helper, we get clean, safe type extraction:
//
// Returns (value, success) where success indicates if the key existed and type assertion succeeded.
func getFromMap[T any](m map[string]interface{}, key string) (value T, success bool) {
	var zero T
	if val, ok := m[key]; ok {
		if typed, ok := val.(T); ok {
			return typed, true
		}
	}
	return zero, false
}

func getStringFromMap(m map[string]interface{}, key string) string {
	val, _ := getFromMap[string](m, key)
	return val
}

// Simple type parser for basic types
// TODO: use enums for nice typing here and find it elsewhere in the codebase
func parseTypeFromString(typeStr string) cty.Type {
	switch typeStr {
	case "string":
		return cty.String
	case "number":
		return cty.Number
	case "bool", "boolean":
		return cty.Bool
	case "list":
		return cty.List(cty.String) // Default to list of strings
	case "map":
		return cty.Map(cty.String) // Default to map of strings
	default:
		return cty.String // Default fallback
	}
}
