// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package refactoring

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestGetEndpointsToRemove(t *testing.T) {
	tests := []struct {
		name        string
		fixtureName string
		want        []*RemoveStatement
		wantError   string
	}{
		{
			name:        "Valid cases",
			fixtureName: "testdata/remove-statement/valid-remove-statements",
			want: []*RemoveStatement{
				{
					From: mustConfigResourceAddr("foo.basic_resource"),
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/main.tf",
						Start:    tfdiags.SourcePos{Line: 1, Column: 1},
						End:      tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					},
				},
				{
					From: addrs.Module{"basic_module"},
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/main.tf",
						Start:    tfdiags.SourcePos{Line: 5, Column: 1, Byte: 41},
						End:      tfdiags.SourcePos{Line: 5, Column: 8, Byte: 48},
					},
				},
				{
					From: mustConfigResourceAddr("module.child.foo.removed_resource_from_root_module"),
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/main.tf",
						Start:    tfdiags.SourcePos{Line: 9, Column: 1, Byte: 83},
						End:      tfdiags.SourcePos{Line: 9, Column: 8, Byte: 90},
					},
				},
				{
					From: mustConfigResourceAddr("module.child.foo.removed_resource_from_child_module"),
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/child/main.tf",
						Start:    tfdiags.SourcePos{Line: 1, Column: 1},
						End:      tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					},
				},
				{
					From: addrs.Module{"child", "removed_module_from_child_module"},
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/child/main.tf",
						Start:    tfdiags.SourcePos{Line: 9, Column: 1, Byte: 112},
						End:      tfdiags.SourcePos{Line: 9, Column: 8, Byte: 119},
					},
				},
				{
					From: mustConfigResourceAddr("module.child.module.grandchild.foo.removed_resource_from_grandchild_module"),
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/child/grandchild/main.tf",
						Start:    tfdiags.SourcePos{Line: 1, Column: 1},
						End:      tfdiags.SourcePos{Line: 1, Column: 8, Byte: 7},
					},
				},
				{
					From: addrs.Module{"child", "grandchild", "removed_module_from_grandchild_module"},
					DeclRange: tfdiags.SourceRange{
						Filename: "testdata/remove-statement/valid-remove-statements/child/grandchild/main.tf",
						Start:    tfdiags.SourcePos{Line: 5, Column: 1, Byte: 66},
						End:      tfdiags.SourcePos{Line: 5, Column: 8, Byte: 73},
					},
				},
			},
			wantError: ``,
		},
		{
			name:        "Error - resource block still exist",
			fixtureName: "testdata/remove-statement/not-valid-resource-block-still-exist",
			want: []*RemoveStatement{
				{From: mustConfigResourceAddr("foo.basic_resource")},
			},
			wantError: `Removed resource block still exists: This statement declares a removal of the resource foo.basic_resource, but this resource block still exists in the configuration. Please remove the resource block.`,
		},
		{
			name:        "Error - module block still exist",
			fixtureName: "testdata/remove-statement/not-valid-module-block-still-exist",
			want:        []*RemoveStatement{},
			wantError:   `Removed module block still exists: This statement declares a removal of the module module.child, but this module block still exists in the configuration. Please remove the module block.`,
		},
		{
			name:        "Error - nested resource block still exist",
			fixtureName: "testdata/remove-statement/not-valid-nested-resource-block-still-exist",
			want:        []*RemoveStatement{},
			wantError:   `Removed resource block still exists: This statement declares a removal of the resource module.child.foo.basic_resource, but this resource block still exists in the configuration. Please remove the resource block.`,
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCfg, _ := loadRefactoringFixture(t, tt.fixtureName)
			got, diags := FindRemoveStatements(rootCfg)

			if tt.wantError != "" {
				if !diags.HasErrors() {
					t.Fatalf("missing expected error\ngot:  <no error>\nwant: %s", tt.wantError)
				}
				errStr := diags.Err().Error()
				if errStr != tt.wantError {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", errStr, tt.wantError)
				}
			} else {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("wrong result\n%s", diff)
				}
			}
		})
	}
}

func mustConfigResourceAddr(s string) addrs.ConfigResource {
	addr, diags := addrs.ParseAbsResourceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr.Config()
}
