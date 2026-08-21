// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := tc.contextBuilder(t.Context())
			v := lintHintsFromContext(ctx)
			got := lintRuleAllowed(v.include, v.exclude, tc.givenRuleID, tc.givenGroupIDs...)
			if tc.want != got {
				t.Errorf("expected for the ruleID %+v (groupIDs %+v) to return %t but got %t", tc.givenRuleID, tc.givenGroupIDs, tc.want, got)
			}
		})
	}
}

func TestExecuteLintRule(t *testing.T) {
	t.Run("rule executed only once", func(t *testing.T) {
		ctx := ContextWithLintFilterHints(t.Context(), collections.NewSet(linting.MustParseRuleAddr("core:foo")), collections.NewSet[linting.RuleAddr]())
		var calls int
		exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) Diagnostics {
			calls++
			return nil
		}
		for range 3 {
			_ = ExecuteLintRule(ctx, exec, SourceRange{Filename: "test.tf", Start: SourcePos{Line: 1, Column: 2}, End: SourcePos{Line: 1, Column: 10}}, linting.MustParseRuleAddr("core:foo"))
		}
		if calls != 1 {
			t.Errorf("expected to be called only once but got %d", calls)
		}
	})
	t.Run("parallel calls locks when called for exactly the same source and lint rule", func(t *testing.T) {
		ruleID := linting.MustParseRuleAddr("core:foo")
		ctx := ContextWithLintFilterHints(t.Context(), collections.NewSet(ruleID), collections.NewSet[linting.RuleAddr]())
		var called atomic.Bool
		exec := func(i int) func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) Diagnostics {
			return func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) Diagnostics {
				called.Store(true)
				return nil
			}
		}
		src := SourceRange{Filename: "test.tf", Start: SourcePos{Line: 1, Column: 2}, End: SourcePos{Line: 1, Column: 10}}
		var wg sync.WaitGroup
		for i := range 500 {
			wg.Go(func() {
				_ = ExecuteLintRule(ctx, exec(i), src, ruleID)
				if !called.Load() {
					t.Errorf("[%d] returned before having the rule executed by the routine that acquired the lock", i)
				}
			})
		}
		wg.Wait()
		// execute the same rule one more time to be sure that the execution of it is not performed (because it should be already registered as executed successfully)
		called.Store(false)
		_ = ExecuteLintRule(ctx, exec(0), src, ruleID)
		if called.Load() {
			t.Errorf("the linting rule unexpectedly called. This means that something is broken in the internals of ExecuteLintRule since this was meant be registered as successfully executed")
		}
	})
	t.Run("parallel calls does not lock when called for different source and lint rule", func(t *testing.T) {
		const samples = 500
		ctx := ContextWithLintFilterHints(t.Context(), collections.NewSet(linting.MustParseRuleAddr("all")), collections.NewSet[linting.RuleAddr]())
		var sources []SourceRange
		for i := range samples {
			sources = append(sources, SourceRange{Filename: "test.tf", Start: SourcePos{Line: i, Column: 2}, End: SourcePos{Line: 1, Column: 10}})
		}
		exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) Diagnostics {
			return nil
		}
		results := make(chan struct{}, samples)
		for i := range samples {
			go func() {
				_ = ExecuteLintRule(ctx, exec, sources[i], linting.MustParseRuleAddr("core:foo"))
				results <- struct{}{}
			}()
		}
		var finished int
	done:
		for {
			select {
			case _, ok := <-results:
				if !ok {
					break done
				}
				finished++
				if finished == samples {
					break done
				}
			case <-time.After(time.Second):
				// This is just a sanity select case to ensure that if something goes wrong, the test actually ends
				// with a clear error message
				t.Error("executions locked. This highlights an issue in the linting rules execution")
				break done
			}
		}
		defer close(results)
		if finished != samples {
			t.Fatalf("expected to have %d executions but got %d", samples, finished)
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
