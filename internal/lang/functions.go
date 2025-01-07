// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/experiments"
	"github.com/opentofu/opentofu/internal/lang/funcs"
)

var impureFunctions = []string{
	"bcrypt",
	"timestamp",
	"uuid",
}

// This should probably be replaced with addrs.Function everywhere
const CoreNamespace = addrs.FunctionNamespaceCore + "::"

// Functions returns the set of functions that should be used to when evaluating
// expressions in the receiving scope.
func (s *Scope) Functions() map[string]function.Function {
	s.funcsLock.Lock()
	if s.funcs == nil {
		s.funcs = makeBaseFunctionTable(s.BaseDir)
		if s.ConsoleMode {
			// The type function is only available in OpenTofu console.
			s.funcs["type"] = funcs.TypeFunc
		} else {
			// The plantimestamp function doesn't make sense in the OpenTofu
			// console.
			s.funcs["plantimestamp"] = funcs.MakeStaticTimestampFunc(s.PlanTimestamp)
		}

		if s.PureOnly {
			// Force our few impure functions to return unknown so that we
			// can defer evaluating them until a later pass.
			for _, name := range impureFunctions {
				s.funcs[name] = function.Unpredictable(s.funcs[name])
			}
		}

		coreNames := make([]string, 0)
		// Add a description to each function and parameter based on the
		// contents of descriptionList.
		// One must create a matching description entry whenever a new
		// function is introduced.
		for name, f := range s.funcs {
			s.funcs[name] = funcs.WithDescription(name, f)
			coreNames = append(coreNames, name)
		}
		// Copy all stdlib funcs into core:: namespace
		for _, name := range coreNames {
			s.funcs[CoreNamespace+name] = s.funcs[name]
		}
	}
	s.funcsLock.Unlock()

	return s.funcs
}

// experimentalFunction checks whether the given experiment is enabled for
// the receiving scope. If so, it will return the given function verbatim.
// If not, it will return a placeholder function that just returns an
// error explaining that the function requires the experiment to be enabled.
//
//lint:ignore U1000 Ignore unused function error for now
func (s *Scope) experimentalFunction(experiment experiments.Experiment, fn function.Function) function.Function {
	if s.activeExperiments.Has(experiment) {
		return fn
	}

	err := fmt.Errorf(
		"this function is experimental and available only when the experiment keyword %s is enabled for the current module",
		experiment.Keyword(),
	)

	return function.New(&function.Spec{
		Params:   fn.Params(),
		VarParam: fn.VarParam(),
		Type: func(args []cty.Value) (cty.Type, error) {
			return cty.DynamicPseudoType, err
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			// It would be weird to get here because the Type function always
			// fails, but we'll return an error here too anyway just to be
			// robust.
			return cty.DynamicVal, err
		},
	})
}

// makeBaseFunctionTable constructs the initial table of functions that we uses as
// the basis for the function table in each scope.
//
// This function intentionally always returns a fresh map on each call because the
// caller is expected to modify it further before storing it as part of a
// particular [Scope], based on the unique settings of that scope.
//
//nolint:funlen // The length of this function naturally scales with the number of functions in the OpenTofu language.
func makeBaseFunctionTable(baseDir string) map[string]function.Function {
	// Some of our functions are just directly the cty stdlib functions.
	// Others are implemented in the subdirectory "funcs" here in this
	// repository. New functions should generally start out their lives
	// in the "funcs" directory and potentially graduate to cty stdlib
	// later if the functionality seems to be something domain-agnostic
	// that would be useful to all applications using cty functions.

	ret := map[string]function.Function{
		"abs":              stdlib.AbsoluteFunc,
		"abspath":          funcs.AbsPathFunc,
		"alltrue":          funcs.AllTrueFunc,
		"anytrue":          funcs.AnyTrueFunc,
		"basename":         funcs.BasenameFunc,
		"base64decode":     funcs.Base64DecodeFunc,
		"base64encode":     funcs.Base64EncodeFunc,
		"base64gzip":       funcs.Base64GzipFunc,
		"base64gunzip":     funcs.Base64GunzipFunc,
		"base64sha256":     funcs.Base64Sha256Func,
		"base64sha512":     funcs.Base64Sha512Func,
		"bcrypt":           funcs.BcryptFunc,
		"can":              tryfunc.CanFunc,
		"ceil":             stdlib.CeilFunc,
		"chomp":            stdlib.ChompFunc,
		"cidrcontains":     funcs.CidrContainsFunc,
		"cidrhost":         funcs.CidrHostFunc,
		"cidrnetmask":      funcs.CidrNetmaskFunc,
		"cidrsubnet":       funcs.CidrSubnetFunc,
		"cidrsubnets":      funcs.CidrSubnetsFunc,
		"coalesce":         funcs.CoalesceFunc,
		"coalescelist":     stdlib.CoalesceListFunc,
		"compact":          stdlib.CompactFunc,
		"concat":           stdlib.ConcatFunc,
		"contains":         stdlib.ContainsFunc,
		"csvdecode":        stdlib.CSVDecodeFunc,
		"dirname":          funcs.DirnameFunc,
		"distinct":         stdlib.DistinctFunc,
		"element":          stdlib.ElementFunc,
		"endswith":         funcs.EndsWithFunc,
		"chunklist":        stdlib.ChunklistFunc,
		"file":             funcs.MakeFileFunc(baseDir, false),
		"fileexists":       funcs.MakeFileExistsFunc(baseDir),
		"fileset":          funcs.MakeFileSetFunc(baseDir),
		"filebase64":       funcs.MakeFileFunc(baseDir, true),
		"filebase64sha256": funcs.MakeFileBase64Sha256Func(baseDir),
		"filebase64sha512": funcs.MakeFileBase64Sha512Func(baseDir),
		"filemd5":          funcs.MakeFileMd5Func(baseDir),
		"filesha1":         funcs.MakeFileSha1Func(baseDir),
		"filesha256":       funcs.MakeFileSha256Func(baseDir),
		"filesha512":       funcs.MakeFileSha512Func(baseDir),
		"flatten":          stdlib.FlattenFunc,
		"floor":            stdlib.FloorFunc,
		"format":           stdlib.FormatFunc,
		"formatdate":       stdlib.FormatDateFunc,
		"formatlist":       stdlib.FormatListFunc,
		"indent":           stdlib.IndentFunc,
		"index":            funcs.IndexFunc, // stdlib.IndexFunc is not compatible
		"join":             stdlib.JoinFunc,
		"jsondecode":       stdlib.JSONDecodeFunc,
		"jsonencode":       stdlib.JSONEncodeFunc,
		"keys":             stdlib.KeysFunc,
		"length":           funcs.LengthFunc,
		"list":             funcs.ListFunc,
		"log":              stdlib.LogFunc,
		"lookup":           funcs.LookupFunc,
		"lower":            stdlib.LowerFunc,
		"map":              funcs.MapFunc,
		"matchkeys":        funcs.MatchkeysFunc,
		"max":              stdlib.MaxFunc,
		"md5":              funcs.Md5Func,
		"merge":            stdlib.MergeFunc,
		"min":              stdlib.MinFunc,
		"one":              funcs.OneFunc,
		"parseint":         stdlib.ParseIntFunc,
		"pathexpand":       funcs.PathExpandFunc,
		"pow":              stdlib.PowFunc,
		"range":            stdlib.RangeFunc,
		"regex":            stdlib.RegexFunc,
		"regexall":         stdlib.RegexAllFunc,
		"replace":          funcs.ReplaceFunc,
		"reverse":          stdlib.ReverseListFunc,
		"rsadecrypt":       funcs.RsaDecryptFunc,
		"sensitive":        funcs.SensitiveFunc,
		"nonsensitive":     funcs.NonsensitiveFunc,
		"issensitive":      funcs.IsSensitiveFunc,
		"setintersection":  stdlib.SetIntersectionFunc,
		"setproduct":       stdlib.SetProductFunc,
		"setsubtract":      stdlib.SetSubtractFunc,
		"setunion":         stdlib.SetUnionFunc,
		"sha1":             funcs.Sha1Func,
		"sha256":           funcs.Sha256Func,
		"sha512":           funcs.Sha512Func,
		"signum":           stdlib.SignumFunc,
		"slice":            stdlib.SliceFunc,
		"sort":             stdlib.SortFunc,
		"split":            stdlib.SplitFunc,
		"startswith":       funcs.StartsWithFunc,
		"strcontains":      funcs.StrContainsFunc,
		"strrev":           stdlib.ReverseFunc,
		"substr":           stdlib.SubstrFunc,
		"sum":              funcs.SumFunc,
		"textdecodebase64": funcs.TextDecodeBase64Func,
		"textencodebase64": funcs.TextEncodeBase64Func,
		"timestamp":        funcs.TimestampFunc,
		"timeadd":          stdlib.TimeAddFunc,
		"timecmp":          funcs.TimeCmpFunc,
		"title":            stdlib.TitleFunc,
		"tostring":         funcs.MakeToFunc(cty.String),
		"tonumber":         funcs.MakeToFunc(cty.Number),
		"tobool":           funcs.MakeToFunc(cty.Bool),
		"toset":            funcs.MakeToFunc(cty.Set(cty.DynamicPseudoType)),
		"tolist":           funcs.MakeToFunc(cty.List(cty.DynamicPseudoType)),
		"tomap":            funcs.MakeToFunc(cty.Map(cty.DynamicPseudoType)),
		"transpose":        funcs.TransposeFunc,
		"trim":             stdlib.TrimFunc,
		"trimprefix":       stdlib.TrimPrefixFunc,
		"trimspace":        stdlib.TrimSpaceFunc,
		"trimsuffix":       stdlib.TrimSuffixFunc,
		"try":              tryfunc.TryFunc,
		"upper":            stdlib.UpperFunc,
		"urlencode":        funcs.URLEncodeFunc,
		"urldecode":        funcs.URLDecodeFunc,
		"uuid":             funcs.UUIDFunc,
		"uuidv5":           funcs.UUIDV5Func,
		"values":           stdlib.ValuesFunc,
		"yamldecode":       ctyyaml.YAMLDecodeFunc,
		"yamlencode":       ctyyaml.YAMLEncodeFunc,
		"zipmap":           stdlib.ZipmapFunc,
	}

	ret["templatefile"] = funcs.MakeTemplateFileFunc(baseDir, func() map[string]function.Function {
		// The templatefile function prevents recursive calls to itself
		// by copying this map and overwriting the "templatefile" entry.
		return ret
	})

	// Registers "templatestring" function in function map.
	ret["templatestring"] = funcs.MakeTemplateStringFunc(baseDir, func() map[string]function.Function {
		// This anonymous function returns the existing map of functions for initialization.
		return ret
	})

	return ret
}
