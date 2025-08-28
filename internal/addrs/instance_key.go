// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// InstanceKey represents the key of an instance within an object that
// contains multiple instances due to using "count" or "for_each" arguments
// in configuration.
//
// IntKey and StringKey are the two implementations of this type. No other
// implementations are allowed. The single instance of an object that _isn't_
// using "count" or "for_each" is represented by NoKey, which is a nil
// InstanceKey.
type InstanceKey interface {
	instanceKeySigil()
	String() string

	// Value returns the cty.Value of the appropriate type for the InstanceKey
	// value.
	Value() cty.Value
}

// ParseInstanceKey returns the instance key corresponding to the given value,
// which must be known and non-null.
//
// If an unknown or null value is provided then this function will panic. This
// function is intended to deal with the values that would naturally be found
// in a hcl.TraverseIndex, which (when parsed from source, at least) can never
// contain unknown or null values.
func ParseInstanceKey(key cty.Value) (InstanceKey, error) {
	switch key.Type() {
	case cty.String:
		return StringKey(key.AsString()), nil
	case cty.Number:
		var idx int
		err := gocty.FromCtyValue(key, &idx)
		return IntKey(idx), err
	default:
		return NoKey, fmt.Errorf("either a string or an integer is required")
	}
}

// NoKey represents the absence of an InstanceKey, for the single instance
// of a configuration object that does not use "count" or "for_each" at all.
var NoKey InstanceKey

// IntKey is the InstanceKey representation representing integer indices, as
// used when the "count" argument is specified or if for_each is used with
// a sequence type.
type IntKey int

func (k IntKey) instanceKeySigil() {
}

func (k IntKey) String() string {
	return fmt.Sprintf("[%d]", int(k))
}

func (k IntKey) Value() cty.Value {
	return cty.NumberIntVal(int64(k))
}

// StringKey is the InstanceKey representation representing string indices, as
// used when the "for_each" argument is specified with a map or object type.
type StringKey string

func (k StringKey) instanceKeySigil() {
}

func (k StringKey) String() string {
	// We use HCL's quoting syntax here so that we can in principle parse
	// an address constructed by this package as if it were an HCL
	// traversal, even if the string contains HCL's own metacharacters.
	return fmt.Sprintf("[%s]", toHCLQuotedString(string(k)))
}

func (k StringKey) Value() cty.Value {
	return cty.StringVal(string(k))
}

// WildcardKey is a special [InstanceKey] type used to represent zero or more
// _hypothetical_ instances in unusual situations where we don't yet have
// enough information to determine which instance keys exist.
//
// This can be used for [ModuleInstance] and [ResourceInstance] keys, but only
// in situations that are documented as being able to handle placeholder
// addresses for not-yet-expanded objects. No _actual_ object has an address
// with an instance key of this type.
type WildcardKey [1]InstanceKeyType

func (k WildcardKey) instanceKeySigil() {}

// ExpectedKeyType returns the type of key that this wildcard is acting as
// a placeholder for.
//
// If this returns a concrete key type (not [UnknownKeyType]) then we do at
// least know that all of the potential instance keys are of that type,
// even though we don't yet know their values or number.
func (k WildcardKey) ExpectedKeyType() InstanceKeyType {
	return k[0]
}

func (k WildcardKey) String() string {
	// There isn't any real way to write down a wildcard key because they
	// represent an absense of information rather than something directly
	// configured, but as a compromise we'll use something resembling HCL's
	// "splat expression" syntax since it's at least hopefully somewhat
	// familiar to OpenTofu users, and * is a character commonly used
	// to represent wildcards in other systems.
	return "[*]"
}

// Value returns an unknown value that's possibly constrained to be either
// a number or string if we at least know what instance key type we're
// expecting.
func (k WildcardKey) Value() cty.Value {
	switch k.ExpectedKeyType() {
	case NoKeyType:
		// This case represents an object using the "enabled" meta-argument
		// with an unknown value and so there isn't really any sensible
		// answer here because we're representing either zero or one instances
		// with no key at all, but we'll arbitrarily just return DynamicVal
		// as a placeholder.
		return cty.DynamicVal
	case IntKeyType:
		return cty.UnknownVal(cty.Number).RefineNotNull()
	case StringKeyType:
		return cty.UnknownVal(cty.String).RefineNotNull()
	default: // (only UnknownKeyType should be left to handle here)
		// If we don't even know what type of instance key we're expecting
		// then we can't really say anything about the value at all.
		return cty.DynamicVal
	}
}

// InstanceKeyLess returns true if the first given instance key i should sort
// before the second key j, and false otherwise.
func InstanceKeyLess(i, j InstanceKey) bool {
	iTy := instanceKeyType(i)
	jTy := instanceKeyType(j)

	switch {
	case i == j:
		return false
	case i == NoKey:
		return true
	case j == NoKey:
		return false
	case iTy != jTy:
		// The ordering here is arbitrary except that we want NoKeyType
		// to sort before the others, so we'll just use the enum values
		// of InstanceKeyType here (where NoKey is zero, sorting before
		// any other).
		return uint32(iTy) < uint32(jTy)
	case iTy == IntKeyType:
		return int(i.(IntKey)) < int(j.(IntKey))
	case iTy == StringKeyType:
		return string(i.(StringKey)) < string(j.(StringKey))
	default:
		// Shouldn't be possible to get down here in practice, since the
		// above is exhaustive.
		return false
	}
}

func instanceKeyType(k InstanceKey) InstanceKeyType {
	if _, ok := k.(StringKey); ok {
		return StringKeyType
	}
	if _, ok := k.(IntKey); ok {
		return IntKeyType
	}
	if k, ok := k.(WildcardKey); ok {
		// Because WildcardKey values are placeholders for instance keys
		// of some other type rather than keys themselves, there is no
		// InstanceKeyType value representing "wildcard" and instead the
		// key type of a wildcard key is whatever it's a placeholder for.
		return k.ExpectedKeyType()
	}
	return NoKeyType
}

// InstanceKeyType represents the different types of instance key that are
// supported. Usually it is sufficient to simply type-assert an InstanceKey
// value to either IntKey or StringKey, but this type and its values can be
// used to represent the types themselves, rather than specific values
// of those types.
type InstanceKeyType rune

const (
	NoKeyType     InstanceKeyType = 0
	IntKeyType    InstanceKeyType = 'I'
	StringKeyType InstanceKeyType = 'S'

	// UnknownKeyType is a special [InstanceKeyType] used only with
	// [WildcardKey] in situations where we don't even know what type of
	// key we're expecting.
	UnknownKeyType InstanceKeyType = '?'
)

// toHCLQuotedString is a helper which formats the given string in a way that
// HCL's expression parser would treat as a quoted string template.
//
// This includes:
//   - Adding quote marks at the start and the end.
//   - Using backslash escapes as needed for characters that cannot be represented directly.
//   - Escaping anything that would be treated as a template interpolation or control sequence.
func toHCLQuotedString(s string) string {
	// This is an adaptation of a similar function inside the hclwrite package,
	// inlined here because hclwrite's version generates HCL tokens but we
	// only need normal strings.
	if len(s) == 0 {
		return `""`
	}
	var buf strings.Builder
	buf.WriteByte('"')
	for i, r := range s {
		switch r {
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '$', '%':
			buf.WriteRune(r)
			remain := s[i+1:]
			if len(remain) > 0 && remain[0] == '{' {
				// Double up our template introducer symbol to escape it.
				buf.WriteRune(r)
			}
		default:
			if !unicode.IsPrint(r) {
				var fmted string
				if r < 65536 {
					fmted = fmt.Sprintf("\\u%04x", r)
				} else {
					fmted = fmt.Sprintf("\\U%08x", r)
				}
				buf.WriteString(fmted)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
	return buf.String()
}
