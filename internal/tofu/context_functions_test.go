// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tofu/testhelpers"
	"github.com/zclconf/go-cty/cty"
)

// Standard scenario using root provider explicitly passed
func TestContext2Functions_providerFunctions(t *testing.T) {
	p := testhelpers.TestProvider("aws")
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"region": &configschema.Attribute{
						Type: cty.String,
					},
				},
			},
		},
		Functions: map[string]providers.FunctionSpec{
			"arn_parse": providers.FunctionSpec{
				Parameters: []providers.FunctionParameterSpec{{
					Name: "arn",
					Type: cty.String,
				}},
				Return: cty.Bool,
			},
		},
	}
	p.CallFunctionResponse = &providers.CallFunctionResponse{
		Result: cty.True,
	}
	m := testhelpers.TestModuleInline(t, map[string]string{
		"main.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

provider "aws" {
  region="us-east-1"
}

module "mod" {
  source = "./mod"
  providers = {
    aws = aws
  }
}
 `,
		"mod/mod.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

variable "obfmod" {
  type = object({
    arns = optional(list(string))
  })
  description = "Configuration for xxx."

  validation {
    condition = alltrue([
      for arn in var.obfmod.arns: can(provider::aws::arn_parse(arn))
    ])
    error_message = "All arns MUST BE a valid AWS ARN format."
  }

  default = {
    arns = [
      "arn:partition:service:region:account-id:resource-id",
    ]
  }
}
`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	diags := ctx.Validate(context.Background(), m)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Expected function call")
	}
}

// Explicitly passed provider with custom function
func TestContext2Functions_providerFunctionsCustom(t *testing.T) {
	p := testhelpers.TestProvider("aws")
	p.GetFunctionsResponse = &providers.GetFunctionsResponse{
		Functions: map[string]providers.FunctionSpec{
			"arn_parse_custom": providers.FunctionSpec{
				Parameters: []providers.FunctionParameterSpec{{
					Name: "arn",
					Type: cty.String,
				}},
				Return: cty.Bool,
			},
		},
	}
	p.CallFunctionResponse = &providers.CallFunctionResponse{
		Result: cty.True,
	}
	m := testhelpers.TestModuleInline(t, map[string]string{
		"main.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

provider "aws" {
  region="us-east-1"
  alias = "primary"
}

module "mod" {
  source = "./mod"
  providers = {
    aws = aws.primary
  }
}
 `,
		"mod/mod.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

variable "obfmod" {
  type = object({
    arns = optional(list(string))
  })
  description = "Configuration for xxx."

  validation {
    condition = alltrue([
      for arn in var.obfmod.arns: can(provider::aws::arn_parse_custom(arn))
    ])
    error_message = "All arns MUST BE a valid AWS ARN format."
  }

  default = {
    arns = [
      "arn:partition:service:region:account-id:resource-id",
    ]
  }
}
`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	diags := ctx.Validate(context.Background(), m)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if p.GetFunctionsCalled {
		t.Fatalf("Unexpected function call")
	}
	if p.CallFunctionCalled {
		t.Fatalf("Unexpected function call")
	}

	p.GetFunctionsCalled = false
	p.CallFunctionCalled = false
	_, diags = ctx.Plan(context.Background(), m, nil, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.GetFunctionsCalled {
		t.Fatalf("Expected function call")
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Expected function call")
	}
}

// Defaulted stub provider with non-custom function
func TestContext2Functions_providerFunctionsStub(t *testing.T) {
	p := testhelpers.TestProvider("aws")

	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Functions: map[string]providers.FunctionSpec{
			"arn_parse": providers.FunctionSpec{
				Parameters: []providers.FunctionParameterSpec{{
					Name: "arn",
					Type: cty.String,
				}},
				Return: cty.Bool,
			},
		},
	}
	p.CallFunctionResponse = &providers.CallFunctionResponse{
		Result: cty.True,
	}

	m := testhelpers.TestModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}
 `,
		"mod/mod.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

variable "obfmod" {
  type = object({
    arns = optional(list(string))
  })
  description = "Configuration for xxx."

  validation {
    condition = alltrue([
      for arn in var.obfmod.arns: can(provider::aws::arn_parse(arn))
    ])
    error_message = "All arns MUST BE a valid AWS ARN format."
  }

  default = {
    arns = [
      "arn:partition:service:region:account-id:resource-id",
    ]
  }
}
`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	diags := ctx.Validate(context.Background(), m)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.GetProviderSchemaCalled {
		t.Fatalf("Unexpected function call")
	}
	if p.GetFunctionsCalled {
		t.Fatalf("Unexpected function call")
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Unexpected function call")
	}

	p.GetProviderSchemaCalled = false
	p.GetFunctionsCalled = false
	p.CallFunctionCalled = false
	_, diags = ctx.Plan(context.Background(), m, nil, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.GetProviderSchemaCalled {
		t.Fatalf("Unexpected function call")
	}
	if p.GetFunctionsCalled {
		t.Fatalf("Expected function call")
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Expected function call")
	}
}

// Defaulted stub provider with custom function (no allowed)
func TestContext2Functions_providerFunctionsStubCustom(t *testing.T) {
	p := testhelpers.TestProvider("aws")

	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Functions: map[string]providers.FunctionSpec{
			"arn_parse": providers.FunctionSpec{
				Parameters: []providers.FunctionParameterSpec{{
					Name: "arn",
					Type: cty.String,
				}},
				Return: cty.Bool,
			},
		},
	}
	p.CallFunctionResponse = &providers.CallFunctionResponse{
		Result: cty.True,
	}

	m := testhelpers.TestModuleInline(t, map[string]string{
		"main.tf": `
module "mod" {
  source = "./mod"
}
 `,
		"mod/mod.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

variable "obfmod" {
  type = object({
    arns = optional(list(string))
  })
  description = "Configuration for xxx."

  validation {
    condition = alltrue([
      for arn in var.obfmod.arns: can(provider::aws::arn_parse_custom(arn))
    ])
    error_message = "All arns MUST BE a valid AWS ARN format."
  }

  default = {
    arns = [
      "arn:partition:service:region:account-id:resource-id",
    ]
  }
}
`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, nil, nil)
	if !diags.HasErrors() {
		t.Fatal("Expected error!")
	}
	expected := `Function not found in provider: Function "provider::aws::arn_parse_custom" was not registered by provider`
	if expected != diags.Err().Error() {
		t.Fatalf("Expected error %q, got %q", expected, diags.Err().Error())
	}
	if !p.GetFunctionsCalled {
		t.Fatalf("Expected function call")
	}
	if p.CallFunctionCalled {
		t.Fatalf("Unexpected function call")
	}
}

// Defaulted stub provider
func TestContext2Functions_providerFunctionsForEachCount(t *testing.T) {
	p := testhelpers.TestProvider("aws")

	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Functions: map[string]providers.FunctionSpec{
			"arn_parse": providers.FunctionSpec{
				Parameters: []providers.FunctionParameterSpec{{
					Name: "arn",
					Type: cty.String,
				}},
				Return: cty.Bool,
			},
		},
	}
	p.CallFunctionResponse = &providers.CallFunctionResponse{
		Result: cty.True,
	}

	m := testhelpers.TestModuleInline(t, map[string]string{
		"main.tf": `
provider "aws" {
  for_each = {"a": 1, "b": 2}
  alias = "iter"
}
module "mod" {
  source = "./mod"
  for_each = {"a": 1, "b": 2}
  providers = {
    aws = aws.iter[each.key]
  }
}
 `,
		"mod/mod.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

variable "obfmod" {
  type = object({
    arns = optional(list(string))
  })
  description = "Configuration for xxx."

  validation {
    condition = alltrue([
      for arn in var.obfmod.arns: can(provider::aws::arn_parse(arn))
    ])
    error_message = "All arns MUST BE a valid AWS ARN format."
  }

  default = {
    arns = [
      "arn:partition:service:region:account-id:resource-id",
    ]
  }
}
`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	diags := ctx.Validate(context.Background(), m)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.GetProviderSchemaCalled {
		t.Fatalf("Unexpected function call")
	}
	if p.GetFunctionsCalled {
		t.Fatalf("Unexpected function call")
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Unexpected function call")
	}

	p.GetProviderSchemaCalled = false
	p.GetFunctionsCalled = false
	p.CallFunctionCalled = false
	_, diags = ctx.Plan(context.Background(), m, nil, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.GetProviderSchemaCalled {
		t.Fatalf("Unexpected function call")
	}
	if p.GetFunctionsCalled {
		t.Fatalf("Expected function call")
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Expected function call")
	}
}

// Functions used as variable values are evaluated correctly
func TestContext2Functions_providerFunctionsVariableCustom(t *testing.T) {
	p := testhelpers.TestProvider("aws")
	p.GetFunctionsResponse = &providers.GetFunctionsResponse{
		Functions: map[string]providers.FunctionSpec{
			"arn_parse_custom": providers.FunctionSpec{
				Parameters: []providers.FunctionParameterSpec{{
					Name: "arn",
					Type: cty.String,
				}},
				Return: cty.Bool,
			},
		},
	}
	p.CallFunctionResponse = &providers.CallFunctionResponse{
		Result: cty.True,
	}
	m := testhelpers.TestModuleInline(t, map[string]string{
		"main.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

provider "aws" {
  region="us-east-1"
  alias = "primary"
}

module "mod" {
  source = "./mod"
  providers = {
    aws = aws.primary
  }
}
 `,
		"mod/mod.tf": `
terraform {
  required_providers {
    aws = ">=5.70.0"
  }
}

module "mod2" {
       source = "./mod2"
       value = provider::aws::arn_parse_custom("foo")
}
`,
		"mod/mod2/mod.tf": `
variable "value" { }
`,
	})

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("aws"): testProviderFuncFixed(p),
		},
	})

	diags := ctx.Validate(context.Background(), m)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if p.GetFunctionsCalled {
		t.Fatalf("Unexpected function call")
	}
	if p.CallFunctionCalled {
		t.Fatalf("Unexpected function call")
	}

	p.GetFunctionsCalled = false
	p.CallFunctionCalled = false
	_, diags = ctx.Plan(context.Background(), m, nil, nil)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if !p.GetFunctionsCalled {
		t.Fatalf("Expected function call")
	}
	if !p.CallFunctionCalled {
		t.Fatalf("Expected function call")
	}
}
