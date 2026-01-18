// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
)

func TestValidateProviderConfigs_WithMetaArguments(t *testing.T) {
	declRange := hcl.Range{
		Filename: "main.tf",
		Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
		End:      hcl.Pos{Line: 5, Column: 2, Byte: 80},
	}

	tests := []struct {
		name                   string
		moduleCall             *ModuleCall
		childHasProviderConfig bool
		wantError              bool
	}{
		{
			name: "count",
			moduleCall: &ModuleCall{
				Name:      "child",
				Count:     &hclsyntax.LiteralValueExpr{},
				DeclRange: declRange,
			},
			childHasProviderConfig: true,
			wantError:              true,
		},
		{
			name: "for_each",
			moduleCall: &ModuleCall{
				Name:      "child",
				ForEach:   &hclsyntax.LiteralValueExpr{},
				DeclRange: declRange,
			},
			childHasProviderConfig: true,
			wantError:              true,
		},
		{
			name: "depends_on",
			moduleCall: &ModuleCall{
				Name:      "child",
				DependsOn: []hcl.Traversal{{}},
				DeclRange: declRange,
			},
			childHasProviderConfig: true,
			wantError:              true,
		},
		{
			name: "enabled",
			moduleCall: &ModuleCall{
				Name:      "child",
				Enabled:   &hclsyntax.LiteralValueExpr{},
				DeclRange: declRange,
			},
			childHasProviderConfig: true,
			wantError:              true,
		},
		{
			name: "no meta-arguments",
			moduleCall: &ModuleCall{
				Name:      "child",
				DeclRange: declRange,
			},
			childHasProviderConfig: true,
			wantError:              false,
		},
		{
			name: "count without child provider config",
			moduleCall: &ModuleCall{
				Name:      "child",
				Count:     &hclsyntax.LiteralValueExpr{},
				DeclRange: declRange,
			},
			childHasProviderConfig: false,
			wantError:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			childModule := &Module{
				ProviderConfigs: map[string]*Provider{},
			}

			if tt.childHasProviderConfig {
				childModule.ProviderConfigs["aws"] = &Provider{
					Name: "aws",
					Config: &hclsyntax.Body{
						Attributes: hclsyntax.Attributes{
							"region": &hclsyntax.Attribute{Name: "region"},
						},
					},
					DeclRange: hcl.Range{
						Filename: "child/main.tf",
						Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
						End:      hcl.Pos{Line: 3, Column: 2, Byte: 30},
					},
				}
			}

			childCfg := &Config{
				Path:       addrs.Module{"child"},
				Module:     childModule,
				SourceAddr: addrs.ModuleSourceLocal("./child"),
				Children:   map[string]*Config{},
			}

			parentModule := &Module{
				ModuleCalls: map[string]*ModuleCall{
					"child": tt.moduleCall,
				},
			}

			parentCfg := &Config{
				Path:     addrs.RootModule,
				Module:   parentModule,
				Children: map[string]*Config{"child": childCfg},
			}
			parentCfg.Root = parentCfg
			childCfg.Root = parentCfg
			childCfg.Parent = parentCfg

			diags := validateProviderConfigs(nil, parentCfg, nil)

			var foundError bool
			for _, diag := range diags {
				if diag.Severity == hcl.DiagError &&
					strings.Contains(diag.Summary, "Module is incompatible with count, for_each") {
					foundError = true
					if !strings.Contains(diag.Detail, "legacy module which contains its own local provider configurations") {
						t.Errorf("expected error detail to mention 'legacy module', got: %s", diag.Detail)
					}
					break
				}
			}

			if tt.wantError && !foundError {
				t.Errorf("expected error, but got none")
			}

			if !tt.wantError && foundError {
				t.Errorf("did not expect error, but got %s", diags)
			}
		})
	}
}
