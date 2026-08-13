// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type UsedVarsCollector interface {
	CollectVariable(vc *configs.Variable)
	CollectLocal(lc *configs.Local)
	Validate(ctx context.Context) tfdiags.Diagnostics
}

type usedVarsCollector struct {
	rootModuleCfg *configs.Config

	usedVars map[string]*configs.Variable
	uvm      sync.Mutex

	usedLocals map[string]*configs.Local
	ulm        sync.Mutex
}

func NewUsedVarsCollector(ctx context.Context, rootModuleCfg *configs.Config) UsedVarsCollector {
	varsEnabled := tfdiags.LintRuleEnabled(ctx, ruleIDVariableNotUsed, GroupIDImprovement)
	localsEnabled := tfdiags.LintRuleEnabled(ctx, ruleIDLocalNotUsed, GroupIDImprovement)
	if !varsEnabled && !localsEnabled {
		return &noOpUsedVarsCollector{}
	}
	var (
		usedVars   map[string]*configs.Variable
		usedLocals map[string]*configs.Local
	)
	if varsEnabled {
		usedVars = make(map[string]*configs.Variable)
	}
	if localsEnabled {
		usedLocals = make(map[string]*configs.Local)
	}
	return &usedVarsCollector{
		rootModuleCfg: rootModuleCfg,
		usedVars:      usedVars,
		usedLocals:    usedLocals,
	}
}

func (u *usedVarsCollector) CollectVariable(vc *configs.Variable) {
	if vc == nil {
		return
	}
	u.uvm.Lock()
	defer u.uvm.Unlock()
	if u.usedVars == nil {
		// not enabled
		return
	}
	u.usedVars[usedKey(vc.Name, vc.DeclRange)] = vc
}

func (u *usedVarsCollector) CollectLocal(lc *configs.Local) {
	if lc == nil {
		return
	}
	u.uvm.Lock()
	defer u.uvm.Unlock()
	if u.usedLocals == nil {
		// not enabled
		return
	}
	u.usedLocals[usedKey(lc.Name, lc.DeclRange)] = lc
}

func (u *usedVarsCollector) Validate(ctx context.Context) tfdiags.Diagnostics {
	if u.rootModuleCfg == nil {
		return nil
	}
	var diags tfdiags.Diagnostics
	if u.usedVars != nil {
		for _, c := range u.rootModuleCfg.Module.Variables {
			if _, ok := u.usedVars[usedKey(c.Name, c.DeclRange)]; ok {
				continue
			}
			diags = diags.Append(variableNotUsed(ctx, c))
		}
	}
	if u.usedLocals != nil {
		for _, c := range u.rootModuleCfg.Module.Locals {
			if _, ok := u.usedLocals[usedKey(c.Name, c.DeclRange)]; ok {
				continue
			}
			diags = diags.Append(localNotUsed(ctx, c))
		}
	}
	return diags
}

func usedKey(name string, declRange hcl.Range) string {
	rawK := fmt.Sprintf("%s-%s", name, declRange.String())
	h := sha256.New()
	h.Write([]byte(rawK))
	return hex.EncodeToString(h.Sum(nil))
}

func variableNotUsed(ctx context.Context, vc *configs.Variable) tfdiags.Diagnostics {
	exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) tfdiags.Diagnostics {
		return tfdiags.New(tfdiags.LintMessage(
			ruleID,
			groupIDs,
			"Variable not used",
			fmt.Sprintf("Found no usage of the variable %q", vc.Name),
			new(tfdiags.SourceRangeFromHCL(vc.DeclRange)),
			nil,
		))
	}
	return tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(vc.DeclRange), ruleIDVariableNotUsed, GroupIDImprovement)
}

func localNotUsed(ctx context.Context, lc *configs.Local) tfdiags.Diagnostics {
	exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) tfdiags.Diagnostics {
		return tfdiags.New(tfdiags.LintMessage(
			ruleID,
			groupIDs,
			"Local not used",
			fmt.Sprintf("Found no usage of the local %q", lc.Name),
			new(tfdiags.SourceRangeFromHCL(lc.DeclRange)),
			nil,
		))
	}
	return tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(lc.DeclRange), ruleIDLocalNotUsed, GroupIDImprovement)
}

type noOpUsedVarsCollector struct {
}

func (n *noOpUsedVarsCollector) CollectLocal(*configs.Local) {
}

func (n *noOpUsedVarsCollector) CollectVariable(*configs.Variable) {
}

func (n *noOpUsedVarsCollector) Validate(context.Context) tfdiags.Diagnostics {
	return nil
}
