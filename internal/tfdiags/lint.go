// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"

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

func (lm lintMessage) Severity() Severity {
	return LintingWarning
}

// Description overrides the diagnosticBase method to provide a custom summary for the
// linting diagnostics.
// The summary entry for a linting diagnostic will have the following format:
//
//	<summary> (<rule_id>[, <groupID1>, <groupID2>, ...)
func (lm lintMessage) Description() Description {
	s := strings.TrimSpace(lm.diagnosticBase.summary)
	if s == "" {
		s = "<missing summary>"
	}
	all := make([]string, len(lm.groupIDs)+1)
	all[0] = lm.ruleID.String()
	for i, r := range lm.groupIDs {
		all[i+1] = r.String()
	}
	desc := lm.diagnosticBase.Description()
	desc.Summary = fmt.Sprintf("%s (%s)", s, strings.Join(all, ", "))
	return desc
}

func (lm lintMessage) Source() Source {
	return Source{
		Subject: lm.subject,
		Context: lm.context,
	}
}

// LintMessage create a new diagnostic that is specifically configured to be processed as a linting warning later.
// Even though the LintingWarning severity is exported from this package, the linting diagnostics should be created
// only by using this function. Otherwise, the rendering and handling of this type of diagnostic might result
// in unexpected behavior.
func LintMessage(ruleID linting.RuleAddr, groupIDs []linting.RuleAddr, summary string, details string, subject *SourceRange, context *SourceRange) Diagnostic {
	return lintMessage{
		diagnosticBase: diagnosticBase{
			severity: LintingWarning,
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
		if lintRuleAllowed(include, exclude, ld.ruleID, ld.groupIDs) {
			ret = append(ret, srcDiag)
		}
	}

	return ret
}

// ConsolidateLint is meant to return only one lint diagnostic of the same type and the same source.
// If the linting diagnostic is configured correctly (with a ruleId and a subject), then this will
// remove duplicates of the same linting diagnostic.
// Duplicates can be issued for the same linting issue on the same subject when multiple phases
// are executed in one command (eg: `tofu apply` runs validate, plan, apply which will generate 3
// duplicates of the same diagnostic).
//
// This particular method skips the handling of nil source on purpose because the used
// method (eg: `Consolidate`), already skips consolidation for those diagnostics.
func (diags Diagnostics) ConsolidateLint() Diagnostics {
	return diags.Consolidate(1, LintingWarning, func(diag Diagnostic) string {
		defaultKey := func() string {
			desc := diag.Description()
			consolidationKey := desc.Summary
			// If the diagnostic has a keyable extra info and it's not empty,
			// use it as the consolidation key, along with the summary.
			// Otherwise use the summary only.
			if key, keyOk := diag.ExtraInfo().(Keyable); keyOk {
				consolidationKey += key.ExtraInfoKey()
			}
			return consolidationKey
		}
		ld, ok := diag.(lintMessage)
		if !ok {
			log.Printf("[ERROR] A non-lint severity diagnostic reached into lint diagnostics consolidation: %s. Returning a default diagnostic consolidation key", reflect.TypeOf(diag))
			return defaultKey()
		}
		consolidationKey := ld.ruleID.String()
		consolidationKey += ld.subject.StartString()
		return consolidationKey
	}, ConsolidationOptDefault^ConsolidationOptIncludeCount)
	// ^ This uses the default consolidation opts and excludes from that the count calculation. This means that any
	// other future option added into the ConsolidationOptDefault will be automatically used by the consolidation call.
}

type lintingRulesCtxKey struct {
}
type lintingRulesCtxValue struct {
	include collections.Set[linting.RuleAddr]
	exclude collections.Set[linting.RuleAddr]
}

// ContextWithLintFilterHints returns a new [context.Context] that's derived
// from the given parent context but also tracks the sets of lint rule IDs that
// will be used to filter any diagnostics returned from whatever function the
// resulting context was passed to.
//
// Use the returned context with [LintRuleEnabled] elsewhere in the system to
// skip expensive work to generate a lint diagnostic that is going to get
// discarded eventually anyway.
func ContextWithLintFilterHints(parent context.Context, include, exclude collections.Set[linting.RuleAddr]) context.Context {
	return context.WithValue(parent, lintingRulesCtxKey{}, lintingRulesCtxValue{
		include: include,
		exclude: exclude,
	})
}

// LintRuleEnabled returns true if the given context has lint filter hints that
// suggest that a [lintMessage] with the given ruleID or at least one groupID would survive UI-level
// filtering of lint messages, or false otherwise.
//
// TODO linting - this is from the RFC and I strongly disagree with this.
//
//	If the given context is not derived from one previously returned by
//	[ContextWithLintFilterHints] then the result is always true, suggesting that
//	nothing will be filtered. This defaults to enabled because we assume that
//	any codepath that isn't calling this function also won't call
//	[Diagnostics.FilterLint]; this pair of functions should typically be used
//	together by the same subsystem, passing the same include/exclude sets to
//	both.
//
// Instead of including any rule, instead, this disables all the rules, which can be used as a indirect way to
// signal to the system that linting is enabled or not. In other words, the existence of lintingRulesCtxKey
// in the ctx is the indication that the linting is enabled or not.
//
// TODO linting (end)
//
// The ruleID has priority, meaning that if that particular ruleID inclusion or exclusion will return
// once it's found.
// If the ruleID is not found as being mentioned specifically, this method checks the given groupIDs
// and applies the same logic: if the group is specifically included, that takes priority in front of its exclusion
// existence.
// If none of the groupIDs are specifically configured to be included/excluded, then this will return true
// if the inclusion contains "all".
func LintRuleEnabled(ctx context.Context, ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) bool {
	v := ctx.Value(lintingRulesCtxKey{})
	if v == nil {
		return false
	}
	val, ok := v.(lintingRulesCtxValue)
	if !ok {
		return false
	}
	return lintRuleAllowed(val.include, val.exclude, ruleID, groupIDs)
}

func lintRuleAllowed(include, exclude collections.Set[linting.RuleAddr], ruleID linting.RuleAddr, groupIDs []linting.RuleAddr) bool {
	if include.Has(ruleID) {
		return true
	}
	if exclude.Has(ruleID) {
		return false
	}
	// If at least one group is included, then include the rule
	for _, d := range groupIDs {
		if include.Has(d) {
			return true
		}
	}
	// If at least one group is exclude, then exclude the rule
	for _, d := range groupIDs {
		if exclude.Has(d) {
			return false
		}
	}
	// In the end, enable the rule only when the "all" configuration is provided
	return include.Has(linting.AllRulesGroupID)
}
