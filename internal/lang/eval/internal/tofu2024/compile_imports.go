// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"
)

func compileImports(
	ctx context.Context,
	declScope exprs.Scope,
	moduleProviders configgraph.CompileProviderConfigRef,
	providersSchema evalglue.ProvidersSchema,
	importConfigs []*configs.Import,
) []*configgraph.Import {
	var imports []*configgraph.Import
	for _, config := range importConfigs {
		imports = append(imports, &configgraph.Import{
			Addr:             config.StaticTo,
			DeclRange:        tfdiags.SourceRangeFromHCL(config.DeclRange),
			InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, nil, nil, dependsOn{}),
			CompileImportInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ImportInstance {
				localScope := instanceLocalScope(declScope, repData)

				instance := &configgraph.ImportInstance{
					AddrFunc: func(ctx context.Context) (addrs.AbsResourceInstance, tfdiags.Diagnostics) {
						return evaluateImportAddress(ctx, config.To, localScope)
					},
					DeclRange:              tfdiags.SourceRangeFromHCL(config.DeclRange),
					Provider:               config.Provider,
					ProviderInstanceValuer: compileProviderConfigRef(ctx, moduleProviders, config.ProviderConfigAddr(), config.ProviderConfigRef, localScope),
				}

				if config.ID != nil {
					instance.IDFunc = func(ctx context.Context) (string, tfdiags.Diagnostics) {
						return evaluateImportIdExpression(ctx, config.ID, localScope)
					}
				}
				if config.Identity != nil {
					instance.IdentityFunc = func(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
						identitySchema, diags := getIdentitySchema(
							ctx, providersSchema,
							config.Provider,
							config.StaticTo.Resource.Type,
							config.Identity.Range(),
							config.StaticTo.String(),
						)
						if diags.HasErrors() {
							return cty.NilVal, diags
						}

						resourceIdentityType := identitySchema.SpecType()
						importIdentity, evalDiags := evaluateImportIdentityExpression(ctx, config.Identity, localScope, resourceIdentityType)
						diags = diags.Append(evalDiags)
						return importIdentity, diags
					}
				}

				return instance
			},
		})
	}
	return imports
}

// Ported from tofu/context_import.go

// getIdentitySchema resolves the identity schema for a resource type by looking it up
// from the provider schema. It is used during both validation and resolution of identity-based
// imports to avoid duplicating the provider lookup and schema existence checks.
// Returns nil with diagnostics if the provider schema cannot be fetched or if the
// resource type does not support identity-based import.
func getIdentitySchema(
	ctx context.Context,
	providersSchema evalglue.ProvidersSchema,
	provider addrs.Provider,
	resourceType string,
	identityRange hcl.Range,
	subjectStr string,
) (*configschema.Object, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// We are assuming that the provider schema should have the resource identity schema attached here,
	// so we need to look up the provider schema first
	resourceSchema, schemaDiags := providersSchema.ResourceTypeSchema(ctx, provider, addrs.ManagedResourceMode, resourceType)
	diags = diags.Append(schemaDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	if resourceSchema == nil || resourceSchema.IdentitySchema == nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to determine identity schema for import identity",
			Detail:   fmt.Sprintf("The provider %q does not provide an identity schema for the resource type %q, which is required when trying to import the resource %q using identity-based import. Please ensure the resource type supports identity-based import.", provider, resourceType, subjectStr),
			Subject:  identityRange.Ptr(),
		})
		return nil, diags
	}

	return resourceSchema.IdentitySchema, diags
}

// evaluateImportExpression is a generic function that evaluates an import expression (id or identity)
// and performs common validation checks. It returns the evaluated cty.Value.
// When allowUnknown is true, unknown values are permitted (used during validation phase).
func evaluateImportExpression(
	ctx context.Context,
	expr hcl.Expression,
	scope exprs.Scope,
	wantType cty.Type,
	fieldName string,
	allowUnknown bool,
) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if expr == nil {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid import %s argument", fieldName),
			Detail:   fmt.Sprintf("The import %s cannot be null.", fieldName),
			Subject:  nil,
		})
	}

	// evaluate the import expression and take into consideration the for_each key (if exists)
	val, evalDiags := exprs.NewClosure(exprs.EvalableHCLExpression(expr), scope).Value(ctx)
	diags = diags.Append(evalDiags)

	if wantType != cty.DynamicPseudoType {
		var convErr error
		val, convErr = convert.Convert(val, wantType)
		if convErr != nil {
			val = cty.UnknownVal(wantType)
			diags = diags.Append(&hcl.Diagnostic{
				Severity:   hcl.DiagError,
				Summary:    "Incorrect value type",
				Detail:     fmt.Sprintf("Invalid expression value: %s.", tfdiags.FormatError(convErr)),
				Subject:    expr.Range().Ptr(),
				Expression: expr,
				//EvalContext: hclCtx,
			})
		}
	}

	if diags.HasErrors() {
		return cty.NilVal, diags
	}

	if val.IsNull() {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid import %s argument", fieldName),
			Detail:   fmt.Sprintf("The import %s cannot be null.", fieldName),
			Subject:  expr.Range().Ptr(),
		})
	}

	if !val.IsWhollyKnown() {
		if allowUnknown {
			return cty.NilVal, diags
		}
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid import %s argument", fieldName),
			Detail:   fmt.Sprintf(`The import block "%s" argument depends on resource attributes that cannot be determined until apply, so OpenTofu cannot plan to import this resource.`, fieldName),
			Subject:  expr.Range().Ptr(),
			Extra:    evalchecks.DiagnosticCausedByUnknown(true),
		})
	}

	if marks.Contains(val, marks.Sensitive) {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid import %s argument", fieldName),
			Detail:   fmt.Sprintf("The import %s cannot be sensitive.", fieldName),
			Subject:  expr.Range().Ptr(),
		})
	}

	if marks.Contains(val, marks.Ephemeral) {
		return cty.NilVal, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid import %s argument", fieldName),
			Detail:   fmt.Sprintf("The import %s cannot be ephemeral.", fieldName),
			Subject:  expr.Range().Ptr(),
		})
	}

	return val, diags
}

func evaluateImportIdExpression(ctx context.Context, expr hcl.Expression, scope exprs.Scope) (string, tfdiags.Diagnostics) {
	val, diags := evaluateImportExpression(ctx, expr, scope, cty.String, "id", false)
	if diags.HasErrors() {
		return "", diags
	}

	// Drop marks here, probably not needed for dep tracking
	val, _ = val.Unmark()

	var importId string
	err := gocty.FromCtyValue(val, &importId)
	if err != nil {
		return "", diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import ID argument",
			Detail:   fmt.Sprintf("The import ID value is unsuitable: %s.", err),
			Subject:  expr.Range().Ptr(),
		})
	}

	return importId, diags
}

func evaluateImportIdentityExpression(ctx context.Context, expr hcl.Expression, scope exprs.Scope, wantType cty.Type) (cty.Value, tfdiags.Diagnostics) {
	val, diags := evaluateImportExpression(ctx, expr, scope, wantType, "identity", false)
	return val, diags
}

// EvaluateImportAddress takes the raw reference expression of the import address
// from the config, and returns the evaluated address addrs.AbsResourceInstance
//
// The implementation is inspired by config.AbsTraversalForImportToExpr, but this time we can evaluate the expression
// in the indexes of expressions. If we encounter a hclsyntax.IndexExpr, we can evaluate the Key expression and create
// an Index Traversal, adding it to the Traverser
func evaluateImportAddress(ctx context.Context, expr hcl.Expression, scope exprs.Scope) (addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	traversal, diags := traversalForImportExpr(ctx, expr, scope)
	if diags.HasErrors() {
		return addrs.AbsResourceInstance{}, diags
	}

	return addrs.ParseAbsResourceInstance(traversal)
}

func traversalForImportExpr(ctx context.Context, expr hcl.Expression, scope exprs.Scope) (hcl.Traversal, tfdiags.Diagnostics) {
	var traversal hcl.Traversal
	var diags tfdiags.Diagnostics

	switch e := expr.(type) {
	case *hclsyntax.IndexExpr:
		t, d := traversalForImportExpr(ctx, e.Collection, scope)
		diags = diags.Append(d)
		traversal = append(traversal, t...)

		tIndex, dIndex := parseImportIndexKeyExpr(ctx, e.Key, scope)
		diags = diags.Append(dIndex)
		traversal = append(traversal, tIndex)
	case *hclsyntax.RelativeTraversalExpr:
		t, d := traversalForImportExpr(ctx, e.Source, scope)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
		traversal = append(traversal, e.Traversal...)
	case *hclsyntax.ScopeTraversalExpr:
		traversal = append(traversal, e.Traversal...)
	default:
		// This should not happen, as it should have failed validation earlier, in config.AbsTraversalForImportToExpr
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import address expression",
			Detail:   "Import address must be a reference to a resource's address, and only allows for indexing with dynamic keys. For example: module.my_module[expression1].aws_s3_bucket.my_buckets[expression2] for resources inside of modules, or simply aws_s3_bucket.my_bucket for a resource in the root module",
			Subject:  expr.Range().Ptr(),
		})
	}

	return traversal, diags
}

// parseImportIndexKeyExpr parses an expression that is used as a key in an index, of an HCL expression representing an
// import target address, into a traversal of type hcl.TraverseIndex.
// After evaluation, the expression must be known, not null, not sensitive, and must be a string (for_each) or a number
// (count)
func parseImportIndexKeyExpr(ctx context.Context, expr hcl.Expression, scope exprs.Scope) (hcl.TraverseIndex, tfdiags.Diagnostics) {
	idx := hcl.TraverseIndex{
		SrcRange: expr.Range(),
	}

	// evaluate and take into consideration the for_each key (if exists)
	val, diags := exprs.NewClosure(exprs.EvalableHCLExpression(expr), scope).Value(ctx)
	if diags.HasErrors() {
		return idx, diags
	}

	if !val.IsKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index that will only be known after apply. Please ensure to use expressions that are known at plan time for the index of an import target address",
			Subject:  expr.Range().Ptr(),
			Extra:    evalchecks.DiagnosticCausedByUnknown(true),
		})
		return idx, diags
	}

	if val.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is null. Please ensure the expression for the index is not null",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	if val.Type() != cty.String && val.Type() != cty.Number {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is not valid for a resource instance (not a string or a number). Please ensure the expression for the index is correct, and returns either a string or a number",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	unmarkedVal, valMarks := val.Unmark()
	if _, sensitive := valMarks[marks.Sensitive]; sensitive {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is sensitive. Please ensure indexes used in the resource address of an import target are not sensitive",
			Subject:  expr.Range().Ptr(),
		})
	}
	if _, ephemeral := valMarks[marks.Ephemeral]; ephemeral {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is ephemeral. Please ensure indexes used in the resource address of an import target are not ephemeral",
			Subject:  expr.Range().Ptr(),
		})
	}

	idx.Key = unmarkedVal
	return idx, diags
}
