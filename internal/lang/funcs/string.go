// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// StartsWithFunc constructs a function that checks if a string starts with
// a specific prefix using strings.HasPrefix
var StartsWithFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name:         "str",
			Type:         cty.String,
			AllowUnknown: true,
		},
		{
			Name: "prefix",
			Type: cty.String,
		},
	},
	Type:         function.StaticReturnType(cty.Bool),
	RefineResult: refineNotNull,
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		prefix := args[1].AsString()

		if !args[0].IsKnown() {
			// If the unknown value has a known prefix then we might be
			// able to still produce a known result.
			if prefix == "" {
				// The empty string is a prefix of any string.
				return cty.True, nil
			}
			if knownPrefix := args[0].Range().StringPrefix(); knownPrefix != "" {
				if strings.HasPrefix(knownPrefix, prefix) {
					return cty.True, nil
				}
				if len(knownPrefix) >= len(prefix) {
					// If the prefix we're testing is no longer than the known
					// prefix and it didn't match then the full string with
					// that same prefix can't match either.
					return cty.False, nil
				}
			}
			return cty.UnknownVal(cty.Bool), nil
		}

		str := args[0].AsString()

		if strings.HasPrefix(str, prefix) {
			return cty.True, nil
		}

		return cty.False, nil
	},
})

// EndsWithFunc constructs a function that checks if a string ends with
// a specific suffix using strings.HasSuffix
var EndsWithFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "str",
			Type: cty.String,
		},
		{
			Name: "suffix",
			Type: cty.String,
		},
	},
	Type:         function.StaticReturnType(cty.Bool),
	RefineResult: refineNotNull,
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		str := args[0].AsString()
		suffix := args[1].AsString()

		if strings.HasSuffix(str, suffix) {
			return cty.True, nil
		}

		return cty.False, nil
	},
})

// ReplaceFunc constructs a function that searches a given string for another
// given substring, and replaces each occurence with a given replacement string.
var ReplaceFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "str",
			Type: cty.String,
		},
		{
			Name: "substr",
			Type: cty.String,
		},
		{
			Name: "replace",
			Type: cty.String,
		},
	},
	Type:         function.StaticReturnType(cty.String),
	RefineResult: refineNotNull,
	Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
		str := args[0].AsString()
		substr := args[1].AsString()
		replace := args[2].AsString()

		// We search/replace using a regexp if the string is surrounded
		// in forward slashes.
		if len(substr) > 1 && substr[0] == '/' && substr[len(substr)-1] == '/' {
			re, err := regexp.Compile(substr[1 : len(substr)-1])
			if err != nil {
				return cty.UnknownVal(cty.String), err
			}

			return cty.StringVal(re.ReplaceAllString(str, replace)), nil
		}

		return cty.StringVal(strings.Replace(str, substr, replace, -1)), nil
	},
})

// StrContainsFunc searches a given string for another given substring,
// if found the function returns true, otherwise returns false.
var StrContainsFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "str",
			Type: cty.String,
		},
		{
			Name: "substr",
			Type: cty.String,
		},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {
		str := args[0].AsString()
		substr := args[1].AsString()

		if strings.Contains(str, substr) {
			return cty.True, nil
		}

		return cty.False, nil
	},
})

// Replace searches a given string for another given substring,
// and replaces all occurences with a given replacement string.
func Replace(str, substr, replace cty.Value) (cty.Value, error) {
	return ReplaceFunc.Call([]cty.Value{str, substr, replace})
}

func StrContains(str, substr cty.Value) (cty.Value, error) {
	return StrContainsFunc.Call([]cty.Value{str, substr})
}

// MakeTemplateStringFunc constructs a function that takes a string and
// an arbitrary object of named values and attempts to render that string
// as a template using HCL template syntax.
func MakeTemplateStringFunc(content string, funcsCb func() map[string]function.Function) function.Function {

	params := []function.Parameter{
		{
			Name:        "data",
			Type:        cty.String,
			AllowMarked: true,
		},
		{
			Name:        "vars",
			Type:        cty.DynamicPseudoType,
			AllowMarked: true,
		},
	}
	loadTmpl := func(content string, marks cty.ValueMarks) (hcl.Expression, error) {

		// This condition checks if the provided string to be rendered as a template is marked as sensitive.
		// If the string is marked as sensitive, it returns an error indicating that sensitive strings cannot be used as template strings.
		if strings.Contains(marks.GoString(), "Sensitive") {
			return nil, function.NewArgErrorf(0, "Sensitive strings cannot be used as template strings. Please ensure that any sensitive information is removed from your template before using them")
		}

		expr, diags := hclsyntax.ParseTemplate([]byte(content), "NoFileNeeded", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, diags
		}

		return expr, nil
	}

	renderTmpl := func(expr hcl.Expression, varsVal cty.Value) (cty.Value, error) {
		if varsTy := varsVal.Type(); !(varsTy.IsMapType() || varsTy.IsObjectType()) {
			return cty.DynamicVal, function.NewArgErrorf(1, "invalid vars value: must be a map") // or an object, but we don't strongly distinguish these most of the time
		}

		// This loop iterates over each template variable's value and checks if any sensitive values are present.
		// If a sensitive value is found, it returns an error indicating that sensitive template variables cannot be used.
		for _, vars := range varsVal.AsValueMap() {
			_, varsMark := vars.Unmark()

			// Check if the variable is marked as sensitive.
			// If it is marked as sensitive, return an error.
			if strings.Contains(varsMark.GoString(), "Sensitive") {
				return cty.DynamicVal, function.NewArgErrorf(1, "Sensitive template variables cannot be used in the template. Please ensure that any sensitive information is removed from your template variables before using them")
			}
		}

		ctx := &hcl.EvalContext{
			Variables: varsVal.AsValueMap(),
		}

		// We require all of the variables to be valid HCL identifiers, because
		// otherwise there would be no way to refer to them in the template
		// anyway. Rejecting this here gives better feedback to the user
		// than a syntax error somewhere in the template itself.
		for n := range ctx.Variables {
			if !hclsyntax.ValidIdentifier(n) {
				// This error message intentionally doesn't describe _all_ of
				// the different permutations that are technically valid as an
				// HCL identifier, but rather focuses on what we might
				// consider to be an "idiomatic" variable name.
				return cty.DynamicVal, function.NewArgErrorf(1, "invalid template variable name %q: must start with a letter, followed by zero or more letters, digits, and underscores", n)
			}
		}

		// We'll pre-check references in the template here so we can give a
		// more specialized error message than HCL would by default, so it's
		// clearer that this problem is coming from a templatestring call.
		for _, traversal := range expr.Variables() {
			root := traversal.RootName()
			if _, ok := ctx.Variables[root]; !ok {
				return cty.DynamicVal, function.NewArgErrorf(1, "vars map does not contain key %q", root)
			}
		}

		givenFuncs := funcsCb() // this callback indirection is to avoid chicken/egg problems
		funcs := make(map[string]function.Function, len(givenFuncs))
		for name, fn := range givenFuncs {
			funcs[name] = fn
		}
		ctx.Functions = funcs

		val, diags := expr.Value(ctx)
		if diags.HasErrors() {
			return cty.DynamicVal, diags
		}
		return val, nil
	}

	return function.New(&function.Spec{
		Params: params,
		Type: func(args []cty.Value) (cty.Type, error) {
			if !(args[0].IsKnown() && args[1].IsKnown()) {
				return cty.DynamicPseudoType, nil
			}

			// We'll render our template now to see what result type it produces.
			// A template consisting only of a single interpolation an potentially
			// return any type.

			pathArg, pathMarks := args[0].Unmark()
			expr, err := loadTmpl(pathArg.AsString(), pathMarks)
			if err != nil {
				return cty.DynamicPseudoType, err
			}

			// This is safe even if args[1] contains unknowns because the HCL
			// template renderer itself knows how to short-circuit those.
			val, err := renderTmpl(expr, args[1])
			return val.Type(), err
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			pathArg, pathMarks := args[0].Unmark()
			expr, err := loadTmpl(pathArg.AsString(), pathMarks)
			if err != nil {
				return cty.DynamicVal, err
			}
			result, err := renderTmpl(expr, args[1])
			return result.WithMarks(pathMarks), err
		},
	})
}
