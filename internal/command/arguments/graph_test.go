// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseGraph_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Graph
	}{
		"defaults": {
			nil,
			graphArgsWithDefaults(nil),
		},
		"draw-cycles flag": {
			[]string{"-draw-cycles"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.DrawCycles = true
			}),
		},
		"type flag": {
			[]string{"-type=plan"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.GraphType = "plan"
			}),
		},
		"type flag plan-refresh-only": {
			[]string{"-type=plan-refresh-only"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.GraphType = "plan-refresh-only"
			}),
		},
		"type flag plan-destroy": {
			[]string{"-type=plan-destroy"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.GraphType = "plan-destroy"
			}),
		},
		"type flag apply": {
			[]string{"-type=apply"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.GraphType = "apply"
			}),
		},
		"module-depth flag": {
			[]string{"-module-depth=2"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.ModuleDepth = 2
			}),
		},
		"module-depth flag zero": {
			[]string{"-module-depth=0"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.ModuleDepth = 0
			}),
		},
		"verbose flag": {
			[]string{"-verbose"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.Verbose = true
			}),
		},
		"plan flag": {
			[]string{"-plan=/path/to/plan.tfplan"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.PlanPath = "/path/to/plan.tfplan"
			}),
		},
		"multiple flags combined": {
			[]string{"-draw-cycles", "-type=plan", "-verbose"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.DrawCycles = true
				graph.GraphType = "plan"
				graph.Verbose = true
			}),
		},
		"all flags combined": {
			[]string{"-draw-cycles", "-type=apply", "-module-depth=3", "-verbose", "-plan=plan.tfplan"},
			graphArgsWithDefaults(func(graph *Graph) {
				graph.DrawCycles = true
				graph.GraphType = "apply"
				graph.ModuleDepth = 3
				graph.Verbose = true
				graph.PlanPath = "plan.tfplan"
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseGraph(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func graphArgsWithDefaults(mutate func(graph *Graph)) *Graph {
	ret := &Graph{
		DrawCycles:  false,
		GraphType:   "",
		ModuleDepth: -1,
		Verbose:     false,
		PlanPath:    "",
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Vars: &Vars{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
