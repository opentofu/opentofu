// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// VarsAndLocalsCollector defines how a collector for used variables and locals looks like.
// This is used by the linting implementation that is meant to issue warnings for the not used (unreferenced)
// locals and variables.
//
// The implementations of this interface must handle properly concurrent requests.
type VarsAndLocalsCollector interface {
	CollectVariable(*configs.Variable)
	CollectLocal(*configs.Local)
	Validate(context.Context) tfdiags.Diagnostics
}

// usedVarsAndLocals provides the main functionality for VarsAndLocalsCollector by storing the
// used variables (CollectVariable) and locals (CollectLocal) and when Validate is called it will generate
// linting diagnostics for all the root configuration variables and locals that were not registered
// as used.
type usedVarsAndLocals struct {
	rootModuleCfg *configs.Config

	usedVars   addrs.SyncMap[addrs.ConfigVariable, *configs.Variable]
	usedLocals addrs.SyncMap[addrs.ConfigLocal, *configs.Local]
}

// UsedVarsAndLocalsCollector creates a new VarsAndLocalsCollector.
//
// The received context.Context is expected to contain the linting configurations provided by the user. If there
// are no such configs, this method will return a noOpUsedVarsAndLocals.
// This particular function handles 2 linting rules: ruleIDLocalNotUsed and ruleIDVariableNotUsed.
// It will initialise the collector with only the capabilities depending on which of the linting rule aforementioned
// is found in the context. If both are missing, it will return a noOpUsedVarsAndLocals.
//
// The received configs.Config is meant to point to the root module to be able to check all the defined variables
// and locals against the collected used ones. If the root config is nil, it will return a noOpUsedVarsAndLocals.
func UsedVarsAndLocalsCollector(ctx context.Context, rootModuleCfg *configs.Config) VarsAndLocalsCollector {
	if rootModuleCfg == nil {
		return &noOpUsedVarsAndLocals{}
	}
	varsEnabled := tfdiags.LintRuleEnabled(ctx, ruleIDVariableNotUsed, GroupIDImprovement)
	localsEnabled := tfdiags.LintRuleEnabled(ctx, ruleIDLocalNotUsed, GroupIDImprovement)
	if !varsEnabled && !localsEnabled {
		return &noOpUsedVarsAndLocals{}
	}
	var (
		usedVars   addrs.SyncMap[addrs.ConfigVariable, *configs.Variable]
		usedLocals addrs.SyncMap[addrs.ConfigLocal, *configs.Local]
	)
	if varsEnabled {
		usedVars = addrs.MakeSyncMap[addrs.ConfigVariable, *configs.Variable]()
	}
	if localsEnabled {
		usedLocals = addrs.MakeSyncMap[addrs.ConfigLocal, *configs.Local]()
	}
	return &usedVarsAndLocals{
		rootModuleCfg: rootModuleCfg,
		usedVars:      usedVars,
		usedLocals:    usedLocals,
	}
}

// CollectVariable stores the given variable as being used. When Validate will be called, the existence of the given
// variable will prevent from issuing lack of usage linting diagnostic.
func (u *usedVarsAndLocals) CollectVariable(vc *configs.Variable) {
	if vc == nil {
		return
	}
	if !u.usedVars.Initialised() {
		return
	}
	u.usedVars.Put(addrs.ConfigVariable{
		Variable:  vc.Addr(),
		DeclRange: vc.DeclRange,
	}, vc)
}

// CollectLocal stores the given local as being used. When Validate will be called, the existence of the given
// local will prevent from issuing lack of usage linting diagnostic.
func (u *usedVarsAndLocals) CollectLocal(lc *configs.Local) {
	if lc == nil {
		return
	}
	if !u.usedLocals.Initialised() {
		return
	}
	u.usedLocals.Put(addrs.ConfigLocal{
		Local:     lc.Addr(),
		DeclRange: lc.DeclRange,
	}, lc)
}

// Validate checks all the variables and locals of the root config (if the linting rule for each of those was enabled)
// and for any subject that was not recorded as being used, it will generate a linting diagnostic.
func (u *usedVarsAndLocals) Validate(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if u.usedVars.Initialised() {
		for _, c := range u.rootModuleCfg.Module.Variables {
			vaddr := addrs.ConfigVariable{Variable: c.Addr(), DeclRange: c.DeclRange}
			if u.usedVars.Has(vaddr) {
				continue
			}
			diags = diags.Append(variableNotUsed(ctx, c))
		}
	}
	if u.usedLocals.Initialised() {
		for _, c := range u.rootModuleCfg.Module.Locals {
			laddr := addrs.ConfigLocal{Local: c.Addr(), DeclRange: c.DeclRange}
			if u.usedLocals.Has(laddr) {
				continue
			}
			diags = diags.Append(localNotUsed(ctx, c))
		}
	}
	return diags
}

// variableNotUsed generates a linting diagnostic for a not used variable.
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

// localNotUsed generates a linting diagnostic for a not used local.
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

// noOpUsedVarsAndLocals implements VarsAndLocalsCollector by discarding any received request.
type noOpUsedVarsAndLocals struct {
}

func (n *noOpUsedVarsAndLocals) CollectLocal(*configs.Local) {
}

func (n *noOpUsedVarsAndLocals) CollectVariable(*configs.Variable) {
}

func (n *noOpUsedVarsAndLocals) Validate(context.Context) tfdiags.Diagnostics {
	return nil
}
