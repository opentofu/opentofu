// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/zclconf/go-cty/cty"
)

func TestLintRuleEnabled(t *testing.T) {
	cases := map[string]struct {
		contextBuilder func(ctx context.Context) context.Context
		givenRuleID    linting.RuleAddr
		givenGroupIDs  []linting.RuleAddr
		givenSource    SourceRange
		want           bool
	}{
		"context missing linting configuration": {
			contextBuilder: func(ctx context.Context) context.Context {
				return ctx
			},
			want:        false,
			givenRuleID: linting.MustParseRuleAddr("foo"),
		},
		"context linting configuration is wrongly configured": {
			contextBuilder: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, lintingRulesCtxKey{}, "definitely not the right struct here")
			},
			want:        false,
			givenRuleID: linting.MustParseRuleAddr("foo"),
		},
		"with all included and nothing else": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](
					linting.AllRulesGroupID,
				)
				exclude := collections.NewSet[linting.RuleAddr]()
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:        true,
			givenRuleID: linting.MustParseRuleAddr("foo"),
		},
		"with all excluded and nothing else": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr]()
				exclude := collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID)
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:        false,
			givenRuleID: linting.MustParseRuleAddr("foo"),
		},
		"with all included and rule id excluded": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID)
				exclude := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("foo"))
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:        false,
			givenRuleID: linting.MustParseRuleAddr("foo"),
		},
		"with all included, given group excluded but the rule included": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID, linting.MustParseRuleAddr("foo"))
				exclude := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo"))
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:          true,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: []linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
		},
		"group included but rule excluded": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo"))
				exclude := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("foo"))
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:          false,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: []linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
		},
		"some groups included and some excluded": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo1"))
				exclude := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo2"))
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:          true,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: []linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo1"), linting.MustParseRuleAddr("group_including_foo2"), linting.MustParseRuleAddr("group_including_foo3")},
		},
		"same group included and excluded": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo"))
				exclude := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo"))
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:          true,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: []linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
		},
		"with all excluded, group included, rule unspecified": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo"))
				exclude := collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID)
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:          true,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: []linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
		},
		"with all included, group excluded, rule unspecified": {
			contextBuilder: func(ctx context.Context) context.Context {
				include := collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID)
				exclude := collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo"))
				return ContextWithLintFilterHints(t.Context(), include, exclude)
			},
			want:          false,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: []linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
		},
		"with the same rule and group being already checked once": {
			contextBuilder: func(ctx context.Context) context.Context {
				v := lintingRulesCtxValue{
					include:  collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID),
					exclude:  collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
					executed: collections.NewSet(keyForLintCall(SourceRange{Filename: "test.tf", Start: SourcePos{Line: 1, Column: 1, Byte: 1}}, linting.MustParseRuleAddr("foo"))), // no group id provided here
				}
				return context.WithValue(ctx, lintingRulesCtxKey{}, &v)
			},
			want:          false,
			givenRuleID:   linting.MustParseRuleAddr("foo"),
			givenGroupIDs: nil, // no group ID provided
			givenSource: SourceRange{
				Filename: "test.tf",
				Start:    SourcePos{Line: 1, Column: 1, Byte: 1},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := tc.contextBuilder(t.Context())
			got := LintRuleEnabledOnSource(ctx, tc.givenSource, tc.givenRuleID, tc.givenGroupIDs...)
			if tc.want != got {
				t.Errorf("expected for the ruleID %+v (groupIDs %+v) to return %t but got %t", tc.givenRuleID, tc.givenGroupIDs, tc.want, got)
			}
		})
	}
	t.Run("running exactly the same rule twice returns true only the first time", func(t *testing.T) {
		rID := linting.MustParseRuleAddr("foo")
		gID := linting.MustParseRuleAddr("group_including_foo")
		src := SourceRange{
			Filename: "test.tf",
			Start:    SourcePos{Line: 1, Column: 1, Byte: 1},
		}
		ctx := ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID), collections.NewSet[linting.RuleAddr](gID))
		for i := range 10 {
			got := LintRuleEnabledOnSource(ctx, src, rID) // no group id provided
			if i == 0 {
				if !got {
					t.Errorf("[%d] expected for the rule to be allowed but was not", i)
				}
				return
			}
			if got {
				t.Errorf("[%d] expected for the rule to be denied because it was executed once but it was allowed", i)
			}
		}
	})
}

func TestFilterLint(t *testing.T) {
	cases := map[string]struct {
		include, exclude collections.Set[linting.RuleAddr]
		givenDiags       Diagnostics
		wantDiags        Diagnostics
	}{
		"with all included and nothing else": {
			include: collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID),
			exclude: collections.NewSet[linting.RuleAddr](),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					nil,
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					nil,
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
		},
		"with all excluded and nothing else": {
			include: collections.NewSet[linting.RuleAddr](),
			exclude: collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					nil,
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
			),
		},
		"with all included and rule id excluded": {
			include: collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID),
			exclude: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("foo")),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					nil,
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
			),
		},
		"with all included, given group excluded but the rule included": {
			include: collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID, linting.MustParseRuleAddr("foo")),
			exclude: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
		},
		"group included but rule excluded": {
			include: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
			exclude: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("foo")),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
			),
		},
		"some groups included and some excluded": {
			include: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo1")),
			exclude: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo2")),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo1"), linting.MustParseRuleAddr("group_including_foo2")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo1"), linting.MustParseRuleAddr("group_including_foo2")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
		},
		"same group included and excluded": {
			include: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
			exclude: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
		},
		"with all excluded, group included, rule unspecified": {
			include: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
			exclude: collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
		},
		"with all included, group excluded, rule unspecified": {
			include: collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID),
			exclude: collections.NewSet[linting.RuleAddr](linting.MustParseRuleAddr("group_including_foo")),
			givenDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
				LintMessage(
					linting.MustParseRuleAddr("foo"),
					[]linting.RuleAddr{linting.MustParseRuleAddr("group_including_foo")},
					"lint summary",
					"lint details",
					nil,
					nil,
				),
			),
			wantDiags: New(
				AttributeValue(Error, "error diag", "error diag details", cty.Path{}),
				AttributeValue(Warning, "warning diag", "warning diag details", cty.Path{}),
			),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.givenDiags.FilterLint(tc.include, tc.exclude)
			if diff := cmp.Diff(tc.wantDiags, got, cmpopts.IgnoreUnexported(attributeDiagnostic{}, lintMessage{})); diff != "" {
				t.Errorf("unexpected returned diagnostics (-want,+got):\n%s", diff)
			}
		})
	}
}
