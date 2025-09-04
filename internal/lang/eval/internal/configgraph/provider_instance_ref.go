// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
)

// providerInstanceTypes is a global repository of all of the provider instance
// capsule types we've generated so far in the current process.
//
// It's only valid to access this map while holding a lock on
// [providerInstanceTypesMu].
//
// There is no mechanism to discard previously-issued types. We assume that
// the number of distinct provider addresses in any single process using
// this package will be finite and relatively small.
var providerInstanceRefTypes = make(map[addrs.Provider]cty.Type)
var providerInstanceRefTypesMu sync.Mutex

type providerInstanceRefTypeMarker struct{}

// ProviderInstanceType returns the cty capsule type representing references
// to instances of a particular provider.
func ProviderInstanceRefType(provider addrs.Provider) cty.Type {
	providerInstanceRefTypesMu.Lock()
	defer providerInstanceRefTypesMu.Unlock()

	if existing, ok := providerInstanceRefTypes[provider]; ok {
		return existing
	}

	ty := cty.CapsuleWithOps("instance of "+provider.String(), reflect.TypeOf(ProviderInstance{}), &cty.CapsuleOps{
		TypeGoString: func(_ reflect.Type) string {
			return fmt.Sprintf("configgraph.ProviderInstanceRefType(%#v)", provider)
		},
		GoString: func(val any) string {
			return fmt.Sprintf("configgraph.ProviderInstanceRefValue(%#v)", val)
		},
		ExtensionData: func(key any) any {
			switch key {
			// [IsProviderInstanceRefType] relies on this to distinguish our
			// capsule types from others defined elsewhere.
			case providerInstanceRefTypeMarker{}:
				return provider
			default:
				return nil
			}
		},

		// TODO: Implement the conversion ops to give better error messages
		// when trying to use a reference to an instance of the wrong provider,
		// saying something like "have instance of FOO, but BAR is required".
		// (The default error message would just be "instance of BAR required".)

	})
	providerInstanceRefTypes[provider] = ty
	return ty
}

// ProviderInstanceRefValue returns a [cty.Value] of a capsule type produced
// by [ProviderInstanceRefType] that acts as a reference to the given provider
// instance which can then be used to send provider instance references through
// our normal expression evaluation mechanisms.
func ProviderInstanceRefValue(inst *ProviderInstance) cty.Value {
	ty := ProviderInstanceRefType(inst.ProviderAddr)
	return cty.CapsuleVal(ty, inst)
}

// ProviderInstanceFromValue attempts to extract an instance of the given
// provider from the given value, returning it if successful or returning
// an error if not.
func ProviderInstanceFromValue(v cty.Value, forProvider addrs.Provider) (Maybe[*ProviderInstance], cty.ValueMarks, error) {
	v, marks := v.UnmarkDeep()
	ty := ProviderInstanceRefType(forProvider)
	v, err := convert.Convert(v, ty)
	if err != nil {
		marks[exprs.EvalError] = struct{}{}
		return nil, marks, err
	}
	if v.IsNull() {
		marks[exprs.EvalError] = struct{}{}
		return nil, marks, errors.New("value must not be null")
	}
	if !v.IsKnown() {
		return nil, marks, nil
	}
	return Known(v.EncapsulatedValue().(*ProviderInstance)), marks, nil
}

// IsProviderInstanceRefValue returns true if the given type represents a
// reference to an instance of any provider.
func IsProviderInstanceRefType(ty cty.Type) bool {
	if !ty.IsCapsuleType() {
		return false
	}
	marker := ty.CapsuleExtensionData(providerInstanceRefTypeMarker{})
	return marker != nil
}
