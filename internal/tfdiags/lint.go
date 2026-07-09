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
		if lintRuleAllowed(include, exclude, ld.ruleID, ld.groupIDs) {
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

	executedM sync.Mutex
	executed  collections.Set[string]
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
	return context.WithValue(parent, lintingRulesCtxKey{}, &lintingRulesCtxValue{
		include:  include,
		exclude:  exclude,
		executed: collections.NewSet[string](),
	})
}

// LintRuleEnabledOnSource checks the given context for any linting configuration and if none found will return false.
// If the context contains a linting configuration, it will return true in the following cases:
//   - the given lint rule on the given source is checked for the first time
//   - the context contained linting configuration allows the rule to be executed (included and not excluded)
//
// Every call at this method will also register a hash of the given arguments which is intended to ensure that the
// same lint rule on the same configuration is not executed multiple times. In other words, if a linting rule on the
// same configuration block is executed first during validate and once during plan, only the one during validate
// will run. This ensures that the same rule for the same configuration block is not executed multiple times which
// would cause duplicate diagnostics for the same entry.
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
// When it comes to how the linting configuration is applied, the ruleID has priority compared with the groupID,
// meaning that if that particular ruleID inclusion or exclusion exists, this will return once it finds that.
// If the ruleID is not found as being mentioned specifically, this method checks the given groupIDs
// and applies the same logic: if the group is specifically included, that takes priority in front of its exclusion
// existence.
// If none of the groupIDs are specifically configured to be included/excluded, then this will return true
// if the inclusion contains "all".
func LintRuleEnabledOnSource(ctx context.Context, src SourceRange, ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) bool {
	v := ctx.Value(lintingRulesCtxKey{})
	if v == nil {
		return false
	}
	val, ok := v.(*lintingRulesCtxValue)
	if !ok {
		return false
	}
	k := keyForLintCall(src, ruleID, groupIDs...)
	val.executedM.Lock()
	if val.executed.Has(k) {
		val.executedM.Unlock()
		return false
	}
	val.executed[k] = struct{}{}
	val.executedM.Unlock()
	return lintRuleAllowed(val.include, val.exclude, ruleID, groupIDs)
}

// keyForLintCall creates a sha256 representation of the given arguments. This is used to be stored and checked against
// when the same linting rule on the same configuration source wants to execute multiple times.
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

// isLint returns true if the given diagnostic is of lintMessage type.
func isLint(diag Diagnostic) bool {
	_, ok := diag.(lintMessage)
	return ok
}
