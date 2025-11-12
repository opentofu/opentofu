package hcl2shim

import (
	"cmp"
	"fmt"
	"math/rand"
	"slices"
	"strings"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// MockValueComposer provides different ways to generate mock values based on
// config schema, attributes, blocks and cty types in general.
type MockValueComposer struct {
	rand *rand.Rand
}

func NewMockValueComposer(seed int64) MockValueComposer {
	return MockValueComposer{
		rand: rand.New(rand.NewSource(seed)),
	}
}

// ComposeBySchema composes mock value based on schema configuration. It uses
// configuration value as a baseline and populates null values with provided overrides.
// If the provided overrides doesn't contain needed fields, ComposeBySchema uses
// its own overrides. ComposeBySchema fails if schema contains dynamic types.
// ComposeBySchema produces the same result with the given input values (seed and func arguments).
// It does so by traversing schema attributes, blocks and data structure elements / fields
// in a stable way by sorting keys or elements beforehand. Then, randomized values match
// between multiple ComposeBySchema calls, because seed and random sequences are the same.
func (mvc MockValueComposer) ComposeBySchema(schema *configschema.Block, config cty.Value, overrides map[string]cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var configMap map[string]cty.Value
	var diags tfdiags.Diagnostics

	if !config.IsNull() {
		configMap = config.AsValueMap()
	}

	impliedTypes := schema.ImpliedType().AttributeTypes()

	mockAttrs, moreDiags := mvc.composeMockValueForAttributes(schema.Attributes, configMap, overrides)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.NilVal, diags
	}

	mockBlocks, moreDiags := mvc.composeMockValueForBlocks(schema, configMap, overrides)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.NilVal, diags
	}

	mockValues := mockAttrs
	for k, v := range mockBlocks {
		mockValues[k] = v
	}

	for k := range overrides {
		if _, ok := impliedTypes[k]; !ok {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				fmt.Sprintf("Invalid override for block field `%v`", k),
				"The field is unknown. Please, ensure it is a part of resource definition.",
			))
		}
	}

	return cty.ObjectVal(mockValues), diags
}

/*
composeMockValueForAttributes follows the truth table below for generating an output value, given schema and inputs:
Required  Optional  Computed  Has config  Has Override  Result
t         f         f         t           t             Error - Not Allowed to override config
t         f         f         t           f             Config
t         f         f         f           t             Error - Required field in config not provided
t         f         f         f           f             Error - Required field in config not provided
f         t         f         t           t             Error - Not Allowed to override config
f         t         f         t           f             Config
f         t         f         f           t             Override Value
f         t         f         f           f             NullVal of the attribute type
f         t         t         t           t             Error - Not Allowed to override config
f         t         t         t           f             Config
f         t         t         f           t             Override
f         t         t         f           f             GenVal
f         f         t         t           t             Error - Not Allowed to override config
f         f         t         t           f             Error - Config not allowed here
f         f         t         f           t             Override
f         f         t         f           f             GenVal
*/
func (mvc MockValueComposer) composeMockValueForAttributes(attrs map[string]*configschema.Attribute, configMap map[string]cty.Value, overrides map[string]cty.Value) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	mockAttrs := make(map[string]cty.Value)

	// Stable order is important here so random values match its fields between function calls.
	for _, kv := range mapToSortedSlice(attrs) {
		k, attr := kv.k, kv.v

		if attr.NestedType != nil && attr.NestedType.Nesting == configschema.NestingGroup {
			// This should not be possible to hit.  Neither tofu or the provider framework will allow
			// NestingGroup in here.  However, this could change at some point and we want to be prepared for it.
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				fmt.Sprintf("Unsupported field `%v` in attribute mocking", k),
				"Overriding non-computed fields is not allowed, so this field cannot be processed.",
			))
		}

		overrideValue, hasOverride := overrides[k]
		configValue, hasConfig := configMap[k]
		// If the configured value is null, it is the same as not specified
		hasConfig = hasConfig && !configValue.IsNull()

		var ovConvert cty.Value

		// Validation of overridden values
		if hasOverride {
			if hasConfig {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid mock/override field `%v`", k),
					"The field is ignored since overriding configuration values is not allowed.",
				))
			}
			if !attr.Computed {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Non-computed field `%v` is not allowed to be overridden", k),
					"Overriding non-computed fields is not allowed, so this field cannot be processed.",
				))
			}

			var err error
			ovConvert, err = convert.Convert(overrideValue, attr.ImpliedType())
			if err != nil {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid mock/override field `%v`", k),
					fmt.Sprintf("Values provided for override / mock must match resource fields types: %v.", tfdiags.FormatError(err)),
				))
			}
		}

		// Determine the value
		if attr.Required {
			// Value from configuration only
			if !hasConfig {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid mock/override field `%v`", k),
					"Required field in configuration not provided",
				))
			}

			mockAttrs[k] = configValue
		} else if attr.Optional {
			if hasConfig {
				// Value from configuration
				mockAttrs[k] = configValue
			} else if hasOverride {
				mockAttrs[k] = ovConvert
			} else if attr.Computed {
				mockAttrs[k] = mvc.getMockValueByType(attr.ImpliedType())
			} else {
				// Null value
				// NOTE: this does not handle configschema.NestedGroup correctly, but
				// at this time there is no possible way for providers to specify NestedGroup.
				mockAttrs[k] = cty.NullVal(attr.ImpliedType())
			}
		} else if attr.Computed {
			// Value from provider only
			if hasConfig {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid mock/override field `%v`", k),
					"Config value can not be specified for computed field",
				))
			}
			if hasOverride {
				mockAttrs[k] = ovConvert
			} else {
				mockAttrs[k] = mvc.getMockValueByType(attr.ImpliedType())
			}
		} else {
			panic("invalid schema: none of configschema.Attribute.Required/Computed/Optional set on " + k)
		}
	}

	return mockAttrs, diags
}

func (mvc MockValueComposer) composeMockValueForBlocks(schema *configschema.Block, configMap map[string]cty.Value, overrides map[string]cty.Value) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	mockBlocks := make(map[string]cty.Value)

	impliedTypes := schema.ImpliedType().AttributeTypes()

	// Stable order is important here so random values match its fields between function calls.
	for _, kv := range mapToSortedSlice(schema.BlockTypes) {
		k, block := kv.k, kv.v

		// Checking if the config value really present for the block.
		// It should be non-null and non-empty collection.

		configValue, hasConfig := configMap[k]
		hasConfig = hasConfig && !configValue.IsNull() && configValue.IsKnown()

		emptyConfig := configValue.Type().IsCollectionType() && configValue.LengthInt() == 0
		hasConfig = hasConfig && !emptyConfig

		overrideValue, hasOverride := overrides[k]
		if hasOverride && !overrideValue.Type().IsObjectType() {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				fmt.Sprintf("Invalid override for block field `%v`", k),
				fmt.Sprintf("Blocks can be overridden only by objects, got `%s`", overrideValue.Type().FriendlyName()),
			))
			continue
		}

		// We must keep blocks the same as it defined in configuration,
		// so provider response validation succeeds later.
		if !hasConfig {
			mockBlocks[k] = block.EmptyValue()

			if hasOverride {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid override for block field `%v`", k),
					"Cannot override block value, because it's not present in configuration.",
				))
				continue
			}

			continue
		}

		var blockOverrides map[string]cty.Value

		if hasOverride {
			blockOverrides = overrideValue.AsValueMap()
		}

		v, moreDiags := mvc.getMockValueForBlock(impliedTypes[k], configValue, &block.Block, blockOverrides)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return nil, diags
		}

		mockBlocks[k] = v
	}

	return mockBlocks, diags
}

// getMockValueForBlock uses an object from the overrides
// to compose each value from the block's inner collection. It recursively calls
// composeMockValueBySchema to proceed with all the inner attributes and blocks
// the same way so all the nested blocks follow the same logic.
func (mvc MockValueComposer) getMockValueForBlock(targetType cty.Type, configValue cty.Value, block *configschema.Block, overrides map[string]cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	switch {
	case targetType.IsObjectType():
		mockBlockVal, moreDiags := mvc.ComposeBySchema(block, configValue, overrides)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return cty.NilVal, diags
		}

		return mockBlockVal, diags

	case targetType.ListElementType() != nil || targetType.SetElementType() != nil:
		var mockBlockVals []cty.Value

		var iterator = configValue.ElementIterator()

		// Stable order is important here so random values match its fields between function calls.
		for iterator.Next() {
			_, blockConfigV := iterator.Element()

			mockBlockVal, moreDiags := mvc.ComposeBySchema(block, blockConfigV, overrides)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				return cty.NilVal, diags
			}

			mockBlockVals = append(mockBlockVals, mockBlockVal)
		}

		if targetType.ListElementType() != nil {
			return cty.ListVal(mockBlockVals), diags
		}
		return cty.SetVal(mockBlockVals), diags

	case targetType.MapElementType() != nil:
		var mockBlockVals = make(map[string]cty.Value)

		var iterator = configValue.ElementIterator()

		// Stable order is important here so random values match its fields between function calls.
		for iterator.Next() {
			blockConfigK, blockConfigV := iterator.Element()

			mockBlockVal, moreDiags := mvc.ComposeBySchema(block, blockConfigV, overrides)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				return cty.NilVal, diags
			}

			mockBlockVals[blockConfigK.AsString()] = mockBlockVal
		}

		return cty.MapVal(mockBlockVals), diags

	default:
		// Shouldn't happen as long as blocks are represented by lists / maps / sets / objs.
		return cty.NilVal, diags.Append(tfdiags.WholeContainingBody(
			tfdiags.Error,
			fmt.Sprintf("Unexpected block type: %v", targetType.FriendlyName()),
			"Failed to generate mock value for this block type. Please, report it as an issue at OpenTofu repository, since it's not expected.",
		))
	}
}

// getMockValueByType generates mock cty.Value based on provided cty.Type.
func (mvc MockValueComposer) getMockValueByType(t cty.Type) cty.Value {
	var v cty.Value

	switch {
	// primitives
	case t.Equals(cty.Number):
		v = cty.Zero
	case t.Equals(cty.Bool):
		v = cty.False
	case t.Equals(cty.String):
		v = cty.StringVal(mvc.getMockString())

	// collections
	case t.ListElementType() != nil:
		v = cty.ListValEmpty(*t.ListElementType())
	case t.MapElementType() != nil:
		v = cty.MapValEmpty(*t.MapElementType())
	case t.SetElementType() != nil:
		v = cty.SetValEmpty(*t.SetElementType())

	// structural
	case t.IsObjectType():
		objVals := make(map[string]cty.Value)

		// Populate the object with mock values. Stable order is important here
		// so random values match its fields between function calls.
		for _, kv := range mapToSortedSlice(t.AttributeTypes()) {
			k, at := kv.k, kv.v

			if t.AttributeOptional(k) {
				continue
			}

			objVals[k] = mvc.getMockValueByType(at)
		}

		v = cty.ObjectVal(objVals)
	case t.IsTupleType():
		v = cty.EmptyTupleVal

	// dynamically typed values
	default:
		v = cty.NullVal(cty.DynamicPseudoType)
	}

	return v
}

func (mvc MockValueComposer) getMockString() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

	const minLength, maxLength = 4, 16

	length := mvc.rand.Intn(maxLength-minLength) + minLength

	b := strings.Builder{}
	b.Grow(length)

	for i := 0; i < length; i++ {
		b.WriteByte(chars[mvc.rand.Intn(len(chars))])
	}

	return b.String()
}

type keyValue[K cmp.Ordered, V any] struct {
	k K
	v V
}

// mapToSortedSlice makes it possible to iterate over map in a stable manner.
func mapToSortedSlice[K cmp.Ordered, V any](m map[K]V) []keyValue[K, V] {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	s := make([]keyValue[K, V], 0, len(m))
	for _, k := range keys {
		s = append(s, keyValue[K, V]{k, m[k]})
	}

	return s
}
