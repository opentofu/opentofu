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
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
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
		wantModuleSource string
		expectedDiags hcl.Diagnostics
	}{
		"invalid": {
			inputModuleSource: "hg",
			wantModuleSource: "",
			expectedDiags: hcl.Diagnostics{
				{
					Summary: "Invalid module source address",
				},
			},
		},
		"generic_git_url": {
			inputModuleSource: "git@github.com:opentofu/terraform-module-test.git",
			wantModuleSource: "git::ssh://git@github.com/opentofu/terraform-module-test.git",
			expectedDiags: nil,
		},
		"github_url": {
			inputModuleSource: "github.com/opentofu/terraform-module-test",
			wantModuleSource: "git::https://github.com/opentofu/terraform-module-test.git",
			expectedDiags: nil,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			pos := hcl.Pos{Line: 1, Column: 1}
			exprName := fmt.Sprintf("\"%s\"", tc.inputModuleSource)
			expr, _ :=  hclsyntax.ParseExpression([]byte(exprName), "", pos)

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


			if trcm.Source.String() != tc.wantModuleSource  {
				t.Fatalf("got %#v; want %#v", trcm.Source.String(), tc.wantModuleSource)
			}
		})
	}
}

func TestLoadMockFile_ValidBlocks(t *testing.T) {
	parser := NewParser(nil)

	mp, diags := parser.LoadMockFile("testdata/mock-source-valid/mocks/resources.tfmock.hcl")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(mp.MockResources) != 2 {
		t.Fatalf("expected 2 mock resources, got %d", len(mp.MockResources))
	}

	var managed, data *MockResource
	for _, r := range mp.MockResources {
		switch r.Mode {
		case addrs.ManagedResourceMode:
			managed = r
		case addrs.DataResourceMode:
			data = r
		}
	}

	if managed == nil || managed.Type != "aws_instance" {
		t.Fatalf("expected mock_resource aws_instance, got %v", managed)
	}
	if data == nil || data.Type != "aws_ami" {
		t.Fatalf("expected mock_data aws_ami, got %v", data)
	}
}

func TestLoadTestFile_MockSourceLoadsFromDir(t *testing.T) {
	parser := NewParser(nil)

	tf, diags := parser.LoadTestFile("testdata/mock-source-valid/main.tftest.hcl")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	mp, ok := tf.MockProviders["aws"]
	if !ok {
		t.Fatal("expected mock_provider aws")
	}

	if len(mp.MockResources) != 2 {
		t.Fatalf("expected 2 mock resources loaded from source dir, got %d", len(mp.MockResources))
	}
}

func TestLoadTestFile_MockSourceTofuPrecedence(t *testing.T) {
	parser := NewParser(nil)

	tf, diags := parser.LoadTestFile("testdata/mock-source-tofuprecedence/main.tftest.hcl")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	mp, ok := tf.MockProviders["aws"]
	if !ok {
		t.Fatal("expected mock_provider aws")
	}

	if len(mp.MockResources) != 1 {
		t.Fatalf("expected exactly 1 mock resource (tofumock wins), got %d", len(mp.MockResources))
	}

	r := mp.MockResources[0]
	if r.Type != "aws_instance" {
		t.Fatalf("expected aws_instance, got %s", r.Type)
	}

	val, ok := r.Defaults["ami"]
	if !ok {
		t.Fatal("expected ami default value")
	}
	if val.AsString() != "ami-from-tofumock" {
		t.Fatalf("expected ami-from-tofumock (tofumock wins), got %s", val.AsString())
	}
}

func TestLoadTestFile_MockSourceInlinePrecedence(t *testing.T) {
	parser := NewParser(nil)

	tf, diags := parser.LoadTestFile("testdata/mock-source-inline-precedence/main.tftest.hcl")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	mp, ok := tf.MockProviders["aws"]
	if !ok {
		t.Fatal("expected mock_provider aws")
	}

	// Should have 2 resources: aws_instance (inline) + aws_vpc (from file)
	if len(mp.MockResources) != 2 {
		t.Fatalf("expected 2 mock resources (1 inline + 1 from file), got %d", len(mp.MockResources))
	}

	resourcesByType := make(map[string]*MockResource)
	for _, r := range mp.MockResources {
		resourcesByType[r.Type] = r
	}

	instance, ok := resourcesByType["aws_instance"]
	if !ok {
		t.Fatal("expected aws_instance")
	}
	if instance.Defaults["ami"].AsString() != "ami-inline" {
		t.Fatalf("inline aws_instance should win, got %s", instance.Defaults["ami"].AsString())
	}

	if _, ok := resourcesByType["aws_vpc"]; !ok {
		t.Fatal("expected aws_vpc loaded from mock file")
	}
}

func TestLoadTestFile_MockSourceMissingDir(t *testing.T) {
	parser := NewParser(nil)

	_, diags := parser.LoadTestFile("testdata/mock-source-missing-dir/main.tftest.hcl")
	if !diags.HasErrors() {
		t.Fatal("expected error for missing source directory")
	}

	found := false
	for _, d := range diags {
		if d.Summary == "Mock source directory not found" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'Mock source directory not found' diagnostic, got: %s", diags.Error())
	}
}

func TestMergeMockResources_InlinePrecedence(t *testing.T) {
	inline := []*MockResource{
		{Mode: addrs.ManagedResourceMode, Type: "aws_instance", Defaults: map[string]cty.Value{}},
	}
	fromFiles := []*MockResource{
		{Mode: addrs.ManagedResourceMode, Type: "aws_instance", Defaults: map[string]cty.Value{}},
		{Mode: addrs.ManagedResourceMode, Type: "aws_vpc", Defaults: map[string]cty.Value{}},
	}

	result := mergeMockResources(inline, fromFiles)

	if len(result) != 2 {
		t.Fatalf("expected 2 resources (inline aws_instance + file aws_vpc), got %d", len(result))
	}
	if result[0].Type != "aws_instance" {
		t.Fatalf("first resource should be the inline aws_instance")
	}
	if result[1].Type != "aws_vpc" {
		t.Fatalf("second resource should be aws_vpc from file")
	}
}
