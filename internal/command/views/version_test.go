// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestVersionViews(t *testing.T) {
	tests := map[string]struct {
		viewType   arguments.ViewType
		viewCall   func(v Version)
		wantStdout string
		wantStderr string
	}{
		"human printVersion with fips disabled": {
			viewType: arguments.ViewHuman,
			viewCall: func(v Version) {
				v.PrintVersion("0.1.0", "dev", "darwin_arm64", false, map[string]string{
					"registry.opentofu.org/test/test": "0.2.0",
				})
			},
			wantStdout: `OpenTofu v0.1.0-dev
on darwin_arm64
+ provider registry.opentofu.org/test/test v0.2.0
`,
			wantStderr: "",
		},
		"human printVersion with fips enabled": {
			viewType: arguments.ViewHuman,
			viewCall: func(v Version) {
				v.PrintVersion("0.1.0", "dev", "darwin_arm64", true, map[string]string{
					"registry.opentofu.org/test/test": "0.2.0",
				})
			},
			wantStdout: `OpenTofu v0.1.0-dev
on darwin_arm64
running in FIPS 140-3 mode (not yet supported)
+ provider registry.opentofu.org/test/test v0.2.0
`,
			wantStderr: "",
		},
		"human printVersion with unversioned provider": {
			viewType: arguments.ViewHuman,
			viewCall: func(v Version) {
				v.PrintVersion("0.1.0", "dev", "darwin_arm64", false, map[string]string{
					"registry.opentofu.org/test/test": "0.0.0",
				})
			},
			wantStdout: `OpenTofu v0.1.0-dev
on darwin_arm64
+ provider registry.opentofu.org/test/test (unversioned)
`,
			wantStderr: "",
		},
		"json printVersion with fips disabled": {
			viewType: arguments.ViewJSON,
			viewCall: func(v Version) {
				v.PrintVersion("0.1.0", "dev", "darwin_arm64", false, map[string]string{
					"registry.opentofu.org/test/test": "0.2.0",
				})
			},
			wantStdout: `{
  "terraform_version": "0.1.0-dev",
  "platform": "darwin_arm64",
  "provider_selections": {
    "registry.opentofu.org/test/test": "0.2.0"
  }
}
`,
			wantStderr: "",
		},
		"json printVersion with fips enabled": {
			viewType: arguments.ViewJSON,
			viewCall: func(v Version) {
				v.PrintVersion("0.1.0", "dev", "darwin_arm64", true, map[string]string{
					"registry.opentofu.org/test/test": "0.2.0",
				})
			},
			wantStdout: `{
  "terraform_version": "0.1.0-dev",
  "platform": "darwin_arm64",
  "fips140": true,
  "provider_selections": {
    "registry.opentofu.org/test/test": "0.2.0"
  }
}
`,
			wantStderr: "",
		},
		"json printVersion with unversioned provider": {
			viewType: arguments.ViewJSON,
			viewCall: func(v Version) {
				v.PrintVersion("0.1.0", "dev", "darwin_arm64", false, map[string]string{
					"registry.opentofu.org/test/test": "0.0.0",
				})
			},
			wantStdout: `{
  "terraform_version": "0.1.0-dev",
  "platform": "darwin_arm64",
  "provider_selections": {
    "registry.opentofu.org/test/test": "0.0.0"
  }
}
`,
			wantStderr: "",
		},
		// Diagnostics
		"warning": {
			viewType: arguments.ViewHuman,
			viewCall: func(v Version) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
		},
		"error": {
			viewType: arguments.ViewHuman,
			viewCall: func(v Version) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
		},
		"multiple diagnostics": {
			viewType: arguments.ViewHuman,
			viewCall: func(v Version) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
		},
		"multiple diagnostics in json": {
			viewType: arguments.ViewJSON,
			viewCall: func(v Version) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
			},
			// The JSON view type does not apply to the diagnostics
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testVersionHuman(t, tc.viewType, tc.viewCall, tc.wantStdout, tc.wantStderr)
		})
	}
}

func testVersionHuman(t *testing.T, viewType arguments.ViewType, call func(v Version), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewVersion(arguments.ViewOptions{ViewType: viewType}, view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}
