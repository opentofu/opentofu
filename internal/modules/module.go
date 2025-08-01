// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package modules

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/experiments"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tofuVersion "github.com/opentofu/opentofu/version"
)

// Module represents the mostly-unevaluated content of a single module, before
// it has been included into a configuration tree and had early evaluation
// applied to it.
//
// This is not the representation of a module that most parts of OpenTofu should
// use. Instead, it's an intermediate representation reflecting the static
// structure of the configuration. (The "final" form of a configuration would
// live in some other package and use different struct types.)
type Module struct {
	Dir string
}

func LoadModuleDir(dir string) (*Module, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &Module{
		Dir: dir,
	}

	primaryFilenames, overrideFilenames, _, diags := filesInModuleDir(dir, "")
	if diags.HasErrors() {
		// If we can't even discover which files we're loading then we'll
		// bail out early here because continuing would probably produce
		// confusing redundant error messages.
		return ret, diags
	}

	primaryFiles := loadConfigFiles(primaryFilenames)
	overrideFiles := loadConfigFiles(overrideFilenames)
	diags = diags.Append(precheckLanguageCompatibility(primaryFiles, tofuVersion.SemVer))
	diags = diags.Append(precheckLanguageCompatibility(overrideFiles, tofuVersion.SemVer))
	if diags.HasErrors() {
		// If the module declared that it isn't compatible with this version
		// then we can expect that some other parts of the configuration will
		// seem invalid to the following code, and so we'll return early to
		// avoid returning confusing errors, focusing only on the version
		// incompatibility.
		return ret, diags
	}

	// TODO: Decode everything else!

	return ret, diags
}

func loadConfigFiles(filenames []string) []*configFile {
	if len(filenames) == 0 {
		return nil
	}
	files := make([]*configFile, len(filenames))
	for i, filename := range filenames {
		files[i] = parseConfigFile(filename)
	}
	return files
}

func precheckLanguageCompatibility(files []*configFile, opentofuVersion *version.Version) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, file := range files {
		if file.SupportedOpenTofuVersions != nil && !file.SupportedOpenTofuVersions.Value.Check(opentofuVersion) {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Incompatible module",
				Detail:   fmt.Sprintf("This module declares that it is not compatible with OpenTofu %s.\n\nYou may need to either use a different OpenTofu version or select a different version of this module.", opentofuVersion),
				Subject:  file.SupportedOpenTofuVersions.SourceRange.ToHCL().Ptr(),
			})
		}
		for _, selected := range file.SelectedLanguageExperiments {
			experiment, err := experiments.GetCurrent(selected.Value)
			switch err := err.(type) {
			case nil:
				// no problem: selected experiment is currently active
			case experiments.UnavailableError:
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Module uses unsupported language experiment",
					Detail:   fmt.Sprintf("This module relies on features from the language experiment %q, which is not available in OpenTofu %s.", err.ExperimentName, opentofuVersion),
					Subject:  selected.SourceRange.ToHCL().Ptr(),
				})
			case experiments.ConcludedError:
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Module uses concluded language experiment",
					Detail:   fmt.Sprintf("This module relies on features from the language experiment %q, which has concluded.\n\n%s.", err.ExperimentName, err.Message),
					Subject:  selected.SourceRange.ToHCL().Ptr(),
				})
			default:
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Module uses invalid language experiment",
					Detail:   fmt.Sprintf("This module refers to language experiment %q, which is invalid: %s.", experiment.Keyword(), err),
					Subject:  selected.SourceRange.ToHCL().Ptr(),
				})
			}
		}
	}
	return diags
}

// filesInModuleDir finds OpenTofu configuration files within dir, splitting
// them into primary and override files based on the filename.
//
// If testsDir is not empty, filesInModuleDir will also retrieve OpenTofu
// testing files both directly within dir and within testsDir as a subdirectory
// of dir. In this way, testsDir acts both as a direction to retrieve test
// files within the main direction and as the location for additional test
// files.
func filesInModuleDir(dir string, testsDir string) (primary, override, tests []string, diags tfdiags.Diagnostics) {
	includeTests := len(testsDir) > 0

	if includeTests {
		testPath := path.Join(dir, testsDir)

		infos, err := os.ReadDir(testPath)
		if err != nil {
			// Then we couldn't read from the testing directory for some reason.

			if os.IsNotExist(err) {
				// Then this means the testing directory did not exist.
				// We won't actually stop loading the rest of the configuration
				// for this, we will add a warning to explain to the user why
				// test files weren't processed but leave it at that.
				if testsDir != defaultTestDirectory {
					// We'll only add the warning if a directory other than the
					// default has been requested. If the user is just loading
					// the default directory then we have no expectation that
					// it should actually exist.
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagWarning,
						Summary:  "Test directory does not exist",
						Detail:   fmt.Sprintf("Requested test directory %s does not exist.", testPath),
					})
				}
			} else {
				// Then there is some other reason we couldn't load. We will
				// treat this as a full error.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Failed to read test directory",
					Detail:   fmt.Sprintf("Test directory %s could not be read: %v.", testPath, err),
				})

				// We'll also stop loading the rest of the config for this.
				return
			}

		} else {
			for _, testInfo := range infos {
				if testInfo.IsDir() || IsIgnoredFile(testInfo.Name()) {
					continue
				}

				ext := fileExt(testInfo.Name())
				if isTestFileExt(ext) {
					tests = append(tests, filepath.Join(testPath, testInfo.Name()))
				}
			}
		}

	}

	infos, err := os.ReadDir(dir)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Failed to read module directory",
			Detail:   fmt.Sprintf("Module directory %s does not exist or cannot be read.", dir),
		})
		return
	}

	for _, info := range infos {
		if info.IsDir() {
			// We only care about tofu configuration files.
			continue
		}

		name := info.Name()
		ext := fileExt(name)
		if ext == "" || IsIgnoredFile(name) {
			continue
		}

		if isTestFileExt(ext) {
			if includeTests {
				tests = append(tests, filepath.Join(dir, name))
			}
			continue
		}

		baseName := name[:len(name)-len(ext)] // strip extension
		isOverride := baseName == "override" || strings.HasSuffix(baseName, "_override")

		fullPath := filepath.Join(dir, name)
		if isOverride {
			override = append(override, fullPath)
		} else {
			primary = append(primary, fullPath)
		}
	}

	return filterTfPathsWithTofuAlternatives(primary), filterTfPathsWithTofuAlternatives(override), filterTfPathsWithTofuAlternatives(tests), diags
}

// filterTfPathsWithTofuAlternatives filters out .tf files if they have an
// alternative .tofu file with the same name.
// For example, if there are both 'resources.tf.json' and
// 'resources.tofu.json' files, the 'resources.tf.json' file will be ignored,
// and only the 'resources.tofu.json' file will be returned as a relevant path.
func filterTfPathsWithTofuAlternatives(paths []string) []string {
	var ignoredPaths []string
	var relevantPaths []string

	for _, p := range paths {
		ext := tfFileExt(p)

		if ext == "" {
			relevantPaths = append(relevantPaths, p)
			continue
		}

		parallelTofuExt := strings.ReplaceAll(ext, ".tf", ".tofu")
		pathWithoutExt, _ := strings.CutSuffix(p, ext)
		parallelTofuPath := pathWithoutExt + parallelTofuExt

		// If the .tf file has a parallel .tofu file in the directory,
		// we'll ignore the .tf file and only use the .tofu file
		if slices.Contains(paths, parallelTofuPath) {
			ignoredPaths = append(ignoredPaths, p)
		} else {
			relevantPaths = append(relevantPaths, p)
		}
	}

	if len(ignoredPaths) > 0 {
		log.Printf("[INFO] filterTfPathsWithTofuAlternatives: Ignored the following .tf files because a .tofu file alternative exists: %q", ignoredPaths)
	}

	return relevantPaths
}

// fileExt returns the OpenTofu configuration extension of the given
// path, or a blank string if it is not a recognized extension.
func fileExt(path string) string {
	extension := tfFileExt(path)

	if extension == "" {
		extension = tofuFileExt(path)
	}

	return extension
}

// tfFileExt returns the OpenTofu .tf configuration extension of the given
// path, or an empty string if it is not a recognized .tf extension.
func tfFileExt(path string) string {
	switch {
	case strings.HasSuffix(path, tfExt):
		return tfExt
	case strings.HasSuffix(path, tfJSONExt):
		return tfJSONExt
	case strings.HasSuffix(path, tfTestExt):
		return tfTestExt
	case strings.HasSuffix(path, tfTestJSONExt):
		return tfTestJSONExt
	default:
		return ""
	}
}

// tofuFileExt returns the OpenTofu .tofu configuration extension of the given
// path, or a blank string if it is not a recognized .tofu extension.
func tofuFileExt(path string) string {
	switch {
	case strings.HasSuffix(path, tofuExt):
		return tofuExt
	case strings.HasSuffix(path, tofuJSONExt):
		return tofuJSONExt
	case strings.HasSuffix(path, tofuTestExt):
		return tofuTestExt
	case strings.HasSuffix(path, tofuTestJSONExt):
		return tofuTestJSONExt
	}

	return ""
}

func isTestFileExt(ext string) bool {
	return ext == tfTestExt || ext == tfTestJSONExt || ext == tofuTestExt || ext == tofuTestJSONExt
}

// IsIgnoredFile returns true if the given filename (which must not have a
// directory path ahead of it) should be ignored as e.g. an editor swap file.
func IsIgnoredFile(name string) bool {
	return strings.HasPrefix(name, ".") || // Unix-like hidden files
		strings.HasSuffix(name, "~") || // vim
		strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") // emacs
}

const (
	defaultTestDirectory = "tests"
)

const (
	tfExt           = ".tf"
	tofuExt         = ".tofu"
	tfJSONExt       = ".tf.json"
	tofuJSONExt     = ".tofu.json"
	tfTestExt       = ".tftest.hcl"
	tofuTestExt     = ".tofutest.hcl"
	tfTestJSONExt   = ".tftest.json"
	tofuTestJSONExt = ".tofutest.json"
)
