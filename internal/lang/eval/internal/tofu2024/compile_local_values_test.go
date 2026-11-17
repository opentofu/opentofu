// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
)

func TestCompileModuleInstanceLocalValues(t *testing.T) {
	// Local values currently have very little special logic and mostly just
	// thinly wrap the general exprs package functionality, so this test
	// is intentionally minimal and assumes we're already testing the
	// underlying functionality well in its own package.

	configs := map[string]*configs.Local{
		"literal": {
			Name: "literal",
			Expr: hcltest.MockExprLiteral(cty.StringVal("literal value")),
		},
		"reference": {
			Name: "reference",
			Expr: hcltest.MockExprTraversalSrc(`a`),
		},
	}
	declScope := exprs.FlatScopeForTesting(map[string]cty.Value{
		"a": cty.StringVal("value of a"),
	})
	moduleInstAddr := addrs.RootModuleInstance.Child("foo", addrs.NoKey)
	wantValues := map[string]cty.Value{
		"literal":   cty.StringVal("literal value"),
		"reference": cty.StringVal("value of a"),
	}

	got := compileModuleInstanceLocalValues(t.Context(), configs, declScope, moduleInstAddr)
	if got, want := len(got), len(wantValues); got != want {
		t.Errorf("unexpected number of compiled results %d; want %d", got, want)
	}

	for name, wantValue := range wantValues {
		t.Run(name, func(t *testing.T) {
			lv := got[addrs.LocalValue{Name: name}]
			if lv == nil {
				t.Fatalf("missing %q value in result", name)
			}
			ctx := grapheval.ContextWithNewWorker(t.Context())
			ctx = grapheval.ContextWithRequestTracker(ctx, grapheval.NewRequestTracker())
			val, diags := lv.Value(ctx)
			if diags.HasErrors() {
				t.Errorf("unexpected errors from %q: %s", name, diags.Err())
			}
			if got, want := val, wantValue; !want.RawEquals(got) {
				t.Errorf("wrong value for %q\ngot:  %#v\nwant: %#v", name, got, want)
			}
			if got, want := lv.Addr, moduleInstAddr.LocalValue(name); got.String() != want.String() {
				t.Errorf("wrong address for %q\ngot:  %s\nwant: %s", name, got, want)
			}
		})
	}
}
