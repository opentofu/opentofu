// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
)

const (
	providerTestName = "local"
)

func TestNewModule_provider_foreach(t *testing.T) {
	mod, diags := testModuleFromDir("testdata/providers_foreach")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	p := addrs.NewProvider(addrs.DefaultProviderRegistryHost, "hashicorp", providerTestName)
	if name, exists := mod.ProviderLocalNames[p]; !exists {
		t.Fatal("provider FQN hashicorp/local not found")
	} else if name != providerTestName {
		t.Fatalf("provider localname mismatch: got %s, want %s", name, providerTestName)
	}

	if len(mod.ProviderConfigs) != 3 {
		t.Fatalf("incorrect number of providers: got %d, expected: %d", len(mod.ProviderConfigs), 3)
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

func TestNewModule_provider_invalid_name(t *testing.T) {
	mod, diags := testModuleFromDir("testdata/providers_iteration_invalid_name")
	if !diags.HasErrors() {
		t.Fatal("expected error")
	}
	expected := "Invalid for_each key alias"
	expectedDetail := "Alias \"0\" must be a valid name. A name must start with a letter or underscore and may contain only letters, digits, underscores, and dashes."

	if gotErr := diags[0].Summary; gotErr != expected {
		t.Errorf("wrong error, got %q, want %q", gotErr, expected)
	}
	if gotErr := diags[0].Detail; gotErr != expectedDetail {
		t.Errorf("wrong error, got %q, want %q", gotErr, expectedDetail)
	}

	p := addrs.NewProvider(addrs.DefaultProviderRegistryHost, "hashicorp", providerTestName)
	if name, exists := mod.ProviderLocalNames[p]; !exists {
		t.Fatal("provider FQN hashicorp/local not found")
	} else if name != providerTestName {
		t.Fatalf("provider localname mismatch: got %s, want %s", name, providerTestName)
	}

	if len(mod.ProviderConfigs) != 0 {
		t.Fatalf("incorrect number of providers: got %d, expected: %d", len(mod.ProviderConfigs), 0)
	}
}
