// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/svchost"

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

func TestLoadConfig_providerInstallationOCIMirror(t *testing.T) {
	for _, configFile := range []string{"provider-installation-oci", "provider-installation-oci.json"} {
		t.Run(configFile, func(t *testing.T) {
			gotConfig, diags := loadConfigFile(filepath.Join(fixtureDir, configFile))
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.Err().Error())
			}

			gotInstBlocks := gotConfig.ProviderInstallation
			if got, want := len(gotInstBlocks), 1; got != want {
				t.Fatalf("wrong number of provider_installation blocks %d; want %d", got, want)
			}
			gotMethods := gotInstBlocks[0].Methods
			if got, want := len(gotMethods), 4; got != want {
				t.Fatalf("wrong number of provider installation methods %d; want %d", got, want)
			}

			providerAddr := addrs.Provider{
				Hostname:  svchost.Hostname("registry.opentofu.org"),
				Namespace: "opentofu",
				Type:      "foo",
			}
			t.Run("all segments in template", func(t *testing.T) {
				method := gotMethods[0]
				loc, ok := method.Location.(ProviderInstallationOCIMirror)
				if !ok {
					t.Fatalf("wrong location type %T; want %T", method.Location, loc)
				}
				gotRegistry, gotRepository, err := loc.RepositoryMapping(providerAddr)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got, want := gotRegistry, "example.com"; got != want {
					t.Errorf("wrong registry domain %q; want %q", got, want)
				}
				if got, want := gotRepository, "registry.opentofu.org/opentofu/foo"; got != want {
					t.Errorf("wrong repository name %q; want %q", got, want)
				}
			})
			t.Run("hostname chosen by include", func(t *testing.T) {
				method := gotMethods[1]
				loc, ok := method.Location.(ProviderInstallationOCIMirror)
				if !ok {
					t.Fatalf("wrong location type %T; want %T", method.Location, loc)
				}
				gotRegistry, gotRepository, err := loc.RepositoryMapping(providerAddr)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got, want := gotRegistry, "example.net"; got != want {
					t.Errorf("wrong registry domain %q; want %q", got, want)
				}
				if got, want := gotRepository, "opentofu-registry/opentofu/foo"; got != want {
					t.Errorf("wrong repository name %q; want %q", got, want)
				}
			})
			t.Run("hostname and namespace chosen by include", func(t *testing.T) {
				method := gotMethods[2]
				loc, ok := method.Location.(ProviderInstallationOCIMirror)
				if !ok {
					t.Fatalf("wrong location type %T; want %T", method.Location, loc)
				}
				gotRegistry, gotRepository, err := loc.RepositoryMapping(providerAddr)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got, want := gotRegistry, "example.net"; got != want {
					t.Errorf("wrong registry domain %q; want %q", got, want)
				}
				if got, want := gotRepository, "opentofu-registry/opentofu-namespace/foo"; got != want {
					t.Errorf("wrong repository name %q; want %q", got, want)
				}
			})
			t.Run("all components chosen by include", func(t *testing.T) {
				method := gotMethods[3]
				loc, ok := method.Location.(ProviderInstallationOCIMirror)
				if !ok {
					t.Fatalf("wrong location type %T; want %T", method.Location, loc)
				}
				gotRegistry, gotRepository, err := loc.RepositoryMapping(providerAddr)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got, want := gotRegistry, "example.net"; got != want {
					t.Errorf("wrong registry domain %q; want %q", got, want)
				}
				if got, want := gotRepository, "opentofu-registry/opentofu-namespace/foo-type"; got != want {
					t.Errorf("wrong repository name %q; want %q", got, want)
				}
			})
		})
	}
}

func TestLoadConfig_providerInstallationOCIMirrorErrors(t *testing.T) {
	t.Run("missing hostname reference", func(t *testing.T) {
		_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-missinghostname"))
		if !diags.HasErrors() {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := diags.Err().Error(), `template must refer to the "hostname" symbol`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
	t.Run("missing namespace reference", func(t *testing.T) {
		_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-missingnamespace"))
		if !diags.HasErrors() {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := diags.Err().Error(), `template must refer to the "namespace" symbol`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
	t.Run("missing type reference", func(t *testing.T) {
		_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-missingtype"))
		if !diags.HasErrors() {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := diags.Err().Error(), `template must refer to the "type" symbol`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
	t.Run("type error in template", func(t *testing.T) {
		_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-typeerror"))
		if !diags.HasErrors() {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := diags.Err().Error(), `This value does not have any indices`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
	t.Run("value error in template", func(t *testing.T) {
		_, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-valueerror"))
		if !diags.HasErrors() {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := diags.Err().Error(), `a number is required`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
	t.Run("dynamic error in template", func(t *testing.T) {
		cfg, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-dynerror"))
		if diags.HasErrors() {
			t.Fatalf("unexpected error for configuration load (error should be only during template evaluation)")
		}
		method, ok := cfg.ProviderInstallation[0].Methods[0].Location.(ProviderInstallationOCIMirror)
		if !ok {
			t.Fatalf("wrong installation method location type")
		}
		_, _, err := method.RepositoryMapping(addrs.Provider{
			Hostname:  svchost.Hostname("example.net"), // This template fails for anything other than "example.com"
			Namespace: "whatever",
			Type:      "anything",
		})
		if err == nil {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := err.Error(), `The given key does not identify an element in this collection value`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
	t.Run("unmappable characters in provider source address", func(t *testing.T) {
		// This deals with a particularly-annoying case: OpenTofu provider source addresses
		// support a wide range of unicode characters with the intent that folks can name
		// their private providers using the alphabet of their native language, but OCI Distribution
		// only allows ASCII characters in repository names, so for now the OCI mirror
		// installation method can only work with providers whose namespace and type
		// are ASCII-only. Non-ASCII characters are pretty rare in practice for public
		// providers, but we can't tell whether they are more common in private provider
		// registries. For now we treat this as an error but we might try to find a better
		// answer for this in a future release if it proves to be a problem in practice.
		cfg, diags := loadConfigFile(filepath.Join(fixtureDir, "provider-installation-oci-passthru"))
		if diags.HasErrors() {
			t.Fatalf("unexpected error for configuration load (error should be only during template evaluation)")
		}
		method, ok := cfg.ProviderInstallation[0].Methods[0].Location.(ProviderInstallationOCIMirror)
		if !ok {
			t.Fatalf("wrong installation method location type")
		}
		_, _, err := method.RepositoryMapping(addrs.Provider{
			Hostname:  svchost.Hostname("example.com"),
			Namespace: "ほげ",
			Type:      "ふが",
		})
		if err == nil {
			t.Fatalf("unexpected success; want error")
		}
		if got, want := err.Error(), `invalid repository "example.com/ほげ/ふが"`; !strings.Contains(got, want) {
			t.Errorf("missing expected error\ngot: %s\nwant substring: %s", got, want)
		}
	})
}
