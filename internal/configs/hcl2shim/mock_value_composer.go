package hcl2shim

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// ComposeMockValueBySchema composes mock value based on schema configuration. It uses
// configuration value as a baseline and populates null values with provided defaults.
// If the provided defaults doesn't contain needed fields, ComposeMockValueBySchema uses
// its own defaults. ComposeMockValueBySchema fails if schema contains dynamic types.
func ComposeMockValueBySchema(schema *configschema.Block, config cty.Value, defaults map[string]cty.Value) (
	cty.Value, tfdiags.Diagnostics) {
	return mockValueComposer{}.composeMockValueBySchema(schema, config, defaults)
}

type mockValueComposer struct {
	getMockStringOverride func() string
}

func (mvc mockValueComposer) getMockString() string {
	f := getRandomAlphaNumString

	if mvc.getMockStringOverride != nil {
		f = mvc.getMockStringOverride
	}

	return f()
}

func (mvc mockValueComposer) composeMockValueBySchema(schema *configschema.Block, config cty.Value, defaults map[string]cty.Value) (cty.Value, tfdiags.Diagnostics) {
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
				tfdiags.Warning,
				fmt.Sprintf("Ignored mock/override field `%v`", k),
				"The field is unknown. Please, ensure it is a part of resource definition.",
			))
		}
	}

	return cty.ObjectVal(mockValues), diags
}

func (mvc mockValueComposer) composeMockValueForAttributes(schema *configschema.Block, configMap map[string]cty.Value, defaults map[string]cty.Value) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	addPotentialDefaultsWarning := func(key, description string) {
		if _, ok := defaults[key]; ok {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Warning,
				fmt.Sprintf("Ignored mock/override field `%v`", key),
				description,
			))
		}
	}

	mockAttrs := make(map[string]cty.Value)

	impliedTypes := schema.ImpliedType().AttributeTypes()

	for k, attr := range schema.Attributes {
		// If the value present in configuration - just use it.
		if cv, ok := configMap[k]; ok && !cv.IsNull() {
			mockAttrs[k] = cv
			addPotentialDefaultsWarning(k, "The field is ignored since overriding configuration values is not allowed.")
			continue
		}

		// Non-computed attributes can't be generated
		// so we set them from configuration only.
		if !attr.Computed {
			mockAttrs[k] = cty.NullVal(attr.Type)
			addPotentialDefaultsWarning(k, "The field is ignored since overriding non-computed fields is not allowed.")
			continue
		}

		// If the attribute is computed and not configured,
		// we use provided value from defaults.
		if ov, ok := defaults[k]; ok {
			typeConformanceErrs := ov.Type().TestConformance(attr.Type)
			if len(typeConformanceErrs) == 0 {
				mockAttrs[k] = ov
				continue
			}

			for _, err := range typeConformanceErrs {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Warning,
					fmt.Sprintf("Ignored mock/override field `%v`", k),
					fmt.Sprintf("Values provided for override / mock must match resource fields types: %v.", err),
				))
			}
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

func (mvc mockValueComposer) composeMockValueForBlocks(schema *configschema.Block, configMap map[string]cty.Value, defaults map[string]cty.Value) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	mockBlocks := make(map[string]cty.Value)

	impliedTypes := schema.ImpliedType().AttributeTypes()

	for k, block := range schema.BlockTypes {
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
			hasDefaultVal = false
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Warning,
				fmt.Sprintf("Ignored mock/override field `%v`", k),
				fmt.Sprintf("Blocks can be overridden only by objects, got `%s`", defaultVal.Type().FriendlyName()),
			))
		}

		// We must keep blocks the same as it defined in configuration,
		// so provider response validation succeeds later.
		if !hasConfigVal {
			mockBlocks[k] = block.EmptyValue()

			if hasDefaultVal {
				diags = diags.Append(tfdiags.WholeContainingBody(
					tfdiags.Warning,
					fmt.Sprintf("Ignored mock/override field `%v`", k),
					"Cannot overridde block value, because it's not present in configuration.",
				))
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
// to compose each value from the block's inner collection. It recursevily calls
// composeMockValueBySchema to proceed with all the inner attributes and blocks
// the same way so all the nested blocks follow the same logic.
func (mvc mockValueComposer) getMockValueForBlock(targetType cty.Type, configVal cty.Value, block *configschema.Block, defaults map[string]cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	switch {
	case targetType.IsObjectType():
		mockBlockVal, moreDiags := mvc.composeMockValueBySchema(block, configVal, defaults)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return cty.NilVal, diags
		}

		return mockBlockVal, diags

	case targetType.ListElementType() != nil || targetType.SetElementType() != nil:
		var mockBlockVals []cty.Value

		var iterator = configVal.ElementIterator()

		for iterator.Next() {
			_, blockConfigV := iterator.Element()

			mockBlockVal, moreDiags := mvc.composeMockValueBySchema(block, blockConfigV, defaults)
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

		for iterator.Next() {
			blockConfigK, blockConfigV := iterator.Element()

			mockBlockVal, moreDiags := mvc.composeMockValueBySchema(block, blockConfigV, defaults)
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
func (mvc mockValueComposer) getMockValueByType(t cty.Type) (cty.Value, bool) {
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

		// populate the object with mock values
		for k, at := range t.AttributeTypes() {
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

func getRandomAlphaNumString() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

	const minLength, maxLength = 4, 16

	length := rand.Intn(maxLength-minLength) + minLength //nolint:gosec // It doesn't need to be secure.

	b := strings.Builder{}
	b.Grow(length)

	for i := 0; i < length; i++ {
		b.WriteByte(chars[rand.Intn(len(chars))]) //nolint:gosec // It doesn't need to be secure.
	}

	return b.String()
}
