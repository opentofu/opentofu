// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file contains test helpers for use in other packages. These functions
// must not be used in non-test code.

// ModulesForTesting returns an [evalglue.ExternalModules] implementation that just
// returns module objects directly from the provided map, without any additional
// logic.
//
// This is intended for unit testing only, and only supports local module
// source addresses because it has no means to resolve remote sources or
// selected versions for registry-based modules.
//
// [configs.ModulesFromStringsForTesting] is a convenient way to build a
// suitable map to pass to this function when the required configuration is
// relatively small.
func ModulesForTesting(modules map[addrs.ModuleSourceLocal]*configs.Module) evalglue.ExternalModules {
	return externalModulesStatic{modules}
}

type externalModulesStatic struct {
	modules map[addrs.ModuleSourceLocal]*configs.Module
}

// ModuleConfig implements ExternalModules.
func (ms externalModulesStatic) ModuleConfig(_ context.Context, source addrs.ModuleSource, _ versions.Set, _ *addrs.AbsModuleCall) (evalglue.UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	localSource, ok := source.(addrs.ModuleSourceLocal)
	if !ok {
		diags = diags.Append(fmt.Errorf("only local module source addresses are supported for this test"))
		return nil, diags
	}
	mod, ok := ms.modules[localSource]
	if !ok {
		diags = diags.Append(fmt.Errorf("module path %q is not available to this test", localSource))
		return nil, diags
	}
	return NewUncompiledModule(source, mod), diags
}
