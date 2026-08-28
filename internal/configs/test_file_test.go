// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/spf13/afero"
)

func TestTestRun_Validate(t *testing.T) {
	tcs := map[string]struct {
		expectedFailures []string
		diagnostic       string
	}{
		"empty": {},
		"supports_expected": {
			expectedFailures: []string{
				"check.expected_check",
				"var.expected_var",
				"output.expected_output",
				"test_resource.resource",
				"resource.test_resource.resource",
				"data.test_resource.resource",
			},
		},
		"count": {
			expectedFailures: []string{
				"count.index",
			},
			diagnostic: "You cannot expect failures from count.index. You can only expect failures from checkable objects such as input variables, output values, check blocks, managed resources and data sources.",
		},
		"foreach": {
			expectedFailures: []string{
				"each.key",
			},
			diagnostic: "You cannot expect failures from each.key. You can only expect failures from checkable objects such as input variables, output values, check blocks, managed resources and data sources.",
		},
		"local": {
			expectedFailures: []string{
				"local.value",
			},
			diagnostic: "You cannot expect failures from local.value. You can only expect failures from checkable objects such as input variables, output values, check blocks, managed resources and data sources.",
		},
		"module": {
			expectedFailures: []string{
				"module.my_module",
			},
			diagnostic: "You cannot expect failures from module.my_module. You can only expect failures from checkable objects such as input variables, output values, check blocks, managed resources and data sources.",
		},
		"path": {
			expectedFailures: []string{
				"path.walk",
			},
			diagnostic: "You cannot expect failures from path.walk. You can only expect failures from checkable objects such as input variables, output values, check blocks, managed resources and data sources.",
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			run := &TestRun{}
			for _, addr := range tc.expectedFailures {
				run.ExpectFailures = append(run.ExpectFailures, parseTraversal(t, addr))
			}

			diags := run.Validate()

			if len(diags) > 1 {
				t.Fatalf("too many diags: %d", len(diags))
			}

			if len(tc.diagnostic) == 0 {
				if len(diags) != 0 {
					t.Fatalf("expected no diags but got: %s", diags[0].Description().Detail)
				}

				return
			}

			if diff := cmp.Diff(tc.diagnostic, diags[0].Description().Detail); len(diff) > 0 {
				t.Fatalf("unexpected diff:\n%s", diff)
			}
		})
	}
}

func parseTraversal(t *testing.T, addr string) hcl.Traversal {
	t.Helper()

	traversal, diags := hclsyntax.ParseTraversalAbs([]byte(addr), "", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("invalid address: %s", diags.Error())
	}
	return traversal
}

func assertDiagsSummaryMatch(t *testing.T, want hcl.Diagnostics, got hcl.Diagnostics) {
	t.Helper()

	for i := range want {
		if want[i].Summary != got[i].Summary {
			t.Errorf("wanted %s as summary, got %s instead", want[i].Summary, got[i].Summary)
		}
	}
}

func TestDecodeTestRunModuleBlock(t *testing.T) {
	tcs := map[string]struct {
		inputModuleSource string
		wantModuleSource  string
		expectedDiags     hcl.Diagnostics
	}{
		"invalid": {
			inputModuleSource: "hg",
			wantModuleSource:  "",
			expectedDiags: hcl.Diagnostics{
				{
					Summary: "Invalid module source address",
				},
			},
		},
		"generic_git_url": {
			inputModuleSource: "git@github.com:opentofu/terraform-module-test.git",
			wantModuleSource:  "git::ssh://git@github.com/opentofu/terraform-module-test.git",
			expectedDiags:     nil,
		},
		"github_url": {
			inputModuleSource: "github.com/opentofu/terraform-module-test",
			wantModuleSource:  "git::https://github.com/opentofu/terraform-module-test.git",
			expectedDiags:     nil,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			pos := hcl.Pos{Line: 1, Column: 1}
			exprName := fmt.Sprintf("\"%s\"", tc.inputModuleSource)
			expr, _ := hclsyntax.ParseExpression([]byte(exprName), "", pos)

			block := &hcl.Block{
				Type: "module",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"source": {
							Name: "source",
							Expr: expr,
						},
					},
				}),
				DefRange: blockRange,
			}

			trcm, diags := decodeTestRunModuleBlock(block)

			if tc.expectedDiags != nil || diags != nil {
				assertDiagsSummaryMatch(t, tc.expectedDiags, diags)
				return
			}

			if len(diags) > 1 {
				t.Fatalf("not expecting errors, but got: %d", len(diags))
			}

			if trcm.Source == nil {
				t.Fatalf("was expecting to have a source, but did not: %d", trcm.Source)
			}

			if trcm.Source.String() != tc.wantModuleSource {
				t.Fatalf("got %#v; want %#v", trcm.Source.String(), tc.wantModuleSource)
			}
		})
	}
}

func TestLoadMockDataFiles_SingleFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := `
	mock_resource "resource_test" {
		defaults = {
			id = "mocked_id"
		}
	}
	
	override_data {
		target = data.test_data.example
		values = {
			id = "override_id"
		}
	}
	`

	if err := afero.WriteFile(fs, "/mocks/single.tfmock.hcl", []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %v", err)
	}

	p := NewParser(fs)

	mockResources, overrideResources, diags := p.loadMockDataFiles("/mocks/single.tfmock.hcl", blockRange)

	if diags.HasErrors() {
		t.Fatalf("Unexpected diagonistic: %s", diags)
	}

	if len(mockResources) != 1 || mockResources[0].Type != "resource_test" {
		t.Fatalf("Expected 1 mock resource of type resource_test but got %#v", mockResources)
	}

	if len(overrideResources) != 1 || overrideResources[0].getBlockName() != "override_data" {
		t.Fatalf("Expected 1 override_data resource got %#v", overrideResources)
	}
}

func TestLoadMockDataFiles_DirectoryWithMultipleFiles(t *testing.T) {
	fs := afero.NewMemMapFs()
	content_one := `
	mock_resource "resource_one" {
		defaults = {
			id = "one_id"
		}
	}
	`

	if err := afero.WriteFile(fs, "/mocks/one.tfmock.hcl", []byte(content_one), 0644); err != nil {
		t.Fatalf("failed to write %v", err)
	}

	content_two := `
	mock_resource "resource_two" {
		defaults = {
			id = "one_id"
		}
	}
	`

	if err := afero.WriteFile(fs, "/mocks/two.tfmock.hcl", []byte(content_two), 0644); err != nil {
		t.Fatalf("failed to write %v", err)
	}

	p := NewParser(fs)

	mockResources, overrideResources, diags := p.loadMockDataFiles("/mocks", blockRange)

	if diags.HasErrors() {
		t.Fatalf("Unexpected diagonistic: %s", diags)
	}

	if len(overrideResources) != 0 {
		t.Fatalf("Expected 0 override resources, got %d", len(overrideResources))
	}

	if len(mockResources) != 2 {
		t.Fatalf("Expected 2 mock resources, got %d", len(mockResources))
	}

	resourcesExtractedTypes := map[string]bool{}

	for _, res := range mockResources {
		resourcesExtractedTypes[res.Type] = true
	}

	if !resourcesExtractedTypes["resource_one"] || !resourcesExtractedTypes["resource_two"] {
		t.Fatalf("Expected resource_one and resource_two, got %v", resourcesExtractedTypes)
	}

}

func TestLoadMockDataFiles_DirectoryWithTofuShadowsTf(t *testing.T) {
	fs := afero.NewMemMapFs()
	content_tofu := `
	mock_resource "resource" {
		defaults = {
			id = "tofu_id"
		}
	}
	`

	if err := afero.WriteFile(fs, "/mocks/one.tofumock.hcl", []byte(content_tofu), 0644); err != nil {
		t.Fatalf("failed to write %v", err)
	}

	content_tf := `
	mock_resource "resource" {
		defaults = {
			id = "tf_id"
		}
	}
	`

	if err := afero.WriteFile(fs, "/mocks/one.tfmock.hcl", []byte(content_tf), 0644); err != nil {
		t.Fatalf("failed to write %v", err)
	}

	p := NewParser(fs)

	mockResources, overrideResources, diags := p.loadMockDataFiles("/mocks", blockRange)

	if diags.HasErrors() {
		t.Fatalf("Unexpected diagonistic: %s", diags)
	}

	if len(overrideResources) != 0 {
		t.Fatalf("Expected 0 override resources, got %d", len(overrideResources))
	}

	if len(mockResources) != 1 {
		t.Fatalf("Expected 1 mock resources, got %d", len(mockResources))
	}

	if mockResources[0].Defaults["id"].AsString() != "tofu_id" {
		t.Fatalf("Expected the .tofumock.hcl version to win, got id=%v", mockResources[0].Defaults["id"])
	}

}

func TestLoadMockDataFiles_MissingPath(t *testing.T) {
	fs := afero.NewMemMapFs()
	p := NewParser(fs)

	_, _, diags := p.loadMockDataFiles("/does/not/exist", blockRange)

	if !diags.HasErrors() {
		t.Fatalf("Expected an error diagnostic for a missing path, got none")
	}
}

func TestDecodeMockProviderBlock_InlineWinsOverSource(t *testing.T) {
	fs := afero.NewMemMapFs()

	content_source := `
	mock_resource "resource" {
		defaults = {
			id = "source_id"
		}
	}
	`

	if err := afero.WriteFile(fs, "/mocks/one.tfmock.hcl", []byte(content_source), 0644); err != nil {
		t.Fatalf("failed to write %v", err)
	}

	content_provider := `
	mock_provider "test" {
		source = "/mocks/one.tfmock.hcl"

		mock_resource "resource" {
			defaults = {
				id = "inline_id"
			}
		}
	}
	`

	f, parseDiags := hclsyntax.ParseConfig([]byte(content_provider), "main.tftest.hcl", hcl.InitialPos)
	if parseDiags.HasErrors() {
		t.Fatalf("Failed to parse fixture: %s", parseDiags)
	}

	fileContent, contentDiags := f.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: blockNameMockProvider, LabelNames: []string{"name"}},
		},
	})
	if contentDiags.HasErrors() {
		t.Fatalf("Failed to extract mock_provider block: %s", contentDiags)
	}
	if len(fileContent.Blocks) != 1 {
		t.Fatalf("Expected 1 mock_provider block, got %d", len(fileContent.Blocks))
	}

	p := NewParser(fs)

	provider, diags := p.decodeMockProviderBlock(fileContent.Blocks[0], "")

	if diags.HasErrors() {
		t.Fatalf("Unexpected diagonistic: %s", diags)
	}

	if len(provider.MockResources) != 1 {
		t.Fatalf("Expected 1 mock resource (inline should win, source duplicate skipped), got %d", len(provider.MockResources))
	}

	if provider.MockResources[0].Defaults["id"].AsString() != "inline_id" {
		t.Fatalf("Expected inline mock_resource to win over source, got id=%v", provider.MockResources[0].Defaults["id"])
	}
}
