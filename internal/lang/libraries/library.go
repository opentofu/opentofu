// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libraries

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Library describes the declarations in a library before it has been compiled,
// so that the rest of the system can ensure that the needed dependencies are
// available and all of the callers are using the library's definitions
// correctly before actually compiling the library.
//
// Use [Library.Compile] to bind the library to a particular evaluation
// context so that its definitions can actually be used.
type Library struct {
	// SourceAddr is the source address that the library was loaded from.
	SourceAddr addrs.ModuleSource

	// Values, Functions, and TypeAliases represent the exported symbols from
	// a library.
	//
	// Libraries are also allowed to declare private symbols used only from
	// elsewhere inside the library, and those are not included in these
	// maps because they are considered an implementation detail of the
	// library, not visible from outside of it.
	Values      map[string]Value
	Functions   map[string]Function
	TypeAliases map[string]TypeAlias

	// RequiredLibraries describes which other libraries (if any) the
	// library depends on.
	//
	// The module packages containing these other libraries must be installed
	// by "tofu init" (or similar) whenever the configuration includes a module
	// that refers to this library. It's the module package installer's
	// responsibility to handle the case where a library depends on itself
	// (directly or indirectly) and thus avoid infinite recursion during
	// installation.
	RequiredLibraries []LibraryRequirement

	// RequiredProviders describes which providers (if any) the library
	// depends on for provider-defined functions.
	//
	// These providers must be installed by "tofu init" (or similar) whenever
	// the configuration includes a module that refers to this provider, and
	// then the functions from each of these providers must be available when
	// the library is compiled.
	RequiredProviders map[addrs.Provider]ProviderRequirement

	// exportedSymbols are the symbols defined in the library that are available
	// for used by the library's caller.
	exportedSymbols *symbols
	// allSymbols is the full set of symbols combining both the exported and
	// private symbols, which is our main lookup table for resolving references
	// during library compilation.
	allSymbols *symbols
}

// LoadLibrary attempts to load a library from the path given in localFilename,
// with the assumption that it's a local copy of the file with the source
// address given in sourceAddr, possibly fetched from a remote location into
// a local cache directory by "tofu init".
//
// Libraries are distributed along with modules as part of module packages,
// so they can reuse all of the same source address schemes and the module
// registry protocol, although the source address for a library refers to
// a single file with a ".tofulib.hcl" filename suffix, rather than to a whole
// directory containing multiple files.
//
// The given source address should either be a local address relative to the
// directory containing the root module for libraries distributed along with
// the root module, or an absolute remote or registry source address for
// libraries distributed in non-local module packages. The source address should
// be local when localFilename refers into the cache directory where "tofu init"
// places packages fetched from remote sources.
//
// If the returned diagnostics contains errors then the returned [Library] may
// either be nil or incomplete.
func LoadLibrary(sourceAddr addrs.ModuleSource, localFilename string) (*Library, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	src, err := os.ReadFile(localFilename)
	if err != nil {
		if _, ok := sourceAddr.(addrs.ModuleSourceLocal); ok {
			// If it's a local address then sourceAddr will not provide
			// any more information than localFilename does, so we'll just
			// report with localFilename alone to avoid redundancy.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to load library",
				fmt.Sprintf("Cannot read library source from %s: %s.", localFilename, tfdiags.FormatError(err)),
			))
		} else {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to load library",
				fmt.Sprintf("Cannot read library source code for %q from %s: %s.", sourceAddr, localFilename, tfdiags.FormatError(err)),
			))
		}
		return nil, diags
	}

	// After this point we use the source address exclusively, because that
	// avoids directly exposing implementation details of our strategy for
	// caching module packages on local disk.
	ret, moreDiags := parseLibrary(src, sourceAddr)
	diags = diags.Append(moreDiags)
	return ret, diags
}

func parseLibrary(src []byte, sourceAddr addrs.ModuleSource) (*Library, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	f, hclDiags := hclsyntax.ParseConfig(src, sourceAddr.String(), hcl.InitialPos)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret, moreDiags := decodeLibrary(f.Body, sourceAddr)
	diags = diags.Append(moreDiags)
	return ret, diags
}

func decodeLibrary(body hcl.Body, sourceAddr addrs.ModuleSource) (*Library, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &Library{
		SourceAddr:  sourceAddr,
		Values:      make(map[string]Value),
		Functions:   make(map[string]Function),
		TypeAliases: make(map[string]TypeAlias),
	}

	exportedSymbols, remain, moreDiags := decodeSymbols(body)
	diags = diags.Append(moreDiags)

	content, hclDiags := remain.Content(librarySchema)
	diags = diags.Append(hclDiags)

	for _, block := range content.Blocks {
		_ = block
	}

	_ = exportedSymbols

	return ret, diags
}

func (l *Library) Compile(ctx context.Context, compileCtx CompileContext) *CompiledLibrary {
	panic("unimplemented")
}

var librarySchema = &hcl.BodySchema{
	// The symbols defined here must be complementary to those in
	// [symbolsSchema], because this schema is applied to whatever is left
	// from the top-level body of a library file after [decodeSymbols] has
	// done its work.
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "private"},
		{Type: "required_providers"},
		{Type: "library", LabelNames: []string{"name"}},
	},
}

// privateSchema is the schema for what can remain in a "private" block after
// [decodeSymbols] has extracted the private symbol definitions.
var privateSchema = &hcl.BodySchema{
	// We currently expect nothing other than symbol definitions in a
	// "private" block.
}
