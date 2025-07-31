// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

// TestParseLoadConfigDirSuccess is a simple test that just verifies that
// a number of test configuration directories (in testdata/valid-modules)
// can be parsed without raising any diagnostics.
//
// It also re-tests the individual files in testdata/valid-files as if
// they were single-file modules, to ensure that they can be bundled into
// modules correctly.
//
// This test does not verify that reading these modules produces the correct
// module element contents. More detailed assertions may be made on some subset
// of these configuration files in other tests.
func TestParserLoadConfigDirSuccess(t *testing.T) {
	dirs, err := os.ReadDir("testdata/valid-modules")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range dirs {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			parser := NewParser(nil)
			path := filepath.Join("testdata/valid-modules", name)

			mod, diags := parser.LoadConfigDir(path, RootModuleCallForTesting())
			if len(diags) != 0 && len(mod.ActiveExperiments) != 0 {
				// As a special case to reduce churn while we're working
				// through experimental features, we'll ignore the warning
				// that an experimental feature is active if the module
				// intentionally opted in to that feature.
				// If you want to explicitly test for the feature warning
				// to be generated, consider using testdata/warning-files
				// instead.
				filterDiags := make(hcl.Diagnostics, 0, len(diags))
				for _, diag := range diags {
					if diag.Severity != hcl.DiagWarning {
						continue
					}
					match := false
					for exp := range mod.ActiveExperiments {
						allowedSummary := fmt.Sprintf("Experimental feature %q is active", exp.Keyword())
						if diag.Summary == allowedSummary {
							match = true
							break
						}
					}
					if !match {
						filterDiags = append(filterDiags, diag)
					}
				}
				diags = filterDiags
			}
			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}

			if mod.SourceDir != path {
				t.Errorf("wrong SourceDir value %q; want %s", mod.SourceDir, path)
			}

			if len(mod.Tests) > 0 {
				// We only load tests when requested, and we didn't request this
				// time.
				t.Errorf("should not have loaded tests, but found %d", len(mod.Tests))
			}
		})
	}

	// The individual files in testdata/valid-files should also work
	// when loaded as modules.
	files, err := os.ReadDir("testdata/valid-files")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range files {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/valid-files", name))
			if err != nil {
				t.Fatal(err)
			}

			parser := testParser(map[string]string{
				"mod/" + name: string(src),
			})

			_, diags := parser.LoadConfigDir("mod", NewStaticModuleCall(addrs.RootModule,
				func(v *Variable) (cty.Value, hcl.Diagnostics) {
					if !v.Required() {
						// Allow defaults in this test
						return v.Default, nil
					}
					panic("Variables not configured for this test!")
				}, "<testing>", ""))
			if diags.HasErrors() {
				t.Errorf("unexpected error diagnostics")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}
		})
	}

}

func TestParserLoadConfigDirWithTests(t *testing.T) {
	directories := []string{
		"testdata/valid-modules/with-tests",
		"testdata/valid-modules/with-tests-expect-failures",
		"testdata/valid-modules/with-tests-nested",
		"testdata/valid-modules/with-tests-very-nested",
		"testdata/valid-modules/with-tests-json",
	}

	for _, directory := range directories {
		t.Run(directory, func(t *testing.T) {

			testDirectory := DefaultTestDirectory
			if directory == "testdata/valid-modules/with-tests-very-nested" {
				testDirectory = "very/nested"
			}

			parser := NewParser(nil)
			mod, diags := parser.LoadConfigDirWithTests(directory, testDirectory, RootModuleCallForTesting())
			if len(diags) > 0 { // We don't want any warnings or errors.
				t.Errorf("unexpected diagnostics")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}

			if len(mod.Tests) != 2 {
				t.Errorf("incorrect number of test files found: %d", len(mod.Tests))
			}
		})
	}
}

func TestParserLoadConfigDirWithTests_ReturnsWarnings(t *testing.T) {
	parser := NewParser(nil)
	mod, diags := parser.LoadConfigDirWithTests("testdata/valid-modules/with-tests", "not_real", RootModuleCallForTesting())
	if len(diags) != 1 {
		t.Errorf("expected exactly 1 diagnostic, but found %d", len(diags))
	} else {
		if diags[0].Severity != hcl.DiagWarning {
			t.Errorf("expected warning severity but found %d", diags[0].Severity)
		}

		if diags[0].Summary != "Test directory does not exist" {
			t.Errorf("expected summary to be \"Test directory does not exist\" but was \"%s\"", diags[0].Summary)
		}

		if diags[0].Detail != "Requested test directory testdata/valid-modules/with-tests/not_real does not exist." {
			t.Errorf("expected detail to be \"Requested test directory testdata/valid-modules/with-tests/not_real does not exist.\" but was \"%s\"", diags[0].Detail)
		}
	}

	// Despite the warning, should still have loaded the tests in the
	// configuration directory.
	if len(mod.Tests) != 2 {
		t.Errorf("incorrect number of test files found: %d", len(mod.Tests))
	}
}

// TestParseLoadConfigDirFailure is a simple test that just verifies that
// a number of test configuration directories (in testdata/invalid-modules)
// produce diagnostics when parsed.
//
// It also re-tests the individual files in testdata/invalid-files as if
// they were single-file modules, to ensure that their errors are still
// detected when loading as part of a module.
//
// This test does not verify that reading these modules produces any
// diagnostics in particular. More detailed assertions may be made on some subset
// of these configuration files in other tests.
func TestParserLoadConfigDirFailure(t *testing.T) {
	dirs, err := os.ReadDir("testdata/invalid-modules")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range dirs {
		name := info.Name()
		t.Run(name, func(t *testing.T) {
			parser := NewParser(nil)
			path := filepath.Join("testdata/invalid-modules", name)

			_, diags := parser.LoadConfigDir(path, RootModuleCallForTesting())
			if !diags.HasErrors() {
				t.Errorf("no errors; want at least one")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}
		})
	}

	// The individual files in testdata/valid-files should also work
	// when loaded as modules.
	files, err := os.ReadDir("testdata/invalid-files")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range files {
		name := info.Name()
		t.Run(fmt.Sprintf("%s as module", name), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/invalid-files", name))
			if err != nil {
				t.Fatal(err)
			}

			parser := testParser(map[string]string{
				"mod/" + name: string(src),
			})

			_, diags := parser.LoadConfigDir("mod", RootModuleCallForTesting())
			if !diags.HasErrors() {
				t.Errorf("no errors; want at least one")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}
		})
	}

}

func TestParserLoadConfigDirWithTests_TofuFiles(t *testing.T) {
	expectedVariablesToOverride := []string{"should_override", "should_override_json"}
	expectedLoadedTestFiles := []string{"test/resources_test.tofutest.hcl", "test/resources_test_json.tofutest.json"}

	tests := []struct {
		name              string
		path              string
		expectedResources []string
	}{
		{
			name:              "only .tofu files",
			path:              "testdata/tofu-only-files",
			expectedResources: []string{"aws_security_group.firewall_tofu", "aws_instance.web_tofu", "test_object.a_tofu", "test_object.b_tofu"},
		},
		{
			name:              ".tofu and .tf files",
			path:              "testdata/tofu-and-tf-files",
			expectedResources: []string{"aws_security_group.firewall_tofu", "aws_instance.web_tofu", "test_object.a_tofu", "test_object.b_tofu", "tf_resource.first", "tf_json_resource.a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(nil)
			path := tt.path

			mod, diags := parser.LoadConfigDirWithTests(path, "test", RootModuleCallForTesting())
			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics")
				for _, diag := range diags {
					t.Logf("- %s", diag)
				}
			}

			if mod.SourceDir != path {
				t.Errorf("wrong SourceDir value %q; want %s", mod.SourceDir, path)
			}

			if len(tt.expectedResources) != len(mod.ManagedResources) {
				t.Errorf("expected to find %d resources but instead got %d resources", len(tt.expectedResources), len(mod.ManagedResources))
			}

			for _, expectedResource := range tt.expectedResources {
				if mod.ManagedResources[expectedResource] == nil {
					t.Errorf("expected to load %s resource as part of configuration but it is missing", expectedResource)
				}
			}

			if len(expectedVariablesToOverride) != len(mod.Variables) {
				t.Errorf("expected to find %d variables but instead got %d resources", len(expectedVariablesToOverride), len(mod.Variables))
			}

			for _, expectedVariable := range expectedVariablesToOverride {
				variableInConfiguration := mod.Variables[expectedVariable]
				if variableInConfiguration == nil {
					t.Errorf("expected to load %s variable as part of configuration but it is missing", expectedVariable)
				} else if variableInConfiguration.Default.AsString() != "overridden by tofu file" {
					t.Errorf("expected variable default value %s to be overridden", expectedVariable)
				}
			}

			if len(mod.Tests) != 2 {
				t.Errorf("incorrect number of test files found: %d", len(mod.Tests))
			}

			for _, expectedTest := range expectedLoadedTestFiles {
				if mod.Tests[expectedTest] == nil {
					t.Errorf("expected to load %s test as part of configuration but it is missing", expectedTest)
				}
			}
		})
	}
}

func TestIsEmptyDir(t *testing.T) {
	val, err := IsEmptyDir(filepath.Join("testdata", "valid-files"))
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if val {
		t.Fatal("should not be empty")
	}
}

func TestIsEmptyDir_noExist(t *testing.T) {
	val, err := IsEmptyDir(filepath.Join("testdata", "nopenopenope"))
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if !val {
		t.Fatal("should be empty")
	}
}

func TestIsEmptyDir_noConfigs(t *testing.T) {
	val, err := IsEmptyDir(filepath.Join("testdata", "dir-empty"))
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if !val {
		t.Fatal("should be empty")
	}
}
