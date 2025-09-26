// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// The main code in this package is intentionally completely unaware of
// any specific symbol table structures in the language, with that logic
// living in concrete implementations of [Scope], [SymbolTable] and [Valuer]
// in other language-specific packages, but for testing purposes here we have a
// contrived "mini-language" that is intentionally shaped like a subset of the
// OpenTofu module language to prove that this design is sufficient to handle
// that and to act as a relatively-concise overview of how a "real" use of this
// package might look.
//
// If a real implementation _were_ shaped like this then all of the types
// defined below would belong to some other package that implements the
// OpenTofu planning phase. Variations of this could also appear in a package
// that implements the validation phase, but in that case it would deal only
// in unexpanded modules and resources. In both cases the types implementing
// [Scope] and [Valuer] would ideally also implement all of the other business
// logic related to whatever they represent to keep e.g. all of the logic
// related to resource evaluation together in one place, but package exprs only
// cares about their implementations of its interfaces.
//
// This example implementation has the significant limitation that it doesn't
// have any way of detecting and reporting reference cycles. If any appear then
// it'll just attempt infinite recursion and smash the stack. A real
// implementation would need to somehow detect and report cyclic references,
// e.g. by internally doing something like what this package does:
//    https://pkg.go.dev/github.com/apparentlymart/go-workgraph/workgraph

func Example_simple() {
	varDefs := exampleMustParseTfvars(`
		name = "stephen"
	`)
	modInst := exampleMustParseModule(`
		variable "name" {}

		resource "example" "foo" {
			name = var.name
		}

		resource "example" "bar" {
			name = example.foo.name
		}
	`, varDefs)

	barR := modInst.Resource(addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "example",
		Name: "bar",
	})
	barV, diags := barR.Value(context.Background())
	if diags.HasErrors() {
		panic(spew.Sdump(diags.ForRPC()))
	}

	fmt.Println(ctydebug.ValueString(barV))

	// Output:
	// cty.ObjectVal(map[string]cty.Value{
	//     "name": cty.StringVal("stephen"),
	// })
}

// testResource represents a module instance, implementing [Scope].
type testModuleInstance struct {
	variables map[addrs.InputVariable]*testInputVariable
	resources map[addrs.Resource]*testResource
}

var _ exprs.Scope = (*testModuleInstance)(nil)

// Resource returns the resource with the given address, or nil if there is
// no such resource declared in the module.
func (t *testModuleInstance) Resource(addr addrs.Resource) *testResource {
	return t.resources[addr]
}

// HandleInvalidStep implements Scope.
func (t *testModuleInstance) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	// NOTE: It isn't possible to get here in practice because we only use
	// this as a top-level scope and HCL's parser only allows TraverseRoot
	// at the start of a reference anyway, so we could only get in here
	// if an [Evalable.References] implementation returns something odd.
	var diags tfdiags.Diagnostics
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference",
		Detail:   "Expected the name of a top-level symbol.",
		Subject:  rng.ToHCL().Ptr(),
	})
	return diags
}

// ResolveAttr implements Scope.
func (t *testModuleInstance) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	// Note that handling this as part of the implementation a module, rather
	// than separately in package addrs, makes it easier for the resolution
	// rules to vary depending on which language edition and language
	// experiments the module is using, because in a real implementation this
	// object would have access to the module configuration.
	//
	// The extra symbols supported in .tftest.hcl files can also be handled
	// by having the test scenario type also implement Scope, handle the
	// test-language-specific symbols first, and then delegate to a wrapped
	// module object for everything else.

	switch ref.Name {
	case "var":
		return exprs.NestedSymbolTable(testInputVariables(t.variables)), nil
	case "resource":
		return exprs.NestedSymbolTable(&testResourcesOfMode{
			mode:         addrs.ManagedResourceMode,
			allResources: t.resources,
		}), nil
	case "data":
		return exprs.NestedSymbolTable(&testResourcesOfMode{
			mode:         addrs.DataResourceMode,
			allResources: t.resources,
		}), nil
	case "ephemeral":
		return exprs.NestedSymbolTable(&testResourcesOfMode{
			mode:         addrs.EphemeralResourceMode,
			allResources: t.resources,
		}), nil
	default:
		return exprs.NestedSymbolTable(&testResourcesOfType{
			mode:         addrs.ManagedResourceMode,
			typeName:     ref.Name,
			allResources: t.resources,
		}), nil
	}
}

// ResolveFunc implements Scope.
func (t *testModuleInstance) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	// A real implementation of this would probably look the function name up
	// in a map built elsewhere, rather than like this.
	switch call.Name {
	case "upper":
		return function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "str",
					Type: cty.String,
				},
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				// NOTE: This is not a robust implementation of "upper", just
				// a placeholder for the sake of this example.
				return cty.StringVal(strings.ToUpper(args[0].AsString())), nil
			},
		}), nil
	default:
		var diags tfdiags.Diagnostics
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Call to unknown function",
			Detail:   fmt.Sprintf("There is no function named %q.", call.Name),
			Subject:  &call.NameRange,
		})
		return function.Function{}, diags
	}
}

// testInputVariables is an intermediate [SymbolTable] dealing with the
// symbols under "var.".
type testInputVariables map[addrs.InputVariable]*testInputVariable

var _ exprs.SymbolTable = testInputVariables(nil)

// HandleInvalidStep implements exprs.SymbolTable.
func (t testInputVariables) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference to input variable",
		Detail:   "Expected an attribute name matching an input variable declared in this module.",
		Subject:  rng.ToHCL().Ptr(),
	})
	return diags
}

// ResolveAttr implements exprs.SymbolTable.
func (t testInputVariables) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	iv, ok := t[addrs.InputVariable{Name: ref.Name}]
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared input variable",
			Detail:   fmt.Sprintf("There is no input variable named %q declared in this module.", ref.Name),
			Subject:  &ref.SrcRange,
		})
		return nil, diags
	}
	return exprs.ValueOf(iv), diags
}

type testInputVariable struct {
	addr       addrs.InputVariable
	targetType cty.Type
	rawVal     cty.Value
	valRange   tfdiags.SourceRange
}

var _ exprs.Valuer = (*testInputVariable)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (t *testInputVariable) TypeConstraint() cty.Type {
	// An input variable's "type" is a target type for conversion rather than
	// just a type constraint, so we need to discard any optional attribute
	// information to get a plain type constraint.
	return t.targetType.WithoutOptionalAttributesDeep()
}

// StaticCheckTraversal implements exprs.Valuer.
func (t *testInputVariable) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return exprs.StaticCheckTraversalThroughType(traversal, t.TypeConstraint())
}

// Value implements exprs.Valuer.
func (t *testInputVariable) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// In a real implementation this type would probably not have the value
	// directly and would instead have an expression from an argument in
	// the calling "module" block, but we'll keep this relatively simple
	// for the sake of example.
	v, err := convert.Convert(t.rawVal, t.targetType)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value for input variable",
			Detail:   fmt.Sprintf("Unsuitable value for input variable %q: %s.", t.addr.Name, err),
			Subject:  t.valRange.ToHCL().Ptr(),
		})
		v = cty.UnknownVal(t.TypeConstraint())
	}
	return v, diags
}

// ValueSourceRange implements exprs.Valuer.
func (t *testInputVariable) ValueSourceRange() *tfdiags.SourceRange {
	return &t.valRange
}

// testInputVariables is an intermediate [SymbolTable] implementation dealing
// with symbols under "resource.", "data.", and "ephemeral.".
type testResourcesOfMode struct {
	mode         addrs.ResourceMode
	allResources map[addrs.Resource]*testResource
}

var _ exprs.SymbolTable = (*testResourcesOfMode)(nil)

// HandleInvalidStep implements exprs.SymbolTable.
func (t *testResourcesOfMode) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference to resource",
		Detail:   "Expected an attribute name matching the type of the resource to refer to.",
		Subject:  rng.ToHCL().Ptr(),
	})
	return diags
}

// ResolveAttr implements exprs.SymbolTable.
func (t *testResourcesOfMode) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	// For now we'll just accept anything here and wait until we've collected
	// enough steps to form a complete addrs.Resource value.
	return exprs.NestedSymbolTable(&testResourcesOfType{
		mode:         t.mode,
		typeName:     ref.Name,
		allResources: t.allResources,
	}), nil
}

// testInputVariables is an intermediate [SymbolTable] implementation dealing
// with symbols under "resource.ANYTHING.", "data.ANYTHING.",
// "ephemeral.ANYTHING.", and "ANYTHING.".
type testResourcesOfType struct {
	mode         addrs.ResourceMode
	typeName     string
	allResources map[addrs.Resource]*testResource
}

var _ exprs.SymbolTable = (*testResourcesOfType)(nil)

// HandleInvalidStep implements exprs.SymbolTable.
func (t *testResourcesOfType) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference to resource",
		Detail:   "Expected an attribute name matching the name of the resource to refer to.",
		Subject:  rng.ToHCL().Ptr(),
	})
	return diags
}

// ResolveAttr implements exprs.SymbolTable.
func (t *testResourcesOfType) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Once we reach this step we've collected enough information to
	// form a resource address.
	addr := addrs.Resource{
		Mode: t.mode,
		Type: t.typeName,
		Name: ref.Name,
	}
	rsrc, ok := t.allResources[addr]
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared resource",
			Detail:   fmt.Sprintf("There is no resource %s declared in this module.", addr),
			Subject:  &ref.SrcRange,
		})
		return nil, diags
	}
	return exprs.ValueOf(rsrc), diags
}

// testResource represents a resource, implementing [Valuer].
//
// A real implementation of this would need to deal with multi-instance resources
// using count/for_each too, probably delegating to another type representing
// each individual resource instance, but we ignore that here because that
// complexity is an implementation detail irrelevant to package exprs.
type testResource struct {
	// config is the [Valuer] for the resource's configuration body.
	//
	// In practice this is an [*exprs.Closure] associating the actual HCL body
	// with the module instance where it was declared, but that is a concern
	// only for the code that constructs this object; the testResource
	// implementation only knows that it can obtain a value from here when
	// needed, without worrying about how that is achieved.
	//
	// (in a real implementation that supported multiple resource instances
	// we'd need to delay constructing the exprs.Valuer until constructing
	// individual resource instances, because in that case the resource instance
	// configs must close over a child scope that also has instance-specific
	// each.key/each.value/count.index in it, but this example is already
	// complicated enough so we'll skip that here.)
	config exprs.Valuer
}

var _ exprs.Valuer = (*testResource)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (t *testResource) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return t.config.StaticCheckTraversal(traversal)
}

// Value implements exprs.Valuer.
func (t *testResource) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return t.config.Value(ctx)
}

// ValueSourceRange implements exprs.Valuer.
func (t *testResource) ValueSourceRange() *tfdiags.SourceRange {
	return t.config.ValueSourceRange()
}

// exampleMustParseTfvars is a helper function just to make these contrived
// examples a little more concise, which tries to interpret the given string
// in a similar way to how OpenTofu would normally deal with a ".tfvars" file.
func exampleMustParseTfvars(src string) map[string]variableDef {
	f, hclDiags := hclsyntax.ParseConfig([]byte(src), "example.tfvars", hcl.InitialPos)
	if hclDiags.HasErrors() {
		panic(fmt.Sprintf("invalid tfvars: %s", hclDiags.Error()))
	}
	attrs, hclDiags := f.Body.JustAttributes()
	if hclDiags.HasErrors() {
		panic(fmt.Sprintf("invalid tfvars: %s", hclDiags.Error()))
	}
	ret := make(map[string]variableDef, len(attrs))
	for name, attr := range attrs {
		v, hclDiags := attr.Expr.Value(nil)
		if hclDiags.HasErrors() {
			panic(fmt.Sprintf("invalid tfvars: %s", hclDiags.Error()))
		}
		ret[name] = variableDef{
			val: v,
			rng: tfdiags.SourceRangeFromHCL(attr.Expr.Range()),
		}
	}
	return ret
}

// exampleMustParseTfvars is a helper function just to make these contrived
// examples a little more concise, which tries to interpret the given string
// as the mini-language implemented in this example.
func exampleMustParseModule(src string, inputVals map[string]variableDef) *testModuleInstance {
	f, hclDiags := hclsyntax.ParseConfig([]byte(src), "config.minitofu", hcl.InitialPos)
	if hclDiags.HasErrors() {
		panic(fmt.Sprintf("invalid module: %s", hclDiags.Error()))
	}

	rootSchema := hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
			{Type: "resource", LabelNames: []string{"type", "name"}},
			{Type: "data", LabelNames: []string{"type", "name"}},
			{Type: "ephemeral", LabelNames: []string{"type", "name"}},
		},
	}
	content, hclDiags := f.Body.Content(&rootSchema)
	if hclDiags.HasErrors() {
		panic(fmt.Sprintf("invalid module: %s", hclDiags.Error()))
	}

	modInst := &testModuleInstance{
		variables: make(map[addrs.InputVariable]*testInputVariable),
		resources: make(map[addrs.Resource]*testResource),
	}
	for _, block := range content.Blocks {
		switch block.Type {
		case "variable":
			addr := addrs.InputVariable{Name: block.Labels[0]}
			def, ok := inputVals[addr.Name]
			if !ok {
				panic(fmt.Sprintf("no value for input variable %q", addr.Name))
			}
			modInst.variables[addr] = &testInputVariable{
				addr:       addr,
				targetType: cty.String, // only strings to keep this example simpler
				rawVal:     def.val,
				valRange:   def.rng,
			}
		case "resource", "data", "ephemeral":
			addr := addrs.Resource{
				Mode: map[string]addrs.ResourceMode{
					"resource":  addrs.ManagedResourceMode,
					"data":      addrs.DataResourceMode,
					"ephemeral": addrs.EphemeralResourceMode,
				}[block.Type],
				Type: block.Labels[0],
				Name: block.Labels[1],
			}
			typeAddr := resourceType{
				Mode: addr.Mode,
				Name: addr.Type,
			}
			schema, ok := resourceTypes[typeAddr]
			if !ok {
				panic(fmt.Sprintf("unsupported resource type %#v", typeAddr))
			}
			modInst.resources[addr] = &testResource{
				config: exprs.NewClosure(
					exprs.EvalableHCLBody(block.Body, schema.DecoderSpec()),
					modInst,
				),
			}
		}
	}
	return modInst
}

type variableDef struct {
	val cty.Value
	rng tfdiags.SourceRange
}

type resourceType struct {
	Mode addrs.ResourceMode
	Name string
}

var resourceTypes = map[resourceType]*configschema.Block{
	resourceType{addrs.ManagedResourceMode, "example"}: &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": &configschema.Attribute{
				Type:     cty.String,
				Required: true,
			},
		},
	},
}
