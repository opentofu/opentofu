// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/providers"
)

// The symbols aliased in this file are defined in [evalglue] really just to
// avoid a dependency between this package and the "compiler" packages
// like ./internal/tofu2024, but we do still need them in our exported API
// here so that other parts of OpenTofu can interact with the evaluator.

type EvalContext = evalglue.EvalContext
type Providers = evalglue.Providers
type Provisioners = evalglue.Provisioners
type ExternalModules = evalglue.ExternalModules

func ModulesForTesting(modules map[addrs.ModuleSourceLocal]*configs.Module) ExternalModules {
	return evalglue.ModulesForTesting(modules)
}

func ProvidersForTesting(schemas map[addrs.Provider]*providers.GetProviderSchemaResponse) Providers {
	return evalglue.ProvidersForTesting(schemas)
}

func ProvisionersForTesting(schemas map[string]*configschema.Block) Provisioners {
	return evalglue.ProvisionersForTesting(schemas)
}
