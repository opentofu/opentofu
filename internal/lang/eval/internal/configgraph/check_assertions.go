// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"iter"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type CheckAssertions struct {
	Addr       addrs.AbsCheck
	Assertions iter.Seq[*CheckRule]
	DeclRange  hcl.Range

	NestedResource func(ctx context.Context) *ResourceInstance

	valueOnce grapheval.Once[cty.ValueMarks]
}

func (c *CheckAssertions) assert(ctx context.Context) (cty.ValueMarks, tfdiags.Diagnostics) {
	// TODO CheckDataResource elsewhere
	ctx = withDebugAddr(ctx, c.Addr, "Assert")
	return c.valueOnce.Do(ctx, func(ctx context.Context) (cty.ValueMarks, tfdiags.Diagnostics) {
		if c.NestedResource != nil {
			val, nestedDiags := c.NestedResource(ctx).Value(ctx)
			if exprs.IsEvalError(val) {
				// TODO nestedDiags does not work correctly here during apply
				msg := "apply bug"
				if len(nestedDiags) > 0 {
					msg = nestedDiags.ErrWithWarnings().Error()
				}
				checks.ReportCheckInContext(ctx, c.Addr, addrs.CheckAssertion, checks.StatusFail, msg)
				return nil, nil
			}
		}
		return CheckAllRulesDefault(ctx, c.Addr, addrs.CheckAssertion, c.Assertions, new(tfdiags.SourceRangeFromHCL(c.DeclRange)))
	}, new(tfdiags.SourceRangeFromHCL(c.DeclRange)), "Assertions of %s", c.Addr)
}

func (c *CheckAssertions) ResourceDependencies(ctx context.Context) iter.Seq[addrs.AbsResourceInstance] {
	marks, _ := c.assert(ctx)
	return func(yield func(addrs.AbsResourceInstance) bool) {
		for mark := range marks {
			if riMark, ok := mark.(ResourceInstanceMark); ok {
				if !yield(riMark.instance.Addr) {
					return
				}
			}
		}
	}
}

func (c *CheckAssertions) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	_, diags := c.assert(ctx)
	return diags
}
