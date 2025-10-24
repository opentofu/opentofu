// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
)

// The functions in this file are intended only for use in tests written in
// this and other packages, offering various shortcuts for getting
// modules and configurations built in preparation for testing other systems
// whose behavior depends on configuration.

// ModuleFromStringForTesting interprets the given string as if it were the
// content of a ".tofu" file in a module directory, parsing and decoding it
// as a single-file module.
//
// Note that THIS FUNCTION DOES NOT PERFORM EARLY EVALUATION. This is intended
// mainly for an experimental new config evaluation strategy where early
// evaluation and config tree assembly are handled outside of this package.
//
// If the configuration is not valid then this halts testing by calling
// [testing.TB.FailNow].
//
// Language experiments are always allowed in the "modules" loaded by this
// function.
func ModuleFromStringForTesting(t testing.TB, src string) *Module {
	t.Helper()
	ret := moduleFromStringForTesting(t, src, "<ModuleFromStringForTesting>")
	if ret == nil {
		t.FailNow() // prevent further execution if the config was invalid
	}
	return ret
}

// ModulesFromStringsForTesting calls [ModuleFromStringForTesting] for each
// element of the given map and then treats the map keys as local module
// source addresses to construct a map from source address to module.
//
// As with [ModuleFromStringForTesting], if any of the given configuration
// strings are invalid then this halts testing by calling [testing.TB.FailNow],
// and experiments are always allowed. The map keys must also be valid local
// module source addresses.
func ModulesFromStringsForTesting(t testing.TB, srcs map[string]string) map[addrs.ModuleSourceLocal]*Module {
	t.Helper()

	if len(srcs) == 0 {
		return nil // weird to ask for nothing, but okay!
	}
	ret := make(map[addrs.ModuleSourceLocal]*Module, len(srcs))
	problem := false
	for sourceAddrRaw, sourceRaw := range srcs {
		sourceAddr, err := addrs.ParseModuleSource(sourceAddrRaw)
		if err != nil {
			t.Errorf("invalid source address %q: %s", sourceAddrRaw, err)
			problem = true
			continue
		}
		localSourceAddr, ok := sourceAddr.(addrs.ModuleSourceLocal)
		if !ok {
			t.Errorf("invalid source address %q: only _local_ source addresses are allowed", sourceAddrRaw)
			problem = true
			continue
		}

		module := moduleFromStringForTesting(t, sourceRaw, sourceAddrRaw+"/for-testing.tf")
		if module == nil {
			// moduleFromStringForTesting should already have written log
			// lines explaining the problem it encountered.
			problem = true
			continue
		}

		ret[localSourceAddr] = module
	}
	if problem {
		// If we encountered at least one problem in the loop above then
		// we'll halt testing now. (We wait to get here so that we can
		// report errors in multiple elements at once when appropriate.)
		t.FailNow()
	}
	return ret
}

// moduleFromStringForTesting is the common code from both
// [ModuleFromStringForTesting] and [ModulesFromStringsForTesting] which
// actually does the module loading.
//
// If errors occur then it calls t.Fail (indirectly) and emits log lines
// explaining the problem before returning nil. It's the caller's responsibility
// to halt further test execution with t.FailNow at some appropriate time.
func moduleFromStringForTesting(t testing.TB, src string, fakeFilename string) *Module {
	t.Helper()

	hclFile, diags := hclsyntax.ParseConfig([]byte(src), fakeFilename, hcl.InitialPos)
	if diags.HasErrors() {
		t.Errorf("unexpected syntax error: %s", diags.Error())
		return nil
	}

	file, diags := loadConfigFileBody(hclFile.Body, fakeFilename, false, true)
	if diags.HasErrors() {
		t.Errorf("unexpected file analysis error: %s", diags.Error())
		return nil
	}

	ret, diags := NewModuleUneval([]*File{file}, nil, fakeFilename, SelectiveLoadAll)
	if diags.HasErrors() {
		t.Errorf("unexpected module analysis error: %s", diags.Error())
		return nil
	}

	return ret
}
