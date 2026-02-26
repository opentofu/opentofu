// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/views/jsonfunction"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty/function"
)

var (
	ignoredFunctions = []addrs.Function{
		addrs.ParseFunction("map").FullyQualified(),
		addrs.ParseFunction("list").FullyQualified(),
	}
)

type MetadataFunctions interface {
	Diagnostics(diags tfdiags.Diagnostics)
	// PrintFunctions returns true if it managed to print the functions and false otherwise.
	PrintFunctions() bool
}

// NewMetadataFunctions returns an initialized MetadataFunctions implementation for the given ViewType.
// In case of this command, the returned [MetadataFunctions] will always print the diagnostics in human format
// and the functions in JSON format.
func NewMetadataFunctions(view *View) MetadataFunctions {
	return &MetadataFunctionsHuman{view: view}
}

type MetadataFunctionsHuman struct {
	view *View
}

var _ MetadataFunctions = (*MetadataFunctionsHuman)(nil)

func (v *MetadataFunctionsHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *MetadataFunctionsHuman) PrintFunctions() bool {
	scope := &lang.Scope{}
	funcs := scope.Functions()
	filteredFuncs := make(map[string]function.Function)
	for k, f := range funcs {
		if isIgnoredFunction(k) {
			continue
		}
		filteredFuncs[k] = f
	}

	jsonFunctions, marshalDiags := jsonfunction.Marshal(filteredFuncs)
	if marshalDiags.HasErrors() {
		v.Diagnostics(marshalDiags)
		return false
	}
	_, _ = v.view.streams.Println(string(jsonFunctions))
	return true
}

func isIgnoredFunction(name string) bool {
	funcAddr := addrs.ParseFunction(name).FullyQualified().String()
	for _, i := range ignoredFunctions {
		if funcAddr == i.String() {
			return true
		}
	}
	return false
}
