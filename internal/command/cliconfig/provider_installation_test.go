// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getproviders"
)

func TestLoadConfig_providerInstallation(t *testing.T) {
	for _, configFile := range []string{"provider-installation", "provider-installation.json"} {
		t.Run(configFile, func(t *testing.T) {
			got, diags := loadConfigFile(filepath.Join(fixtureDir, configFile))
			if diags.HasErrors() {
				t.Errorf("unexpected diagnostics: %s", diags.Err().Error())
			}

			want := &Config{
				ProviderInstallation: []*ProviderInstallation{
					{
						Methods: []*ProviderInstallationMethod{
							{
								Location: ProviderInstallationFilesystemMirror("/tmp/example1"),
								Include:  []string{"example.com/*/*"},
							},
							{
								Location: ProviderInstallationNetworkMirror("https://tf-Mirror.example.com/"),
								Include:  []string{"registry.opentofu.org/*/*"},
								Exclude:  []string{"registry.OpenTofu.org/foobar/*"},
							},
							{
								Location: ProviderInstallationFilesystemMirror("/tmp/example2"),
							},
							{
								Location: ProviderInstallationDirect,
								Exclude:  []string{"example.com/*/*"},
							},
						},

						DevOverrides: map[addrs.Provider]getproviders.PackageLocalDir{
							addrs.MustParseProviderSourceString("hashicorp/boop"):  getproviders.PackageLocalDir(filepath.FromSlash("/tmp/boop")),
							addrs.MustParseProviderSourceString("hashicorp/blorp"): getproviders.PackageLocalDir(filepath.FromSlash("/tmp/blorp")),
						},
					},
				},
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("wrong result\n%s", diff)
			}
		})
	}
}

func TestLoadConfig_providerInstallationOCIMirror(t *testing.T) {
	config, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-mirror"))
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.Err().Error())
	}

	if got, want := len(config.ProviderInstallation), 1; got != want {
		t.Fatalf("wrong number of provider_installation blocks %d; want %d", got, want)
	}
	methods := config.ProviderInstallation[0].Methods
	if got, want := len(methods), 4; got != want {
		t.Fatalf("wrong number of provider installation methods %d; want %d", got, want)
	}

	// We expect all of the configured methods to be oci_mirror and
	// to produce the same repository address from their templates.
	providerAddr := addrs.Provider{
		Hostname:  svchost.Hostname("example.net"),
		Namespace: "foo",
		Type:      "bar",
	}
	wantRepositoryAddr := getproviders.OCIRepository{
		Hostname: "example.com",
		Name:     "example.net__foo__bar",
	}

	for _, method := range methods {
		location, ok := method.Location.(ProviderInstallationOCIMirror)
		if !ok {
			t.Fatalf("wrong installation method type %T; want %T", method.Location, location)
		}

		gotAddr, diags := location.RepositoryAddrFunc(providerAddr)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.Err().Error())
		}
		if gotAddr != wantRepositoryAddr {
			t.Fatalf("wrong result from RepositoryAddrFunc\ngot:  %#v\nwant: %#v", gotAddr, wantRepositoryAddr)
		}
	}
}

func TestLoadConfig_providerInstallationErrors(t *testing.T) {
	_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-errors"))
	want := `7 problems:

- Invalid provider_installation method block: Unknown provider installation method "not_a_thing" at 2:3.
- Invalid provider_installation method block: Invalid filesystem_mirror block at 1:1: "path" argument is required.
- Invalid provider_installation method block: Invalid network_mirror block at 1:1: "url" argument is required.
- Invalid provider_installation method block: The items inside the provider_installation block at 1:1 must all be blocks.
- Invalid provider_installation method block: The blocks inside the provider_installation block at 1:1 may not have any labels.
- Invalid provider_installation block: The provider_installation block at 9:1 must not have any labels.
- Invalid provider_installation block: The provider_installation block at 11:1 must not be introduced with an equals sign.`

	// The above error messages include only line/column location information
	// and not file location information because HCL 1 does not store
	// information about the filename a location belongs to. (There is a field
	// for it in token.Pos but it's always an empty string in practice.)

	if got := diags.Err().Error(); got != want {
		t.Errorf("wrong diagnostics\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestLoadConfig_providerInstallationErrorsOCIMirror(t *testing.T) {
	// We have this particular installation method separated into its own test because
	// it has a variety of different errors of its own.
	_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-mirror-errors"))
	want := `9 problems:

- Invalid provider_installation method block: Invalid oci_mirror block at 2:14: "repository_template" argument is required.
- Invalid expression: Expected the start of an expression, but found an invalid expression token.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 6:14: template must refer to the "hostname" symbol unless the "include" argument selects exactly one registry hostname.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 6:14: template must refer to the "namespace" symbol unless the "include" argument selects exactly one provider namespace.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 6:14: template must refer to the "type" symbol unless the "include" argument selects exactly one provider.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 9:14: template must refer to the "namespace" symbol unless the "include" argument selects exactly one provider namespace.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 9:14: template must refer to the "type" symbol unless the "include" argument selects exactly one provider.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 17:14: template must refer to the "type" symbol unless the "include" argument selects exactly one provider.
- Invalid oci_mirror repository template: Invalid oci_mirror block at 25:14: template must refer to the "hostname" symbol unless the "include" argument selects exactly one registry hostname.`

	// The above error messages include only line/column location information
	// and not file location information because HCL 1 does not store
	// information about the filename a location belongs to. (There is a field
	// for it in token.Pos but it's always an empty string in practice.)

	if diff := cmp.Diff(want, diags.Err().Error()); diff != "" {
		t.Error("wrong diagnostics\n" + diff)
	}

	// FIXME: This test doesn't cover cases where the repository_template is
	// syntactically valid but fails evaluation when given a specific
	// provider address. We probably need another test function for that,
	// since this one is set up to completely fail loading its fixture.
}

func TestLoadConfig_providerInstallationDirectWithOCIExperiment(t *testing.T) {
	// This test covers the temporary opt-in for the OCI-registry-as-provider-registry experiment.
	// If this experiment succeeds and we decide to implement it for real then this
	// test can be completely removed because the OCI registry functionality will be
	// incorporated into the main ProviderInstallationDirect selection instead.

	got, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-direct-with-oci"))
	if diags.HasErrors() {
		t.Errorf("unexpected diagnostics: %s", diags.Err().Error())
	}

	want := &Config{
		ProviderInstallation: []*ProviderInstallation{
			{
				Methods: []*ProviderInstallationMethod{
					{
						Location: ProviderInstallationDirectWithOCIExperiment,
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}
}
