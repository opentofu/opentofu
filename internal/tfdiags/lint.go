// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/linting"
)

// lintMessage represents a message describing a linting problem.
//
// Pass pointers to values of this type to [Diagnostics.Append] to include
// lint messages as part of your returned diagnostics.
type lintMessage struct {
	diagnosticBase
	// ruleID is the rule ID. This is a mandatory field since each lintMessage
	// needs to be linked directly to a specific linting rule.
	ruleID linting.RuleAddr

	// groupIDs is a set of all of the linting group IDs this message relates to.
	//
	// This is an optional field that identifies one or multiple groups this linting
	// rule is part of.
	groupIDs []linting.RuleAddr

	subject *SourceRange

	// context is an optional additional source range that must enclose
	// the range in subject but can include additional characters that provide
	// relevant context for understanding the problem. The human-oriented
	// diagnostic renderer uses this to extend the source code snippet to
	// include additional lines from the input configuration.
	context *SourceRange
}

// Description overrides the diagnosticBase method to provide a custom summary for the
// linting diagnostics.
// The summary entry for a linting diagnostic will have the following format:
//
//	<summary> (<rule_id>)
//
// For the moment, the groupIDs will be documented separately in the public documentation
// and later we can enhance our way of informing the operators about the groups a rule
// is part of.
func (lm lintMessage) Description() Description {
	s := strings.TrimSpace(lm.diagnosticBase.summary)
	if s == "" {
		s = "<missing summary>"
	}
	desc := lm.diagnosticBase.Description()
	desc.Summary = fmt.Sprintf("%s (%s)", s, lm.ruleID.String())
	return desc
}

func (lm lintMessage) Source() Source {
	return Source{
		Subject: lm.subject,
		Context: lm.context,
	}
}

// LintMessage create a new diagnostic that is specifically configured to be processed as a linting warning later.
// This is the main method to create linting related diagnostics. If new types will be added to support additional
// linting functionality, ensure that the type is included correctly in all the places linting diagnostics have
// specific processing logic.
func LintMessage(ruleID linting.RuleAddr, groupIDs []linting.RuleAddr, summary string, details string, subject *SourceRange, context *SourceRange) Diagnostic {
	return lintMessage{
		diagnosticBase: diagnosticBase{
			severity: Warning,
			summary:  summary,
			detail:   details,
		},
		ruleID:   ruleID,
		groupIDs: groupIDs,
		subject:  subject,
		context:  context,
	}
}

// FilterLint modifies the backing array of the receiver so that any
// [lintMessage] diagnostics which don't match the given include and exclude rules
// are removed, and then returns a new slice over the valid part of the updated
// backing array.
//
// UI code should call this to exclude any unrequested lint rules before
// rendering the diagnostics.
func (diags Diagnostics) FilterLint(include, exclude collections.Set[linting.RuleAddr]) Diagnostics {
	if len(diags) == 0 {
		return nil
	}

	ret := make(Diagnostics, 0, len(diags))
	for _, srcDiag := range diags {
		ld, isLd := srcDiag.(lintMessage)
		if !isLd {
			ret = append(ret, srcDiag)
			continue
		}
		if lintRuleAllowed(include, exclude, ld.ruleID, ld.groupIDs...) {
			ret = append(ret, srcDiag)
		}
	}

	return ret
}

// SplitLint is a method that returns two slices, first with non-linting diagnostics
// and the second one with linting related diagnostics.
// This is needed to be able to not mix linting warning diagnostics with the regular warning diagnostics while
// the warning diagnostics are consolidated.
func (diags Diagnostics) SplitLint() (Diagnostics, Diagnostics) {
	if len(diags) == 0 {
		return nil, nil
	}

	nonLintDiags := make(Diagnostics, 0)
	lintDiags := make(Diagnostics, 0)
	for _, srcDiag := range diags {
		if isLint(srcDiag) {
			lintDiags = append(lintDiags, srcDiag)
			continue
		}
		nonLintDiags = append(nonLintDiags, srcDiag)
	}

	return nonLintDiags, lintDiags
}

type lintingRulesCtxKey struct {
}
type lintingRulesCtxValue struct {
	include collections.Set[linting.RuleAddr]
	exclude collections.Set[linting.RuleAddr]

	// lintOnSourceExecution holds the execution container for a specific (linting rule on a specific config source).
	// This is to ensure that even if it happens to have the same linting rule executed twice for the same configuration
	// construct, only one will report the final status.
	// Further, this is a way to signal for the different execution phases (validate, plan, apply) that a linting rule
	// on a specific config construct has been executed. This way it avoids running the same logic multiple times.
	// The future phases execution will be skipped only if the entry in this map contains a success run.
	lintOnSourceExecution sync.Map
}

// ContextWithLintFilterHints returns a new [context.Context] that's derived
// from the given parent context but also tracks the sets of lint rule IDs that
// will be used to filter any diagnostics returned from whatever function the
// resulting context was passed to.
//
// Use the returned context with [lintRuleEnabled] elsewhere in the system to
// skip expensive work to generate a lint diagnostic that is going to get
// discarded eventually anyway.
func ContextWithLintFilterHints(parent context.Context, include, exclude collections.Set[linting.RuleAddr]) context.Context {
	return context.WithValue(parent, lintingRulesCtxKey{}, &lintingRulesCtxValue{
		include:               include,
		exclude:               exclude,
		lintOnSourceExecution: sync.Map{},
	})
}

// lintHintsFromContext returns the *lintingRulesCtxValue from the given context.
// If the context has no value, it returns nil.
func lintHintsFromContext(ctx context.Context) *lintingRulesCtxValue {
	v := ctx.Value(lintingRulesCtxKey{})
	if v == nil {
		return nil
	}
	val, ok := v.(*lintingRulesCtxValue)
	if !ok {
		return nil
	}
	return val
}

// ExecuteLintRule executes the given linting rule logic after it checks that it is allowed to execute or not.
// If the context is configured for linting, it carries a container that stores the sucess status of each combination
// of (ruleID, ruleIDs and src). If the container contains a successfull run for that combination, it doesn't execute
// it again. This way, the same combination can be executed multiple times and it will not be executed more than
// once.
//
// NOTE: later, f can be enhanced to return another bool to have the run skipped and not marked as a success execution,
// which can silently control when a phase has enough information and when not to determine if the rule can be
// executed reliably.
func ExecuteLintRule(ctx context.Context, f func() Diagnostics, src SourceRange, ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) Diagnostics {
	lintCtx := lintHintsFromContext(ctx)
	if lintCtx == nil {
		return nil
	}
	// first, determine that the user actually asked for this
	if !lintRuleAllowed(lintCtx.include, lintCtx.exclude, ruleID, groupIDs...) {
		return nil
	}

	// generate the identification key based on the ruleID, groupIDs and the place in the configuration for which
	// this is executed
	k := keyForLintCall(src, ruleID, groupIDs...)

	type lintExecution struct {
		m       sync.Mutex
		success bool
	}
	loadExec := func(execId string) *lintExecution {
		// create a new lint rule executin and lock it to store it into the status container
		exec := &lintExecution{
			m:       sync.Mutex{},
			success: false,
		}
		exec.m.Lock()
		loadedExec, loaded := lintCtx.lintOnSourceExecution.LoadOrStore(k, exec)
		currentExec := exec
		// another lint execution for this linting rule and source was created before so we need to work with it instead of
		// the above created one.
		if loaded {
			// we unlock the previous created execution just to be sure that we don't leave that mutex locked,
			// even if it will e discarded upon return from this function
			exec.m.Unlock()
			// get the existing execution, acquire lock, and use this instead of the previously created execution
			cLoadedExec := loadedExec.(*lintExecution)
			cLoadedExec.m.Lock()
			currentExec = cLoadedExec
		}

		return currentExec
	}

	exec := loadExec(k)
	defer exec.m.Unlock()
	if exec.success {
		return nil
	}
	diags := f()
	if !diags.HasErrors() {
		// safe to set this here since the lock was acquired when this was created, before being stored into the
		// status container
		exec.success = true
	}
	return diags
}

// keyForLintCall creates a sha256 representation of the given arguments. This is used to be stored and avoid
// having the same combination of (ruleID+groupIDs+src) executed more than once.
func keyForLintCall(src SourceRange, ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) string {
	var s bytes.Buffer
	_, _ = s.WriteString(src.StartString())
	_, _ = s.WriteString(ruleID.String())
	for _, d := range groupIDs {
		_, _ = s.WriteString(d.String())
	}
	h := sha256.New()
	_, _ = h.Write(s.Bytes())
	return hex.EncodeToString(h.Sum(nil))
}

func lintRuleAllowed(include, exclude collections.Set[linting.RuleAddr], ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) bool {
	if include.Has(ruleID) {
		return true
	}
	if exclude.Has(ruleID) {
		return false
	}
	// If at least one group is included, then include the rule
	if slices.ContainsFunc(groupIDs, include.Has) {
		return true
	}
	// If at least one group is exclude, then exclude the rule
	if slices.ContainsFunc(groupIDs, exclude.Has) {
		return false
	}
	// In the end, enable the rule only when the "all" configuration is provided
	return include.Has(linting.AllRulesGroupID)
}

// isLint returns true if the given diagnostic is of lintMessage type.
func isLint(diag Diagnostic) bool {
	_, ok := diag.(lintMessage)
	return ok
}
