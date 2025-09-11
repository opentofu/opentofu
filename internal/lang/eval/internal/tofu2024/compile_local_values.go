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

func compileModuleInstanceLocalValues(_ context.Context, configs map[string]*configs.Local, declScope exprs.Scope, moduleInstAddr addrs.ModuleInstance) map[addrs.LocalValue]*configgraph.LocalValue {
	ret := make(map[addrs.LocalValue]*configgraph.LocalValue, len(configs))
	for name, vc := range configs {
		addr := addrs.LocalValue{Name: name}
		value := configgraph.ValuerOnce(exprs.NewClosure(
			exprs.EvalableHCLExpression(vc.Expr),
			declScope,
		))
		ret[addr] = &configgraph.LocalValue{
			Addr:     moduleInstAddr.LocalValue(name),
			RawValue: value,
		}
	}
	return ret
}
