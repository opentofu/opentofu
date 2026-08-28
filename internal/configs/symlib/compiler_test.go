// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

func assertNoDiags(t *testing.T, diags hcl.Diagnostics) {
	t.Helper()
	if len(diags) > 0 {
		if diags.HasErrors() {
			t.Fatalf("%#v", diags.Error())
		}
		t.Fatalf("%#v", diags)
	}
}

// All non-tofu specific functions
var testCtyFuncs = map[string]function.Function{
	"abs":             stdlib.AbsoluteFunc,
	"can":             tryfunc.CanFunc,
	"ceil":            stdlib.CeilFunc,
	"chomp":           stdlib.ChompFunc,
	"coalescelist":    stdlib.CoalesceListFunc,
	"compact":         stdlib.CompactFunc,
	"concat":          stdlib.ConcatFunc,
	"contains":        stdlib.ContainsFunc,
	"csvdecode":       stdlib.CSVDecodeFunc,
	"distinct":        stdlib.DistinctFunc,
	"element":         stdlib.ElementFunc,
	"chunklist":       stdlib.ChunklistFunc,
	"flatten":         stdlib.FlattenFunc,
	"floor":           stdlib.FloorFunc,
	"format":          stdlib.FormatFunc,
	"formatdate":      stdlib.FormatDateFunc,
	"formatlist":      stdlib.FormatListFunc,
	"indent":          stdlib.IndentFunc,
	"join":            stdlib.JoinFunc,
	"jsondecode":      stdlib.JSONDecodeFunc,
	"jsonencode":      stdlib.JSONEncodeFunc,
	"keys":            stdlib.KeysFunc,
	"log":             stdlib.LogFunc,
	"lower":           stdlib.LowerFunc,
	"max":             stdlib.MaxFunc,
	"merge":           stdlib.MergeFunc,
	"min":             stdlib.MinFunc,
	"parseint":        stdlib.ParseIntFunc,
	"pow":             stdlib.PowFunc,
	"range":           stdlib.RangeFunc,
	"regex":           stdlib.RegexFunc,
	"regexall":        stdlib.RegexAllFunc,
	"reverse":         stdlib.ReverseListFunc,
	"setintersection": stdlib.SetIntersectionFunc,
	"setproduct":      stdlib.SetProductFunc,
	"setsubtract":     stdlib.SetSubtractFunc,
	"setunion":        stdlib.SetUnionFunc,
	"signum":          stdlib.SignumFunc,
	"slice":           stdlib.SliceFunc,
	"sort":            stdlib.SortFunc,
	"split":           stdlib.SplitFunc,
	"strrev":          stdlib.ReverseFunc,
	"substr":          stdlib.SubstrFunc,
	"timeadd":         stdlib.TimeAddFunc,
	"title":           stdlib.TitleFunc,
	"trim":            stdlib.TrimFunc,
	"trimprefix":      stdlib.TrimPrefixFunc,
	"trimspace":       stdlib.TrimSpaceFunc,
	"trimsuffix":      stdlib.TrimSuffixFunc,
	"try":             tryfunc.TryFunc,
	"upper":           stdlib.UpperFunc,
	"values":          stdlib.ValuesFunc,
	"yamldecode":      ctyyaml.YAMLDecodeFunc,
	"yamlencode":      ctyyaml.YAMLEncodeFunc,
	"zipmap":          stdlib.ZipmapFunc,
}

var ctyCmpOpts = cmp.Options{
	cmp.Comparer(cty.Type.Equals),
	cmp.Comparer(func(a cty.Value, b cty.Value) bool { return a.Equals(b).True() }),
}

func testCompile(t *testing.T, tcfiles map[string]string) (*Library, hcl.Diagnostics) {
	files := map[string][]*SymbolFile{}
	for path, contents := range tcfiles {
		parts := strings.Split(path, "/")
		fileName := parts[len(parts)-1]
		dir := strings.Join(parts[:len(parts)-1], "/") + "/"

		parsed, diags := hclsyntax.ParseConfig([]byte(contents), fileName, hcl.InitialPos)
		assertNoDiags(t, diags)

		sFile, diags := LoadSymbolFile(parsed.Body)
		assertNoDiags(t, diags)

		files[dir] = append(files[dir], sFile)
	}

	var loader Loader
	loader = func(call *SymbolCall) (*Library, hcl.Diagnostics) {
		var source string
		diags := gohcl.DecodeExpression(call.Source, nil, &source)
		assertNoDiags(t, diags)
		return CompileLibrary(files[source], loader, testCtyFuncs)
	}
	return CompileLibrary(files["./"], loader, testCtyFuncs)
}

// TestFunctionResultTypeConversion is a regression test to ensure that we always convert the type
// of the function's return value to the declared function return type. This solves
// issues where we need to ensure that we're converting the types to pass through chains correctly.
func TestFunctionResultTypeConversion(t *testing.T) {
	files := map[string]string{
		"./functions.sym.hcl": `
function "get_list" {
	description = "returns a list"
	type = list(string)
	parameter "values" {
		description = "input"
		type = string
		variadic = true
	}

		return = [for x in param.values: x]
}`,
	}

	lib, diags := testCompile(t, files)
	assertNoDiags(t, diags)
	f, ok := lib.functions["get_list"]
	if !ok {
		t.Fatal("Failed to find function get_list")
	}

	got, err := f.Call([]cty.Value{cty.StringVal("hi"), cty.StringVal("mom")})
	if err != nil {
		t.Fatalf("Error during function call: %v", err)
	}

	if !got.Type().Equals(cty.List(cty.String)) {
		t.Fatalf("Unexpected return type, expected list(string), got %s", got.Type().FriendlyName())
	}

	wanted := cty.ListVal([]cty.Value{cty.StringVal("hi"), cty.StringVal("mom")})

	if diff := cmp.Diff(wanted, got, ctyCmpOpts); diff != "" {
		t.Fatal(diff)
	}
}

// TestFunctionParallelExternalCalls ensures that functions
// called from outside the current symbol library have their
// own dedicated workgraph worker.
func TestFunctionParallelExternalCalls(t *testing.T) {
	files := map[string]string{
		"./functions.sym.hcl": `
function "outer" {
	parameter "value" {
		type = string
	}
	return = symbols::inner(param.value)
}
function "inner" {
	parameter "value" {
		type = string
	}
	return = "Value: ${param.value}"
}
`,
	}

	lib, diags := testCompile(t, files)
	assertNoDiags(t, diags)
	f, ok := lib.functions["outer"]
	if !ok {
		t.Fatal("Failed to find function outer")
	}

	var w sync.WaitGroup
	for i := range 2048 {
		w.Go(func() {
			str := fmt.Sprintf("hi %v", i)
			got, err := f.Call([]cty.Value{cty.StringVal(str)})
			if err != nil {
				t.Fatalf("Error during function call: %v", err)
			}

			if !got.Type().Equals(cty.String) {
				t.Fatalf("Unexpected return type, expected string, got %s", got.Type().FriendlyName())
			}

			wanted := cty.StringVal(fmt.Sprintf("Value: %s", str))
			if diff := cmp.Diff(wanted, got, ctyCmpOpts); diff != "" {
				t.Fatal(diff)
			}
		})
	}

	w.Wait()
}

// This initial set of tests mirrors what is in the RFC (rfc/20260424-symbol-libraries.md)
func TestRFCExamples(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		assert func(*testing.T, *Library)
	}{{
		name: "types",
		files: map[string]string{
			"./types.sym.hcl": `
typedef "simple_type" {
  # https://opentofu.org/docs/language/expressions/types/
  # Any builtin type may be used here
  type = number
}

typedef "complex_type" {
  type = object({
    ncpus = number
    # Types may reference other types within the same symbol library (or other imported libraries)
    memory_size = symbols::simple_type()
  })
}

typedef "defaults_type" {
  type = object({
    # Defaults (via optional()) will be stored alongside the type
    complex = optional(symbols::complex_type(), { ncpus = 1, memory_size = 1024 })
  })
}`,
		},
		assert: func(t *testing.T, lib *Library) {
			complexType := cty.Object(map[string]cty.Type{"memory_size": cty.Number, "ncpus": cty.Number})
			defaultsType := cty.ObjectWithOptionalAttrs(map[string]cty.Type{"complex": cty.Object(map[string]cty.Type{"memory_size": cty.Number, "ncpus": cty.Number})}, []string{"complex"})
			types := []struct {
				name string
				def  *typeexpr.Defaults
				ty   cty.Type
			}{
				{"simple_type", nil, cty.Number},
				{"complex_type", nil, complexType},
				{"defaults_type", &typeexpr.Defaults{Type: defaultsType, DefaultValues: map[string]cty.Value{"complex": cty.ObjectVal(map[string]cty.Value{"memory_size": cty.NumberIntVal(1024), "ncpus": cty.NumberIntVal(1)})}}, defaultsType},
			}
			for _, entry := range types {
				t.Run(entry.name, func(t *testing.T) {
					ty, def, diags := lib.typeWithDefaults(entry.name, hcl.Range{})
					assertNoDiags(t, diags)
					if diff := cmp.Diff(entry.def, def, ctyCmpOpts); diff != "" {
						t.Error(diff)
					}
					if diff := cmp.Diff(entry.ty, ty, ctyCmpOpts); diff != "" {
						t.Error(diff)
					}
				})
			}
		},
	}, {
		name: "locals",
		files: map[string]string{
			"./locals.sym.hcl": `
values {
  simple = 10
  custom_regex = "<some complex regex>"
  # Most builtin functions are allowed, TBD if we allow "file", "plantimestamp" and other non-constant functions
  upper_regex = upper(value.custom_regex)
}`,
		},
		assert: func(t *testing.T, lib *Library) {
			expected := cty.ObjectVal(map[string]cty.Value{
				"simple":       cty.NumberIntVal(10),
				"custom_regex": cty.StringVal("<some complex regex>"),
				"upper_regex":  cty.StringVal("<SOME COMPLEX REGEX>"),
			})
			if diff := cmp.Diff(expected, lib.value(), ctyCmpOpts); diff != "" {
				t.Error(diff)
			}
		},
	}, {
		name: "functions",
		files: map[string]string{
			"./functions.sym.hcl": `
# Simple function definition
function "add" {
  description = "add two numbers together" # Optional
  type        = number                     # Optional
  parameter "a" {
    description = "first number"
    type = number
  }
  parameter "b" {
    description = "second number"
    type = number
  }
  return = param.a + param.b 
}

# Function with parameter validation
# https://opentofu.org/docs/language/expressions/custom-conditions/#input-variable-validation
function "divide" {
  parameter "a" {
    type = number
  }
  parameter "b" {
    type = number
    validation {
      condition     = param.b != 0
      error_message = "Divide by zero"
    }
  }
  return = param.a / param.b 
}

# Function using variadic parameters and multiple expressions
function "greeting" {
  parameter "prefix" {
    type    = string
  }
  parameter "name" {
    type     = string
    variadic = true
  }
  locals {
    messages = [for x in param.name: "${param.prefix} ${x}!"]
  }
  return = local.messages
}

# Function that uses custom types and calls other functions, both builtin and user defined
typedef "vec3" {
  type = object({ x = number, y = number, z = number})
}
function "vec3_lengthsq" {
  parameter "vec" {
    type = symbols::vec3()
  }
  locals {
    xx = param.vec.x * param.vec.x
    yy = param.vec.y * param.vec.y
    zz = param.vec.z * param.vec.z
    squared_sum = symbols::add(symbols::add(local.xx, local.yy), local.zz)
  }
  // We don't have a sqrt function...
  return = local.squared_sum
}`,
		},
		assert: func(t *testing.T, lib *Library) {
			calls := []struct {
				name     string
				expr     string
				expected cty.Value
			}{
				{"add", "symbols::add(2, 4)", cty.NumberIntVal(6)},
				{"divide", "symbols::divide(4, 2)", cty.NumberIntVal(2)},
				{"greeting", `symbols::greeting("hello", "world")`, cty.TupleVal([]cty.Value{cty.StringVal("hello world!")})},
				{"vec3_lengthsq", `symbols::vec3_lengthsq({x: 2, y: 3, z: 4})`, cty.NumberIntVal(29)},
			}

			for _, call := range calls {
				expr, diags := hclsyntax.ParseExpression([]byte(call.expr), "<test>", hcl.InitialPos)
				assertNoDiags(t, diags)

				impl, diags := lib.function(call.name, hcl.Range{})
				assertNoDiags(t, diags)

				got, diags := expr.Value(&hcl.EvalContext{Functions: map[string]function.Function{"symbols::" + call.name: *impl}})
				assertNoDiags(t, diags)

				if diff := cmp.Diff(call.expected, got, ctyCmpOpts); diff != "" {
					t.Error(diff)
				}
			}
		},
	}, {
		name: "nested",
		files: map[string]string{
			"./internal/types.sym.hcl": `
typedef "custom" {
  type = object({id = string, size = number})
}

typedef "other" {
  type = list(symbols::custom())
}
`,
			"./contents.sym.hcl": `
# Import the internal library
symbols "internal" {
  source = "./internal/"
}


# Re-export the custom type defined in the internal library
typedef "customexp" {
  type = symbols::internal::custom()
}
`,
		},
		assert: func(t *testing.T, lib *Library) {
			ty, _, diags := lib.typeWithDefaults("customexp", hcl.Range{})
			assertNoDiags(t, diags)

			expected := cty.Object(map[string]cty.Type{"id": cty.String, "size": cty.Number})
			if diff := cmp.Diff(expected, ty, ctyCmpOpts); diff != "" {
				t.Error(diff)
			}
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse files into "directory" structure
			files := map[string][]*SymbolFile{}
			for path, contents := range tc.files {
				parts := strings.Split(path, "/")
				fileName := parts[len(parts)-1]
				dir := strings.Join(parts[:len(parts)-1], "/") + "/"

				parsed, diags := hclsyntax.ParseConfig([]byte(contents), fileName, hcl.InitialPos)
				assertNoDiags(t, diags)

				sFile, diags := LoadSymbolFile(parsed.Body)
				assertNoDiags(t, diags)

				files[dir] = append(files[dir], sFile)
			}

			var loader Loader
			loader = func(call *SymbolCall) (*Library, hcl.Diagnostics) {
				var source string
				diags := gohcl.DecodeExpression(call.Source, nil, &source)
				assertNoDiags(t, diags)
				return CompileLibrary(files[source], loader, testCtyFuncs)
			}
			lib, diags := testCompile(t, tc.files)
			assertNoDiags(t, diags)
			tc.assert(t, lib)
		})
	}
}

// These additional tests cover common error conditions
func TestRFCErrors(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		assert func(*testing.T, hcl.Diagnostics)
	}{{
		name: "missing library",
		files: map[string]string{
			"./contents.sym.hcl": `
values {
  x = symbols.foo.magic
  y = symbols::bar::func()
}
`,
		},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			if len(diags) != 2 {
				t.Error("Expected 2 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Missing symbol library" {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}, {
		name:  "missing function",
		files: map[string]string{"./contents.sym.hcl": `values {  y = symbols::func() }`},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			// TODO ideally this would be 1 diag, but two is fine for now
			if len(diags) != 2 {
				t.Error("Expected 2 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Missing function" && diag.Summary != "Call to unknown function" {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}, {
		name:  "missing local",
		files: map[string]string{"./contents.sym.hcl": `values {  x = value.magic } `},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			if len(diags) != 1 {
				t.Error("Expected 1 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Missing value" {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}, {
		name:  "missing type",
		files: map[string]string{"./contents.sym.hcl": `typedef "foo" { type = symbols::bar() } `},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			// TODO this should be a better error message
			if len(diags) != 1 {
				t.Error("Expected 1 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Invalid type specification" {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}, {
		name: "circular function",
		files: map[string]string{"./contents.sym.hcl": `
function "foo" { return = symbols::foo() }
values { foo = symbols::foo() }`},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			if len(diags) != 1 {
				t.Error("Expected 1 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Error in function call" && !strings.Contains(diag.Detail, "Recursive call") {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}, {
		name:  "circular local",
		files: map[string]string{"./contents.sym.hcl": `values {  x = value.x } `},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			if len(diags) != 1 {
				t.Error("Expected 1 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Self-referential expressions" {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}, {
		name:  "circular type",
		files: map[string]string{"./contents.sym.hcl": `typedef "foo" { type = symbols::foo() } `},
		assert: func(t *testing.T, diags hcl.Diagnostics) {
			// TODO this should be a better error message
			if len(diags) != 1 {
				t.Error("Expected 1 diags")
			}
			for _, diag := range diags {
				if diag.Summary != "Self-referential expressions" {
					t.Errorf("Unexpected diag %#v", diag)
				}
			}
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse files into "directory" structure
			files := map[string][]*SymbolFile{}
			for path, contents := range tc.files {
				parts := strings.Split(path, "/")
				fileName := parts[len(parts)-1]
				dir := strings.Join(parts[:len(parts)-1], "/") + "/"

				parsed, diags := hclsyntax.ParseConfig([]byte(contents), fileName, hcl.InitialPos)
				assertNoDiags(t, diags)

				sFile, diags := LoadSymbolFile(parsed.Body)
				assertNoDiags(t, diags)

				files[dir] = append(files[dir], sFile)
			}

			var loader Loader
			loader = func(call *SymbolCall) (*Library, hcl.Diagnostics) {
				var source string
				diags := gohcl.DecodeExpression(call.Source, nil, &source)
				assertNoDiags(t, diags)
				return CompileLibrary(files[source], loader, testCtyFuncs)
			}
			_, diags := testCompile(t, tc.files)
			tc.assert(t, diags)
		})
	}
}
