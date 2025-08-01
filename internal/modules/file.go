// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package modules

import (
	"fmt"
	"os"
	"strings"

	version "github.com/hashicorp/go-version"
	hcl "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// configFile represents a single ".tofu", ".tf", etc configuration file in
// isolation, before assembling potentially-many files together to represent
// a single module.
//
// This is intentionally only very shallowly-decoded, deferring as much work
// as possible until after a caller has collected all of the files in a module
// and so is able to consider them together.
//
// It's unexported because it's used only during the early part of the
// configuration decoding process; everything outside of this package should
// be thinking about entire modules, not the individual source files they
// were made from.
type configFile struct {
	// Filename is the filepath of the file that this data was built from.
	Filename string

	// SupportedOpenTofuVersions represents which versions of OpenTofu the
	// module author declared it to be compatible with.
	//
	// This is eagerly decoded on a best-effort basis so that callers can
	// quickly reject incompatible modules while avoiding as much as possible
	// getting tripped up by incompatible language features in the rest of
	// the file, or in other files belonging to the same module.
	SupportedOpenTofuVersions *WithSourceRange[*version.Constraints]

	// SelectedLanguageExperiments is the set of language experiment names that
	// the module has opted into.
	//
	// This is eagerly decoded on a best-effort basis so that all subsequent
	// decoding can potentially vary depending on which experiments are
	// selected across all files in a module.
	//
	// The language experiment names are not checked against the current
	// active experiments in this version of OpenTofu. It's the caller's
	// responsibility to check whether each experiment is name is valid to use.
	SelectedLanguageExperiments []WithSourceRange[string]

	// ConfigBlocks are all of the top-level blocks of supported types other
	// than "language" that were found in the file.
	//
	// The shallow decode of the top-level configuration structure verifies
	// that all blocks are of supported types and that each block has the
	// correct number of labels for its type, but all other analysis is deferred
	// to a later step.
	//
	// Blocks of unsupported types are not included at all, even if they appear
	// in the source file, but the Diagnostics field will contain errors about
	// them. Blocks of known types that don't have the correct number of
	// labels _may_ be included, depending on the error-handling details of
	// the specific HCL syntax in use, and so callers must take care when
	// accessing elements of this slice in structures where Diagnostics
	// contains errors.
	ConfigBlocks []*hcl.Block

	// Diagnostics are the diagnostics returned by HCL during either the
	// parsing or top-level shallow decoding step.
	//
	// These are returned as a property of the file, rather than as a
	// separate return value alongside this object, because callers are
	// expected to collect all of the files for a module and check their
	// SupportedOpenTofuVersions and ActiveLanguageExperiments fields before
	// making any other use of a file, and should avoid returning the
	// diagnostics from this field of any file in the module if at least one
	// file in the module has incompatible version requirements or unsupported
	// experiments.
	Diagnostics tfdiags.Diagnostics
}

func parseConfigSource(src []byte, filename string) *configFile {
	ret := &configFile{
		Filename: filename,
	}

	var hclFile *hcl.File
	var hclDiags hcl.Diagnostics
	if strings.HasSuffix(filename, ".json") {
		hclFile, hclDiags = hcljson.Parse(src, filename)
		ret.Diagnostics = ret.Diagnostics.Append(hclDiags)
	} else {
		hclFile, hclDiags = hclsyntax.ParseConfig(src, filename, hcl.InitialPos)
		ret.Diagnostics = ret.Diagnostics.Append(hclDiags)
	}
	if hclFile == nil {
		// If the syntax was so invalid that we couldn't even get a file
		// then we'll just bail out here with a diagnostics-only result.
		return ret
	}

	content, hclDiags := hclFile.Body.Content(configFileSchema)
	ret.Diagnostics = ret.Diagnostics.Append(hclDiags)
	ret.ConfigBlocks = content.Blocks

	// Although we intentionally defer decoding _most_ of the deeper-level
	// content so the caller can load all of a module's files first, the
	// supported OpenTofu versions and enabled language experiments could
	// potentially cause us to skip any deeper decoding _at all_, and so
	// we make a best effort to decode those early here.
	reqdVersions, experiments, moreDiags := sniffConfigFileLanguageSettings(ret)
	ret.SupportedOpenTofuVersions = reqdVersions
	ret.SelectedLanguageExperiments = experiments
	ret.Diagnostics = ret.Diagnostics.Append(moreDiags)

	return ret
}

func sniffConfigFileLanguageSettings(f *configFile) (*WithSourceRange[*version.Constraints], []WithSourceRange[string], tfdiags.Diagnostics) {
	// TODO: Search f.ConfigBlocks for a "terraform" block and do a careful
	// decode of its contents to try to find required_version, experiments,
	// and language arguments, and then make a best effort to evaluate them.
	return nil, nil, nil
}

func parseConfigFile(filename string) *configFile {
	src, err := os.ReadFile(filename)
	if err != nil {
		ret := &configFile{
			Filename: filename,
		}
		if os.IsNotExist(err) {
			ret.Diagnostics = ret.Diagnostics.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Configuration file not found",
				fmt.Sprintf("There is no file at %q.", filename),
			))
		} else {
			ret.Diagnostics = ret.Diagnostics.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to open configuration file",
				fmt.Sprintf("Cannot open %q: %s.", filename, err),
			))
		}
		return ret
	}
	return parseConfigSource(src, filename)
}

// configFileSchema is the schema for the top-level of a config file. We use
// the low-level HCL API for this level so we can easily deal with each
// block type separately with its own decoding logic.
var configFileSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "terraform"},
		{Type: "required_providers"},
		{Type: "provider", LabelNames: []string{"name"}},
		{Type: "variable", LabelNames: []string{"name"}},
		{Type: "locals"},
		{Type: "output", LabelNames: []string{"name"}},
		{Type: "module", LabelNames: []string{"name"}},
		{Type: "resource", LabelNames: []string{"type", "name"}},
		{Type: "data", LabelNames: []string{"type", "name"}},
		{Type: "moved"},
		{Type: "import"},
		{Type: "check", LabelNames: []string{"name"}},
		{Type: "removed"},
	},
}
