// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// ValueWithPrivate is a [cty.Value] associated with an arbitrary "private"
// byte array that is somehow related to it.
//
// Refer to the documentation of any field or argument using this type to
// learn how the value and the byte array are related. Typically this is used
// to allow a provider to transfer some additional out-of-band information
// alongside a value that it will use when the same value is resubmitted to
// the same provider later.
type ValueWithPrivate struct {
	Value   cty.Value
	Private []byte
}

func (rt *ManagedResourceType) TestValueConformance(ctx context.Context, value cty.Value, dispAddr addrs.AbsResourceInstanceObject) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	schema, moreDiags := rt.LoadSchema(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return diags
	}

	// After transforming the state, the new value must conform to the current schema. When
	// going over RPC this is actually already ensured by the
	// marshaling/unmarshaling of the new value, but we'll check it here
	// anyway for robustness, e.g. for in-process providers.
	if errs := value.Type().TestConformance(schema.Block.ImpliedType()); len(errs) > 0 {
		for _, err := range errs {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid resource state transformation",
				fmt.Sprintf("The %s provider changed the state for %s, but produced an invalid result: %s.", rt.providerAddr, dispAddr, tfdiags.FormatError(err)),
			))
		}
	}
	return diags
}

type EncodedValue struct {
	SchemaVersion uint64
	ValueJSON     []byte
}

func (rt *ManagedResourceType) EncodeValue(ctx context.Context, value cty.Value, dispAddr addrs.AbsResourceInstanceObject) (*EncodedValue, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, moreDiags := rt.LoadSchema(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	// TODO marks handling
	valueUnmarked, _ := value.UnmarkDeep()

	src, err := ctyjson.Marshal(valueUnmarked, schema.Block.ImpliedType())
	if err != nil {
		// We just checked for type conformance above, so getting into this
		// codepath is probably a bug.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to encode result of resource state transformation",
			fmt.Sprintf("Failed to encode state for %s after resource schema upgrade: %s.", dispAddr, tfdiags.FormatError(err)),
		))
	}
	return &EncodedValue{
		SchemaVersion: uint64(schema.Version),
		ValueJSON:     src,
	}, diags
}
