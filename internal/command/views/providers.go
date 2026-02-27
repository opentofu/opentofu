// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/xlab/treeprint"
)

type Providers interface {
	Diagnostics(diags tfdiags.Diagnostics)
	ModuleRequirements(cfg *configs.ModuleRequirements)
	StateRequirements(stateReqs getproviders.Requirements)
}

// NewProviders returns an initialized Providers implementation for the given ViewType.
func NewProviders(args arguments.ViewOptions, view *View) Providers {
	switch args.ViewType {
	case arguments.ViewHuman:
		return &ProvidersHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}
}

type ProvidersHuman struct {
	view *View
}

var _ Providers = (*ProvidersHuman)(nil)

func (v *ProvidersHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersHuman) ModuleRequirements(reqs *configs.ModuleRequirements) {
	printRoot := treeprint.New()
	populateTreeNode(printRoot, reqs)

	_, _ = v.view.streams.Println("\nProviders required by configuration:")
	_, _ = v.view.streams.Println(printRoot.String())

}

func (v *ProvidersHuman) StateRequirements(stateReqs getproviders.Requirements) {
	if len(stateReqs) == 0 {
		return
	}
	reqs := slices.Collect(maps.Keys(stateReqs))
	slices.SortFunc(reqs, func(a, b addrs.Provider) int {
		return strings.Compare(a.String(), b.String())
	})
	_, _ = v.view.streams.Println("Providers required by state:")
	// Initially, the newline was at the end of the message above, but go built-in linting does not allow it
	_, _ = v.view.streams.Println("")
	for _, fqn := range reqs {
		_, _ = v.view.streams.Println(fmt.Sprintf("    provider[%s]\n", fqn.String()))
	}
}

func populateTreeNode(tree treeprint.Tree, node *configs.ModuleRequirements) {
	for fqn, dep := range node.Requirements {
		versionsStr := getproviders.VersionConstraintsString(dep)
		if versionsStr != "" {
			versionsStr = " " + versionsStr
		}
		tree.AddNode(fmt.Sprintf("provider[%s]%s", fqn.String(), versionsStr))
	}
	for name, testNode := range node.Tests {
		name = strings.TrimSuffix(name, ".tftest.hcl")
		name = strings.ReplaceAll(name, "/", ".")
		branch := tree.AddBranch(fmt.Sprintf("test.%s", name))

		for fqn, dep := range testNode.Requirements {
			versionsStr := getproviders.VersionConstraintsString(dep)
			if versionsStr != "" {
				versionsStr = " " + versionsStr
			}
			branch.AddNode(fmt.Sprintf("provider[%s]%s", fqn.String(), versionsStr))
		}

		for _, run := range testNode.Runs {
			branch := branch.AddBranch(fmt.Sprintf("run.%s", run.Name))
			populateTreeNode(branch, run)
		}
	}
	for name, childNode := range node.Children {
		branch := tree.AddBranch(fmt.Sprintf("module.%s", name))
		populateTreeNode(branch, childNode)
	}
}
