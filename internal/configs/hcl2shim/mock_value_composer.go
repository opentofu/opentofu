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
		rand: rand.New(rand.NewSource(seed)), //nolint:gosec // It doesn't need to be secure.
	}
}

// ComposeBySchema composes mock value based on schema configuration. It uses
// configuration value as a baseline and populates null values with provided defaults.
// If the provided defaults doesn't contain needed fields, ComposeBySchema uses
// its own defaults. ComposeBySchema fails if schema contains dynamic types.
// ComposeBySchema produces the same result with the given input values (seed and func arguments).
// It does so by traversing schema attributes, blocks and data structure elements / fields
// in a stable way by sorting keys or elements beforehand. Then, randomized values match
// between multiple ComposeBySchema calls, because seed and random sequences are the same.
func (mvc MockValueComposer) ComposeBySchema(schema *configschema.Block, config cty.Value, defaults map[string]cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var configMap map[string]cty.Value
	var diags tfdiags.Diagnostics

	if !config.IsNull() {
		configMap = config.AsValueMap()
	}

	impliedTypes := schema.ImpliedType().AttributeTypes()

	mockAttrs, moreDiags := mvc.composeMockValueForAttributes(schema, configMap, defaults)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.NilVal, diags
	}

	mockBlocks, moreDiags := mvc.composeMockValueForBlocks(schema, configMap, defaults)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.NilVal, diags
	}

	mockValues := mockAttrs
	for k, v := range mockBlocks {
		mockValues[k] = v
	}

	for k := range defaults {
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

func (mvc MockValueComposer) composeMockValueForAttributes(schema *configschema.Block, configMap map[string]cty.Value, defaults map[string]cty.Value) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	mockAttrs := make(map[string]cty.Value)

	impliedTypes := schema.ImpliedType().AttributeTypes()

	// Stable order is important here so random values match its fields between function calls.
	for _, kv := range mapToSortedSlice(schema.Attributes) {
		k, attr := kv.k, kv.v

		// If the value present in configuration - just use it.
		if cv, ok := configMap[k]; ok && !cv.IsNull() {
			if _, ok := defaults[k]; ok {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid mock/override field `%v`", k),
					"The field is ignored since overriding configuration values is not allowed.",
				))
				continue
			}
			mockAttrs[k] = cv
			continue
		}

		// Non-computed attributes can't be generated
		// so we set them from configuration only.
		if !attr.Computed {
			mockAttrs[k] = cty.NullVal(attr.Type)
			if _, ok := defaults[k]; ok {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Non-computed field `%v` is not allowed to be overridden", k),
					"Overriding non-computed fields is not allowed, so this field cannot be processed.",
				))
			}
			continue
		}

		// If the attribute is computed and not configured,
		// we use provided value from defaults.
		if ov, ok := defaults[k]; ok {
			converted, err := convert.Convert(ov, attr.Type)
			if err != nil {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid mock/override field `%v`", k),
					fmt.Sprintf("Values provided for override / mock must match resource fields types: %v.", tfdiags.FormatError(err)),
				))
				continue
			}

			mockAttrs[k] = converted
			continue
		}

		// If there's no value in defaults, we generate our own.
		v, ok := mvc.getMockValueByType(impliedTypes[k])
		if !ok {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				"Failed to generate mock value",
				fmt.Sprintf("Mock value cannot be generated for dynamic type. Please specify the `%v` field explicitly in the configuration.", k),
			))
			continue
		}

		mockAttrs[k] = v
	}

	return mockAttrs, diags
}

func (mvc MockValueComposer) composeMockValueForBlocks(schema *configschema.Block, configMap map[string]cty.Value, defaults map[string]cty.Value) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	mockBlocks := make(map[string]cty.Value)

	impliedTypes := schema.ImpliedType().AttributeTypes()

	// Stable order is important here so random values match its fields between function calls.
	for _, kv := range mapToSortedSlice(schema.BlockTypes) {
		k, block := kv.k, kv.v

		// Checking if the config value really present for the block.
		// It should be non-null and non-empty collection.

		configVal, hasConfigVal := configMap[k]
		if hasConfigVal && configVal.IsNull() {
			hasConfigVal = false
		}

		if hasConfigVal && !configVal.IsKnown() {
			hasConfigVal = false
		}

		if hasConfigVal && configVal.Type().IsCollectionType() && configVal.LengthInt() == 0 {
			hasConfigVal = false
		}

		defaultVal, hasDefaultVal := defaults[k]
		if hasDefaultVal && !defaultVal.Type().IsObjectType() {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				fmt.Sprintf("Invalid override for block field `%v`", k),
				fmt.Sprintf("Blocks can be overridden only by objects, got `%s`", defaultVal.Type().FriendlyName()),
			))
			continue
		}

		// We must keep blocks the same as it defined in configuration,
		// so provider response validation succeeds later.
		if !hasConfigVal {
			mockBlocks[k] = block.EmptyValue()

			if hasDefaultVal {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Error,
					fmt.Sprintf("Invalid override for block field `%v`", k),
					"Cannot overridde block value, because it's not present in configuration.",
				))
				continue
			}

			continue
		}

		var blockDefaults map[string]cty.Value

		if hasDefaultVal {
			blockDefaults = defaultVal.AsValueMap()
		}

		v, moreDiags := mvc.getMockValueForBlock(impliedTypes[k], configVal, &block.Block, blockDefaults)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return nil, diags
		}

		mockBlocks[k] = v
	}

	return mockBlocks, diags
}

// getMockValueForBlock uses an object from the defaults (overrides)
// to compose each value from the block's inner collection. It recursively calls
// composeMockValueBySchema to proceed with all the inner attributes and blocks
// the same way so all the nested blocks follow the same logic.
func (mvc MockValueComposer) getMockValueForBlock(targetType cty.Type, configVal cty.Value, block *configschema.Block, defaults map[string]cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	switch {
	case targetType.IsObjectType():
		mockBlockVal, moreDiags := mvc.ComposeBySchema(block, configVal, defaults)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return cty.NilVal, diags
		}

		return mockBlockVal, diags

	case targetType.ListElementType() != nil || targetType.SetElementType() != nil:
		var mockBlockVals []cty.Value

		var iterator = configVal.ElementIterator()

		// Stable order is important here so random values match its fields between function calls.
		for iterator.Next() {
			_, blockConfigV := iterator.Element()

			mockBlockVal, moreDiags := mvc.ComposeBySchema(block, blockConfigV, defaults)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				return cty.NilVal, diags
			}

			mockBlockVals = append(mockBlockVals, mockBlockVal)
		}

		if targetType.ListElementType() != nil {
			return cty.ListVal(mockBlockVals), diags
		} else {
			return cty.SetVal(mockBlockVals), diags
		}

	case targetType.MapElementType() != nil:
		var mockBlockVals = make(map[string]cty.Value)

		var iterator = configVal.ElementIterator()

		// Stable order is important here so random values match its fields between function calls.
		for iterator.Next() {
			blockConfigK, blockConfigV := iterator.Element()

			mockBlockVal, moreDiags := mvc.ComposeBySchema(block, blockConfigV, defaults)
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

// getMockValueByType tries to generate mock cty.Value based on provided cty.Type.
// It will return non-ok response if it encounters dynamic type.
func (mvc MockValueComposer) getMockValueByType(t cty.Type) (cty.Value, bool) {
	var v cty.Value

	// just to be sure for cases when the logic below misses something
	if t.HasDynamicTypes() {
		return cty.Value{}, false
	}

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

			objV, ok := mvc.getMockValueByType(at)
			if !ok {
				return cty.Value{}, false
			}

			objVals[k] = objV
		}

		v = cty.ObjectVal(objVals)
	case t.IsTupleType():
		v = cty.EmptyTupleVal

	// dynamically typed values are not supported
	default:
		return cty.Value{}, false
	}

	return v, true
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
