// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestNewModule_provider_foreach_name(t *testing.T) {
	mod, diags := testModuleFromDir("testdata/providers_foreach")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	p := addrs.NewProvider(addrs.DefaultProviderRegistryHost, "hashicorp", "local")
	if name, exists := mod.ProviderLocalNames[p]; !exists {
		t.Fatal("provider FQN hashicorp/local not found")
	} else if name != "local" {
		t.Fatalf("provider localname mismatch: got %s, want local", name)
	}

	if len(mod.ProviderConfigs) != 3 {
		t.Fatal("incorrect numver of providers")
	}

	_, foundDev := mod.GetProviderConfig("foo-test", "dev")
	if !foundDev {
		t.Fatal("unable to find dev provider")
	}

	_, foundTest := mod.GetProviderConfig("foo-test", "test")
	if !foundTest {
		t.Fatal("unable to find test provider")
	}

	_, foundProd := mod.GetProviderConfig("foo-test", "prod")
	if !foundProd {
		t.Fatal("unable to find prod provider")
	}
}

func TestNewModule_provider_count(t *testing.T) {
	mod, diags := testModuleFromDir("testdata/providers_count")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	p := addrs.NewProvider(addrs.DefaultProviderRegistryHost, "hashicorp", "local")
	if name, exists := mod.ProviderLocalNames[p]; !exists {
		t.Fatal("provider FQN hashicorp/local not found")
	} else if name != "local" {
		t.Fatalf("provider localname mismatch: got %s, want local", name)
	}

	if len(mod.ProviderConfigs) != 3 {
		t.Fatal("incorrect numver of providers")
	}

	_, foundDev := mod.GetProviderConfig("foo-test", "[0]")
	if !foundDev {
		t.Fatal("unable to find 0 provider")
	}

	_, foundTest := mod.GetProviderConfig("foo-test", "[1]")
	if !foundTest {
		t.Fatal("unable to find 1 provider")
	}

	_, foundProd := mod.GetProviderConfig("foo-test", "[2]")
	if !foundProd {
		t.Fatal("unable to find 2 provider")
	}
}
