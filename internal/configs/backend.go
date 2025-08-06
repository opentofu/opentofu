// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// Backend represents a "backend" block inside a "terraform" block in a module
// or file.
type Backend struct {
	Type   string
	Config hcl.Body
	Eval   *StaticEvaluator

	TypeRange hcl.Range
	DeclRange hcl.Range
}

func decodeBackendBlock(block *hcl.Block) (*Backend, hcl.Diagnostics) {
	return &Backend{
		Type:      block.Labels[0],
		TypeRange: block.LabelRanges[0],
		Config:    block.Body,
		DeclRange: block.DefRange,
	}, nil
}

// Hash produces a hash value for the receiver that covers the type and the
// portions of the config that conform to the given schema.
//
// If the config does not conform to the schema then the result is not
// meaningful for comparison since it will be based on an incomplete result.
//
// As an exception, required attributes in the schema are treated as optional
// for the purpose of hashing, so that an incomplete configuration can still
// be hashed. Other errors, such as extraneous attributes, have no such special
// case.
// TODO ephemeral - check if ephemeral should be able to be used here or not. Seems that it shouldn't
// but we need to double check
func (b *Backend) Hash(ctx context.Context, schema *configschema.Block) (int, hcl.Diagnostics) {
	// Don't fail if required attributes are not set. Instead, we'll just
	// hash them as nulls.
	schema = schema.NoneRequired()

	// This is a bit of an odd workaround, but the decode below intentionally ignores
	// errors.  I don't want to try to change that at this point, but it may be worth doing
	// at some point. For now, I'm just looking to see if there are any references that are
	// not valid that the user should look at, instead of just producing an invalid backend object.
	diags := b.referenceDiagnostics(ctx, schema)

	val, _ := b.Decode(ctx, schema)
	if val == cty.NilVal {
		val = cty.UnknownVal(schema.ImpliedType())
	}

	if marks.Contains(val, marks.Sensitive) {
		return -1, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Backend config contains sensitive values",
			Detail:   "The backend configuration is stored in .terraform/terraform.tfstate as well as plan files. It is recommended to instead supply sensitive credentials via backend specific environment variables",
			Subject:  b.DeclRange.Ptr(),
		})
	}

	toHash := cty.TupleVal([]cty.Value{
		cty.StringVal(b.Type),
		val,
	})

	return toHash.Hash(), diags
}

func (b *Backend) Decode(ctx context.Context, schema *configschema.Block) (cty.Value, hcl.Diagnostics) {
	return b.Eval.DecodeBlock(ctx, b.Config, schema.DecoderSpec(), StaticIdentifier{
		Module:    addrs.RootModule,
		Subject:   fmt.Sprintf("backend.%s", b.Type),
		DeclRange: b.DeclRange,
	})
}

// This is a hack that may not be needed, but preserves the idea that invalid backends will show a cryptic error about running init during plan/apply startup.
func (b *Backend) referenceDiagnostics(ctx context.Context, schema *configschema.Block) hcl.Diagnostics {
	var diags hcl.Diagnostics

	refs, refsDiags := lang.References(addrs.ParseRef, hcldec.Variables(b.Config, schema.DecoderSpec()))
	diags = append(diags, refsDiags.ToHCL()...)
	if diags.HasErrors() {
		return diags
	}

	_, ctxDiags := b.Eval.scope(StaticIdentifier{
		Module:    addrs.RootModule,
		Subject:   fmt.Sprintf("backend.%s", b.Type),
		DeclRange: b.DeclRange,
	}).EvalContext(ctx, refs)
	diags = append(diags, ctxDiags.ToHCL()...)

	return diags
}
