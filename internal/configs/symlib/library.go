package symlib

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

const TypeSymbols = "symbols"

type TypeRef struct {
	Namespace string
	Name      string
	Range     hcl.Range
}
type FunctionRef struct {
	Namespace string
	Name      string
	Range     hcl.Range
}
type ValueRef struct {
	Namespace string
	Range     hcl.Range
}

type typeWithDefault struct {
	ty  cty.Type
	def *typeexpr.Defaults
}
type Library struct {
	name      string
	types     map[string]typeWithDefault
	values    map[string]cty.Value
	functions map[string]function.Function
}

func (l Library) typeWithDefaults(name string, rng hcl.Range) (cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
	tyd, ok := l.types[name]
	if !ok {
		return cty.NilType, nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing type",
			Detail:   fmt.Sprintf("Type %s not defined in symbol library %s", name, l.name),
			Subject:  rng.Ptr(),
		}}
	}
	return tyd.ty, tyd.def, nil
}

func (l Library) value() cty.Value {
	return cty.ObjectVal(l.values)
}

func (l Library) function(name string, rng hcl.Range) (function.Function, hcl.Diagnostics) {
	fn, ok := l.functions[name]
	if !ok {
		return function.Function{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing function",
			Detail:   fmt.Sprintf("Function %s not defined in symbol library %s", name, l.name),
			Subject:  rng.Ptr(),
		}}
	}
	return fn, nil
}

type Table map[string]*Library

func (t Table) library(name string, rng hcl.Range) (*Library, hcl.Diagnostics) {
	lib, ok := t[name]
	if !ok {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing symbol library",
			Detail:   fmt.Sprintf("Symbol library %s not declared", name),
			Subject:  rng.Ptr(),
		}}
	}
	return lib, nil
}

func (t Table) Type(ref TypeRef) (cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
	lib, diags := t.library(ref.Namespace, ref.Range)
	if diags.HasErrors() {
		return cty.NilType, nil, diags
	}
	return lib.typeWithDefaults(ref.Name, ref.Range)
}

func (t Table) TypeContext() typeexpr.TypeContext {
	return typeexpr.TypeContext{TypeFunc: func(call *hcl.StaticCall) (*cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
		split := strings.Split(call.Name, "::")
		if split[0] != TypeSymbols {
			return nil, nil, nil
		}

		switch len(split) {
		case 3:
			ty, def, diags := t.Type(TypeRef{
				Namespace: split[1],
				Name:      split[2],
				Range:     call.NameRange,
			})
			return &ty, def, diags
		default:
			return nil, nil, nil
		}
	}}
}

func (t Table) Value(ref ValueRef) (cty.Value, hcl.Diagnostics) {
	lib, diags := t.library(ref.Namespace, ref.Range)
	if diags.HasErrors() {
		return cty.NilVal, diags
	}
	return lib.value(), diags
}

func (t Table) Values() map[string]cty.Value {
	values := map[string]cty.Value{}
	for name, lib := range t {
		values[name] = lib.value()
	}
	return values
}

func (t Table) Function(ref FunctionRef) (function.Function, hcl.Diagnostics) {
	lib, diags := t.library(ref.Namespace, ref.Range)
	if diags.HasErrors() {
		return function.Function{}, diags
	}
	return lib.function(ref.Name, ref.Range)
}

type Loader func(*SymbolCall) (*Library, hcl.Diagnostics)

func BuildTable(calls []*SymbolCall, loader Loader) (Table, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	table := Table{}
	for _, call := range calls {
		lib, lDiags := loader(call)
		diags = diags.Extend(lDiags)
		if lib != nil {
			lib.name = call.Name
			table[call.Name] = lib
		}
	}
	return table, diags
}

var EmptyTable = Table{}

// A consistent detail message for all "not a valid identifier" diagnostics.
// Duplicated from the configs package
const badIdentifierDetail = "A name must start with a letter or underscore and may contain only letters, digits, underscores, and dashes."
