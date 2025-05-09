// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsfile

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getproviders"
)

func TestLocksEqual(t *testing.T) {
	boopProvider := addrs.NewDefaultProvider("boop")
	v2 := getproviders.MustParseVersion("2.0.0")
	v2LocalBuild := getproviders.MustParseVersion("2.0.0+awesomecorp.1")
	v2GtConstraints := getproviders.MustParseVersionConstraints(">= 2.0.0")
	v2EqConstraints := getproviders.MustParseVersionConstraints("2.0.0")
	hash1 := getproviders.HashScheme("test").New("1")
	hash2 := getproviders.HashScheme("test").New("2")
	hash3 := getproviders.HashScheme("test").New("3")

	equalBothWays := func(t *testing.T, a, b *Locks) {
		t.Helper()
		if !a.Equal(b) {
			t.Errorf("a should be equal to b")
		}
		if !b.Equal(a) {
			t.Errorf("b should be equal to a")
		}
	}
	nonEqualBothWays := func(t *testing.T, a, b *Locks) {
		t.Helper()
		if a.Equal(b) {
			t.Errorf("a should be equal to b")
		}
		if b.Equal(a) {
			t.Errorf("b should be equal to a")
		}
	}

	t.Run("both empty", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		equalBothWays(t, a, b)
	})
	t.Run("an extra provider lock", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		b.SetProvider(boopProvider, v2, v2GtConstraints, nil)
		nonEqualBothWays(t, a, b)
	})
	t.Run("both have boop provider with same version", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		// Note: the constraints are not part of the definition of "Equal", so they can differ
		a.SetProvider(boopProvider, v2, v2GtConstraints, nil)
		b.SetProvider(boopProvider, v2, v2EqConstraints, nil)
		equalBothWays(t, a, b)
	})
	t.Run("both have boop provider with different versions", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		a.SetProvider(boopProvider, v2, v2EqConstraints, nil)
		b.SetProvider(boopProvider, v2LocalBuild, v2EqConstraints, nil)
		nonEqualBothWays(t, a, b)
	})
	t.Run("both have boop provider with same version and same hashes", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		hashes := []getproviders.Hash{hash1, hash2, hash3}
		a.SetProvider(boopProvider, v2, v2EqConstraints, hashes)
		b.SetProvider(boopProvider, v2, v2EqConstraints, hashes)
		equalBothWays(t, a, b)
	})
	t.Run("both have boop provider with same version but different hashes", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		hashesA := []getproviders.Hash{hash1, hash2}
		hashesB := []getproviders.Hash{hash1, hash3}
		a.SetProvider(boopProvider, v2, v2EqConstraints, hashesA)
		b.SetProvider(boopProvider, v2, v2EqConstraints, hashesB)
		nonEqualBothWays(t, a, b)
	})
}

func TestLocksEqualProviderAddress(t *testing.T) {
	boopProvider := addrs.NewDefaultProvider("boop")
	v2 := getproviders.MustParseVersion("2.0.0")
	v2LocalBuild := getproviders.MustParseVersion("2.0.0+awesomecorp.1")
	v2GtConstraints := getproviders.MustParseVersionConstraints(">= 2.0.0")
	v2EqConstraints := getproviders.MustParseVersionConstraints("2.0.0")
	hash1 := getproviders.HashScheme("test").New("1")
	hash2 := getproviders.HashScheme("test").New("2")
	hash3 := getproviders.HashScheme("test").New("3")

	equalProviderAddressBothWays := func(t *testing.T, a, b *Locks) {
		t.Helper()
		if !a.EqualProviderAddress(b) {
			t.Errorf("a should be equal to b")
		}
		if !b.EqualProviderAddress(a) {
			t.Errorf("b should be equal to a")
		}
	}
	nonEqualProviderAddressBothWays := func(t *testing.T, a, b *Locks) {
		t.Helper()
		if a.EqualProviderAddress(b) {
			t.Errorf("a should be equal to b")
		}
		if b.EqualProviderAddress(a) {
			t.Errorf("b should be equal to a")
		}
	}

	t.Run("both empty", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		equalProviderAddressBothWays(t, a, b)
	})
	t.Run("an extra provider lock", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		b.SetProvider(boopProvider, v2, v2GtConstraints, nil)
		nonEqualProviderAddressBothWays(t, a, b)
	})
	t.Run("both have boop provider with different versions", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		a.SetProvider(boopProvider, v2, v2EqConstraints, nil)
		b.SetProvider(boopProvider, v2LocalBuild, v2EqConstraints, nil)
		equalProviderAddressBothWays(t, a, b)
	})
	t.Run("both have boop provider with same version but different hashes", func(t *testing.T) {
		a := NewLocks()
		b := NewLocks()
		hashesA := []getproviders.Hash{hash1, hash2}
		hashesB := []getproviders.Hash{hash1, hash3}
		a.SetProvider(boopProvider, v2, v2EqConstraints, hashesA)
		b.SetProvider(boopProvider, v2, v2EqConstraints, hashesB)
		equalProviderAddressBothWays(t, a, b)
	})
}

func TestLocksProviderSetRemove(t *testing.T) {
	beepProvider := addrs.NewDefaultProvider("beep")
	boopProvider := addrs.NewDefaultProvider("boop")
	v2 := getproviders.MustParseVersion("2.0.0")
	v2EqConstraints := getproviders.MustParseVersionConstraints("2.0.0")
	v2GtConstraints := getproviders.MustParseVersionConstraints(">= 2.0.0")
	hash := getproviders.HashScheme("test").New("1")

	locks := NewLocks()
	if got, want := len(locks.AllProviders()), 0; got != want {
		t.Fatalf("fresh locks object already has providers")
	}

	locks.SetProvider(boopProvider, v2, v2EqConstraints, []getproviders.Hash{hash})
	{
		got := locks.AllProviders()
		want := map[addrs.Provider]*ProviderLock{
			boopProvider: {
				addr:               boopProvider,
				version:            v2,
				versionConstraints: v2EqConstraints,
				hashes:             []getproviders.Hash{hash},
			},
		}
		if diff := cmp.Diff(want, got, ProviderLockComparer); diff != "" {
			t.Fatalf("wrong providers after SetProvider boop\n%s", diff)
		}
	}

	locks.SetProvider(beepProvider, v2, v2GtConstraints, []getproviders.Hash{hash})
	{
		got := locks.AllProviders()
		want := map[addrs.Provider]*ProviderLock{
			boopProvider: {
				addr:               boopProvider,
				version:            v2,
				versionConstraints: v2EqConstraints,
				hashes:             []getproviders.Hash{hash},
			},
			beepProvider: {
				addr:               beepProvider,
				version:            v2,
				versionConstraints: v2GtConstraints,
				hashes:             []getproviders.Hash{hash},
			},
		}
		if diff := cmp.Diff(want, got, ProviderLockComparer); diff != "" {
			t.Fatalf("wrong providers after SetProvider beep\n%s", diff)
		}
	}

	locks.RemoveProvider(boopProvider)
	{
		got := locks.AllProviders()
		want := map[addrs.Provider]*ProviderLock{
			beepProvider: {
				addr:               beepProvider,
				version:            v2,
				versionConstraints: v2GtConstraints,
				hashes:             []getproviders.Hash{hash},
			},
		}
		if diff := cmp.Diff(want, got, ProviderLockComparer); diff != "" {
			t.Fatalf("wrong providers after RemoveProvider boop\n%s", diff)
		}
	}

	locks.RemoveProvider(beepProvider)
	{
		got := locks.AllProviders()
		want := map[addrs.Provider]*ProviderLock{}
		if diff := cmp.Diff(want, got, ProviderLockComparer); diff != "" {
			t.Fatalf("wrong providers after RemoveProvider beep\n%s", diff)
		}
	}
}

func TestProviderLockContainsAll(t *testing.T) {
	provider := addrs.NewDefaultProvider("provider")
	v2 := getproviders.MustParseVersion("2.0.0")
	v2EqConstraints := getproviders.MustParseVersionConstraints("2.0.0")

	t.Run("non-symmetric", func(t *testing.T) {
		target := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"K43RHM2klOoywtyW",
			"swJPXfuCNhJsTM5c",
		})

		original := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"1ZAChGWUMWn4zmIk",
			"K43RHM2klOoywtyW",
			"HWjRvIuWZ1LVatnc",
			"swJPXfuCNhJsTM5c",
			"KwhJK4p/U2dqbKhI",
		})

		if !original.ContainsAll(target) {
			t.Errorf("original should contain all hashes in target")
		}
		if target.ContainsAll(original) {
			t.Errorf("target should not contain all hashes in original")
		}
	})

	t.Run("symmetric", func(t *testing.T) {
		target := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"K43RHM2klOoywtyW",
			"swJPXfuCNhJsTM5c",
		})

		original := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"K43RHM2klOoywtyW",
			"swJPXfuCNhJsTM5c",
		})

		if !original.ContainsAll(target) {
			t.Errorf("original should contain all hashes in target")
		}
		if !target.ContainsAll(original) {
			t.Errorf("target should not contain all hashes in original")
		}
	})

	t.Run("edge case - null", func(t *testing.T) {
		original := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"K43RHM2klOoywtyW",
			"swJPXfuCNhJsTM5c",
		})

		if !original.ContainsAll(nil) {
			t.Fatalf("original should report true on nil")
		}
	})

	t.Run("edge case - empty", func(t *testing.T) {
		original := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"K43RHM2klOoywtyW",
			"swJPXfuCNhJsTM5c",
		})

		target := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{})

		if !original.ContainsAll(target) {
			t.Fatalf("original should report true on empty")
		}
	})

	t.Run("edge case - original empty", func(t *testing.T) {
		original := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{})

		target := NewProviderLock(provider, v2, v2EqConstraints, []getproviders.Hash{
			"9r3i9a9QmASqMnQM",
			"K43RHM2klOoywtyW",
			"swJPXfuCNhJsTM5c",
		})

		if original.ContainsAll(target) {
			t.Fatalf("original should report false when empty")
		}
	})
}

func TestLocksUpgradeFromPredecessorProject(t *testing.T) {
	locks, diags := LoadLocksFromBytes([]byte(`
		provider "registry.terraform.io/hashicorp/a" {
			version = "1.0.0"
			constraints = ">= 1.0.0"
			hashes = [
				"h1:jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9MhT0=",
			]
		}

		provider "registry.terraform.io/anything-else/b" {
			version = "2.0.0"
			hashes = [
				"h1:jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9MhT0=",
			]
		}

		provider "registry.terraform.io/hashicorp/c" {
			version = "3.0.0"
			hashes = [
				"h1:jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9MhT0=",
			]
		}
		provider "registry.opentofu.org/hashicorp/c" {
			version = "4.0.0"
			hashes = [
				"h1:jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9T0Mh=",
			]
		}
	`), "")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err().Error())
	}

	gotChanges := locks.UpgradeFromPredecessorProject()
	wantChanges := map[addrs.Provider]addrs.Provider{
		addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/a"): addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/a"),
	}
	if diff := cmp.Diff(wantChanges, gotChanges); diff != "" {
		t.Error("wrong reported changes\n" + diff)
	}

	gotLocks := locks.AllProviders()
	wantLocks := map[addrs.Provider]*ProviderLock{
		// This one is still included because we want to let the provider
		// installer be the one to decide to remove it, once it's convinced
		// itself that this provider is definitely no longer needed...
		addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/a"): {
			addr:               addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/a"),
			version:            getproviders.MustParseVersion("1.0.0"),
			versionConstraints: getproviders.MustParseVersionConstraints(">= 1.0.0"),
			hashes: []getproviders.Hash{
				getproviders.HashScheme1.New("jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9MhT0="),
			},
		},

		// ...but we now have this new one that describes the OpenTofu equivalent
		// of it, with the same version but not yet any hashes. The hashes
		// must be determined by a subsequent installation step.
		addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/a"): {
			addr:               addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/a"),
			version:            getproviders.MustParseVersion("1.0.0"),
			versionConstraints: getproviders.MustParseVersionConstraints(">= 1.0.0"),
			// intentionally no hashes here, but the version and versionConstraints
			// must match the entry above.
		},

		// This one does not get any special treatment because it's not in
		// the namespace where the OpenTofu project maintains
		// directly-corresponding releases.
		addrs.MustParseProviderSourceString("registry.terraform.io/anything-else/b"): {
			addr:    addrs.MustParseProviderSourceString("registry.terraform.io/anything-else/b"),
			version: getproviders.MustParseVersion("2.0.0"),
			hashes: []getproviders.Hash{
				getproviders.HashScheme1.New("jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9MhT0="),
			},
		},

		// The following two both survive unchanged because we don't want to
		// destroy any existing lock file entry for a provider from the
		// OpenTofu registry even if there's an entry that could potentially
		// have been translated.
		addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/c"): {
			addr:    addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/c"),
			version: getproviders.MustParseVersion("3.0.0"),
			hashes: []getproviders.Hash{
				getproviders.HashScheme1.New("jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9MhT0="),
			},
		},
		addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/c"): {
			addr:    addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/c"),
			version: getproviders.MustParseVersion("4.0.0"),
			hashes: []getproviders.Hash{
				getproviders.HashScheme1.New("jsKjBiLb+v3OIC3xuDiY4sR0r1OHUMSWPYKult9T0Mh="),
			},
		},
	}
	if diff := cmp.Diff(wantLocks, gotLocks, ProviderLockComparer); diff != "" {
		t.Error("wrong updated locks\n" + diff)
	}
}
