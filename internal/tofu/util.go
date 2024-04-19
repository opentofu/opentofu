// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// Semaphore is a wrapper around a channel to provide
// utility methods to clarify that we are treating the
// channel as a semaphore
type Semaphore chan struct{}

// NewSemaphore creates a semaphore that allows up
// to a given limit of simultaneous acquisitions
func NewSemaphore(n int) Semaphore {
	if n <= 0 {
		panic("semaphore with limit <=0")
	}
	ch := make(chan struct{}, n)
	return Semaphore(ch)
}

// Acquire is used to acquire an available slot.
// Blocks until available.
func (s Semaphore) Acquire() {
	s <- struct{}{}
}

// TryAcquire is used to do a non-blocking acquire.
// Returns a bool indicating success
func (s Semaphore) TryAcquire() bool {
	select {
	case s <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release is used to return a slot. Acquire must
// be called as a pre-condition.
func (s Semaphore) Release() {
	select {
	case <-s:
	default:
		panic("release without an acquire")
	}
}

// composeMockValueBySchema composes mock value based on schema configuration. It uses
// configuration value as a baseline and populates null values with provided defaults.
// If the provided defaults doesn't contain needed fields, composeMockValueBySchema uses
// its own defaults. composeMockValueBySchema fails if schema contains dynamic types.
func composeMockValueBySchema(schema *configschema.Block, config cty.Value, defaults map[string]cty.Value) (
	cty.Value, tfdiags.Diagnostics) {

	mockValue := make(map[string]cty.Value)

	var configMap map[string]cty.Value
	var diags tfdiags.Diagnostics

	if !config.IsNull() {
		configMap = config.AsValueMap()
	}

	addPotentialDefaultsWarning := func(key, description string) {
		if _, ok := defaults[key]; ok {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Warning,
				fmt.Sprintf("Ignored mock/override field `%v`", key),
				description,
			))
		}

	}

	attributeTypes := schema.ImpliedType().AttributeTypes()

	for k, t := range attributeTypes {
		// If the value present in configuration - just use it.
		if cv, ok := configMap[k]; ok && !cv.IsNull() {
			mockValue[k] = cv
			addPotentialDefaultsWarning(k, "The field is ignored since overriding configuration values not allowed.")
			continue
		}

		// Computed attributes can't be generated
		// so we set them from configuration only.
		if attr, ok := schema.Attributes[k]; ok && !attr.Computed {
			mockValue[k] = cty.NullVal(attr.Type)
			addPotentialDefaultsWarning(k, "The field is ignored since overriding non-computed fields not allowed.")
			continue
		}

		// Optional blocks shouldn't be populated with mock values.
		if block, ok := schema.BlockTypes[k]; ok && block.MinItems == 0 && block.MaxItems == 0 {
			mockValue[k] = block.EmptyValue()
			addPotentialDefaultsWarning(k, "The field is ignored since overriding optional blocks not allowed.")
			continue
		}

		// If the attribute is computed and not configured,
		// we use provided value from defaults.
		if ov, ok := defaults[k]; ok {
			mockValue[k] = ov
			continue
		}

		// If there's no value in defaults, we generate our own.
		v, ok := getMockValueByType(t)
		if !ok {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				"Failed to generate mock value",
				fmt.Sprintf("Mock value cannot be generated for dynamic type. Please, specify `%v` field explicitly in configuration.", k),
			))
			continue
		}

		mockValue[k] = v
	}

	for k := range defaults {
		if _, ok := attributeTypes[k]; !ok {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Warning,
				fmt.Sprintf("Ignored mock/override field `%v`", k),
				"The field is unknown. Please, ensure it's a part of resource definition.",
			))
		}
	}

	return cty.ObjectVal(mockValue), diags
}

// getMockValueByType tries to generate mock cty.Value based on provided cty.Type.
// It will return non-ok response if it encounters dynamic type.
func getMockValueByType(t cty.Type) (cty.Value, bool) {
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
		v = cty.StringVal(getRandomAlphaNumString(8))

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

			v, ok := getMockValueByType(at)
			if !ok {
				return cty.Value{}, false
			}

			objVals[k] = v
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

func getRandomAlphaNumString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

	b := strings.Builder{}
	b.Grow(length)

	for i := 0; i < length; i++ {
		b.WriteByte(chars[rand.Intn(len(chars))])
	}

	return b.String()
}
