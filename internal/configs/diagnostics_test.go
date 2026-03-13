// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	hcVersion "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/zclconf/go-cty/cty"
)

func TestRequiredVersion(t *testing.T) {
	tests := []struct {
		Name             string
		CurrentVersion   string
		RequiredVersions string
		Err              bool
	}{
		{
			"doesn't match",
			"0.1.0",
			"> 0.6.0",
			true,
		},
		{
			"matches",
			"0.7.0",
			"> 0.6.0",
			false,
		},
		{
			"prerelease doesn't match with inequality",
			"0.8.0",
			"> 0.7.0-beta",
			true,
		},
		{
			"prerelease doesn't match with equality",
			"0.7.0",
			"0.7.0-beta",
			true,
		},
	}

	for _, test := range tests {

		t.Run(test.Name, func(t *testing.T) {
			fakeSourceRange := hcl.Range{
				// This must have a .tofu suffix for the required_version
				// subtest to work, because we only support that legacy form
				// in OpenTofu-specific files.
				Filename: "versions.tofu",
				Start:    hcl.InitialPos,
				End:      hcl.InitialPos,
			}
			currentVersion := hcVersion.Must(hcVersion.NewVersion(test.CurrentVersion))

			t.Logf("matching constraint %q against current version %q", test.RequiredVersions, test.CurrentVersion)

			t.Run("language block", func(t *testing.T) {
				body := hcltest.MockBody(&hcl.BodyContent{
					Blocks: []*hcl.Block{
						{
							Type: "language",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Blocks: []*hcl.Block{
									{
										Type: "compatible_with",
										Body: hcltest.MockBody(&hcl.BodyContent{
											Attributes: hcl.Attributes{
												"opentofu": {
													Name:      "opentofu",
													Expr:      hcl.StaticExpr(cty.StringVal(test.RequiredVersions), fakeSourceRange),
													Range:     fakeSourceRange,
													NameRange: fakeSourceRange,
												},
											},
										}),
										DefRange:  fakeSourceRange,
										TypeRange: fakeSourceRange,
									},
								},
							}),
							DefRange:  fakeSourceRange,
							TypeRange: fakeSourceRange,
						},
					},
				})
				diags := checkVersionRequirements(body, currentVersion)
				if test.Err && !diags.HasErrors() {
					t.Error("unexpected success; want error")
				} else if !test.Err && diags.HasErrors() {
					t.Errorf("unexpected error: %s", diags.Error())
				}
			})

			t.Run("legacy required_version", func(t *testing.T) {
				body := hcltest.MockBody(&hcl.BodyContent{
					Blocks: []*hcl.Block{
						{
							Type: "terraform",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"required_version": {
										Name:      "required_version",
										Expr:      hcl.StaticExpr(cty.StringVal(test.RequiredVersions), fakeSourceRange),
										Range:     fakeSourceRange,
										NameRange: fakeSourceRange,
									},
								},
							}),
							DefRange:  fakeSourceRange,
							TypeRange: fakeSourceRange,
						},
					},
				})
				diags := checkVersionRequirements(body, currentVersion)
				if test.Err && !diags.HasErrors() {
					t.Error("unexpected success; want error")
				} else if !test.Err && diags.HasErrors() {
					t.Errorf("unexpected error: %s", diags.Error())
				}
			})
		})
	}
}
