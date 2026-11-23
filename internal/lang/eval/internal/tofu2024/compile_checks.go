// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
)

func compileCheckAssertions(ctx context.Context, configs map[string]*configs.Check, declScope exprs.Scope, moduleInstanceAddr addrs.ModuleInstance, resourceNodes map[addrs.Resource]*configgraph.Resource) map[addrs.Check]*configgraph.CheckAssertions {
	// TODO allow nested data sources here and only here
	ret := map[addrs.Check]*configgraph.CheckAssertions{}
	for _, config := range configs {
		confAddr := config.Addr()
		addr := confAddr.Absolute(moduleInstanceAddr)
		assertions := &configgraph.CheckAssertions{
			Addr:       addr,
			Assertions: compileCheckRules(config.Asserts, declScope),
			DeclRange:  config.DeclRange,
		}

		if config.DataResource != nil {
			assertions.NestedResource = func(ctx context.Context) *configgraph.ResourceInstance {
				return resourceNodes[config.DataResource.Addr()].Instances(ctx)[addrs.NoKey]
			}
		}

		ret[confAddr] = assertions
	}
	return ret
}
