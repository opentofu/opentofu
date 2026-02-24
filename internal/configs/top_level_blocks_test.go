// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"
)

// TestTopLevelBlocksOnly verifies that the parser correctly identifies
// top-level blocks in a configuration file. It checks that the
// expected number of each type of block is present and that they
// are correctly parsed into the file structure.
func TestTopLevelBlocksOnly(t *testing.T) {
	topLevelOnlyConfig := `
backend "local" {
  path = "test.tfstate"
}

required_providers {
  test = {
    source  = "hashicorp/test"
    version = "1.0.0"
  }
}

cloud {
  organization = "test-org"
  workspaces {
    name = "test-workspace"
  }
}

provider_meta "test" {
  foo = "bar"
}

encryption {
  key_provider "pgp" "test-key" {
    key_id = "keyid"
  }
}
`
	parser := testParser(map[string]string{
		"top-level-only.tf": topLevelOnlyConfig,
	})

	file, diags := parser.LoadConfigFile("top-level-only.tf")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags)
	}

	if file == nil {
		t.Fatal("Expected file to be non-nil")
	}

	if len(file.Backends) != 1 {
		t.Errorf("Expected 1 backend block, got %d", len(file.Backends))
	}
	if len(file.RequiredProviders) != 1 {
		t.Errorf("Expected 1 required_providers block, got %d", len(file.RequiredProviders))
	}
	if len(file.CloudConfigs) != 1 {
		t.Errorf("Expected 1 cloud block, got %d", len(file.CloudConfigs))
	}
	if len(file.ProviderMetas) != 1 {
		t.Errorf("Expected 1 provider_meta block, got %d", len(file.ProviderMetas))
	}
	if len(file.Encryptions) != 1 {
		t.Errorf("Expected 1 encryption block, got %d", len(file.Encryptions))
	}
}

// TestTopLevelBlocksConflict_ShouldError verifies that the parser correctly
// identifies that there is a conflict between a top-level "backend"
// block and a "terraform" block in the same configuration file.
func TestTopLevelBlocksConflict_ShouldError(t *testing.T) {
	conflictingContent := `
terraform {
  required_version = ">= 1.6.0"
  backend "remote" {}
}

backend "local" {}
`
	parser := testParser(map[string]string{
		"conflict.tf": conflictingContent,
	})

	_, diags := parser.LoadConfigFile("conflict.tf")
	if !diags.HasErrors() {
		t.Fatal("expected error diagnostics for conflicting blocks")
	}

	assertExactDiagnostics(t, diags, []string{"conflict.tf:7,1-16: Top-level \"backend\" block not allowed alongside terraform block; A \"backend\" block cannot be used at the top level whilst a terraform block exists in the file. Move this \"backend\" block inside the terraform block or remove the existing terraform block."})
}

// TestTerraformBlocksOnly verifies that the parser correctly
// identifies and parses the terraform block correctly when it has no other
// conflicting top-level blocks.
func TestTerraformBlocksOnly(t *testing.T) {
	terraformOnlyConfig := `
terraform {
  required_version = ">= 1.6.0"
  
  backend "remote" {
    organization = "test-org"
    workspaces {
      name = "test-workspace-internal"
    }
  }
  
  required_providers {
    test-internal = {
      source  = "hashicorp/test-internal"
      version = "1.0.0"
    }
  }

  provider_meta "test" {
    foo = "bar"
  }

  encryption {
    key_provider "pgp" "test-key" {
      key_id = "keyid"
    }
  }
}
`
	parser := testParser(map[string]string{
		"terraform-block-only.tf": terraformOnlyConfig,
	})

	file, diags := parser.LoadConfigFile("terraform-block-only.tf")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags)
	}

	if len(file.Backends) != 1 {
		t.Errorf("Expected 1 backend block, got %d", len(file.Backends))
	}
	if len(file.RequiredProviders) != 1 {
		t.Errorf("Expected 1 required_providers block, got %d", len(file.RequiredProviders))
	}
	if len(file.ProviderMetas) != 1 {
		t.Errorf("Expected 1 provider_meta block, got %d", len(file.ProviderMetas))
	}
	if len(file.Encryptions) != 1 {
		t.Errorf("Expected 1 encryption block, got %d", len(file.Encryptions))
	}

	module, modDiags := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "", SelectiveLoadAll)
	if modDiags.HasErrors() {
		t.Fatalf("unexpected module errors: %s", modDiags)
	}

	if module.ProviderRequirements == nil {
		t.Fatal("Expected provider requirements to be set")
	}

	providers := module.ProviderRequirements.RequiredProviders
	if len(providers) != 1 {
		t.Errorf("Expected 1 provider in requirements, got %d", len(providers))
	}
	if _, exists := providers["test-internal"]; !exists {
		t.Error("Expected test-internal provider to be present")
	}
}
