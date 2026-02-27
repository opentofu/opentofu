// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestProvidersView(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v Providers)
		wantStdout string
		wantStderr string
	}{
		// Diagnostics
		"warning": {
			viewCall: func(v Providers) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
		},
		"error": {
			viewCall: func(v Providers) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
		},
		"multiple_diagnostics": {
			viewCall: func(v Providers) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
		},
		"module requirements": {
			viewCall: func(v Providers) {
				reqs := &configs.ModuleRequirements{
					Name: "root",
					Requirements: getproviders.Requirements{
						addrs.NewDefaultProvider("aws"): getproviders.MustParseVersionConstraints(">= 3.0.0"),
					},
					Children: map[string]*configs.ModuleRequirements{
						"vpc": {
							Name: "vpc",
							Requirements: getproviders.Requirements{
								addrs.NewDefaultProvider("random"): getproviders.MustParseVersionConstraints("~> 3.1.0"),
							},
							Children: map[string]*configs.ModuleRequirements{},
							Tests:    map[string]*configs.TestFileModuleRequirements{},
						},
					},
					Tests: map[string]*configs.TestFileModuleRequirements{
						"main.tftest.hcl": {
							Requirements: getproviders.Requirements{
								addrs.NewDefaultProvider("aws"): getproviders.MustParseVersionConstraints(">= 3.2.0"),
							},
							Runs: map[string]*configs.ModuleRequirements{
								"apply": {
									Requirements: map[addrs.Provider]getproviders.VersionConstraints{
										addrs.NewDefaultProvider("aws"): getproviders.MustParseVersionConstraints(">= 3.3.0"),
									},
								},
							},
						},
					},
				}
				v.ModuleRequirements(reqs)
			},
			wantStdout: `
Providers required by configuration:
.
├── provider[registry.opentofu.org/hashicorp/aws] >= 3.0.0
├── test.main
│   ├── provider[registry.opentofu.org/hashicorp/aws] >= 3.2.0
│   └── run.
│       └── provider[registry.opentofu.org/hashicorp/aws] >= 3.3.0
└── module.vpc
    └── provider[registry.opentofu.org/hashicorp/random] ~> 3.1.0

`,
		},
		"state requirements": {
			viewCall: func(v Providers) {
				stateReqs := getproviders.Requirements{
					addrs.NewDefaultProvider("aws"):    getproviders.MustParseVersionConstraints(">= 3.0.0"),
					addrs.NewDefaultProvider("random"): getproviders.MustParseVersionConstraints("~> 3.1.0"),
				}

				v.StateRequirements(stateReqs)
			},
			wantStdout: `Providers required by state:

    provider[registry.opentofu.org/hashicorp/aws]

    provider[registry.opentofu.org/hashicorp/random]

`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testProvidersHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
		})
	}
}

func testProvidersHuman(t *testing.T, call func(v Providers), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewProviders(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}
