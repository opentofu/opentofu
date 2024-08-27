// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package refactoring

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/terramate-io/opentofulib/internal/addrs"
)

func TestGetEndpointsToRemove(t *testing.T) {
	tests := []struct {
		name        string
		fixtureName string
		want        []addrs.ConfigRemovable
		wantError   string
	}{
		{
			name:        "Valid cases",
			fixtureName: "testdata/remove-statement/valid-remove-statements",
			want: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("foo.basic_resource")).(addrs.ConfigRemovable),
				interface{}(addrs.Module{"basic_module"}).(addrs.ConfigRemovable),
				interface{}(mustConfigResourceAddr("module.child.foo.removed_resource_from_root_module")).(addrs.ConfigRemovable),
				interface{}(mustConfigResourceAddr("module.child.foo.removed_resource_from_child_module")).(addrs.ConfigRemovable),
				interface{}(addrs.Module{"child", "removed_module_from_child_module"}).(addrs.ConfigRemovable),
				interface{}(mustConfigResourceAddr("module.child.module.grandchild.foo.removed_resource_from_grandchild_module")).(addrs.ConfigRemovable),
				interface{}(addrs.Module{"child", "grandchild", "removed_module_from_grandchild_module"}).(addrs.ConfigRemovable),
			},
			wantError: ``,
		},
		{
			name:        "Error - resource block still exist",
			fixtureName: "testdata/remove-statement/not-valid-resource-block-still-exist",
			want: []addrs.ConfigRemovable{
				interface{}(mustConfigResourceAddr("foo.basic_resource")).(addrs.ConfigRemovable),
			},
			wantError: `Removed resource block still exists: This statement declares a removal of the resource foo.basic_resource, but this resource block still exists in the configuration. Please remove the resource block.`,
		},
		{
			name:        "Error - module block still exist",
			fixtureName: "testdata/remove-statement/not-valid-module-block-still-exist",
			want:        []addrs.ConfigRemovable{},
			wantError:   `Removed module block still exists: This statement declares a removal of the module module.child, but this module block still exists in the configuration. Please remove the module block.`,
		},
		{
			name:        "Error - nested resource block still exist",
			fixtureName: "testdata/remove-statement/not-valid-nested-resource-block-still-exist",
			want:        []addrs.ConfigRemovable{},
			wantError:   `Removed resource block still exists: This statement declares a removal of the resource module.child.foo.basic_resource, but this resource block still exists in the configuration. Please remove the resource block.`,
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCfg, _ := loadRefactoringFixture(t, tt.fixtureName)
			got, diags := GetEndpointsToRemove(rootCfg)

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
