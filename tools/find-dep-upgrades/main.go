// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// find-dep-upgrades is a utility for finding the available upgrades for our
// Go module dependencies and proposing an order to upgrade them in so that
// as far as possible each upgrade touches only one upstream module at a time.
//
// This is because upgrading dependencies, particularly to newer minor versions,
// can potentially change OpenTofu's end-user-observable behavior and so we
// may need to document such behavior changes in our changelog. The analysis
// required to do that gets far more complicated when upgrading many different
// dependencies at once, but by upgrading "leaf" dependencies first and only
// then upgrading what they depend on we can minimize the scope of each upgrade.
//
// Run this from the root of your work tree for the OpenTofu repository so
// that it can find the project's "go.mod" file in the current working
// directory:
//
//	go run ./tools/find-dep-upgrades
package main

import (
	"cmp"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/opentofu/opentofu/internal/dag"
)

func main() {
	log.SetOutput(os.Stderr)

	candidates, err := findUpgradeCandidates()
	if err != nil {
		log.Fatalf("failed searching for upgrade candidates: %s", err)
	}

	// Now we'll collect the dependencies of the potential new version of
	// each upgrade candidate, so we can understand which upgrades would
	// force other upgrades to happen as a side-effect.
	latestVersionDeps := make(map[ModulePath]map[ModulePath]Version)
	for modulePath, candidate := range candidates {
		deps, err := findModuleDependencies(modulePath, candidate.LatestVersion)
		if err != nil {
			log.Fatalf("failed finding dependencies of %s@%s: %s", modulePath, candidate.LatestVersion, err)
		}
		latestVersionDeps[modulePath] = deps
	}

	// We'll now build a dependency graph for upgrades, where we consider A
	// to depend on B if upgrading A would force upgrading B (and therefore
	// we ideally want to upgrade B first).
	//
	// We already have the necessary algorithms implemented in our package dag,
	// so we'll use that here even though the complexity of that package's
	// design is arguably overkill for this relatively simple problem.
	//
	// Note that, counter-intuitively, we're using AcyclicGraph here even though
	// the graph we're going to build likely _will_ contain cycles, because
	// package dag only offers the methods for finding cycles on the
	// AcyclicGraph type!
	g := &dag.AcyclicGraph{}
	for candidatePath := range candidates {
		g.Add(candidatePath)

		// If the latest version of this one requires a newer version of
		// another one than we currently have selected then we've found
		// an upgrade ordering constraint.
		for depPath, reqdVersion := range latestVersionDeps[candidatePath] {
			depCandidate, ok := candidates[depPath]
			if !ok {
				// This dependency is not for a module we care about for the
				// sake of this analysis.
				continue
			}
			if reqdVersion.GreaterThan(depCandidate.CurrentVersion) {
				g.Connect(dag.BasicEdge(candidatePath, depPath))
			}
		}
	}

	// This dependency graph _will_ tend to contain cycles because we're
	// considering each upgrade separately and it's possible that the MVS
	// ratchet effect provides no ordering that would upgrade only exactly
	// one module at a time.
	//
	// For any cycle we find we'll group all of the involved modules together
	// into a single node that we treat as a single unit for upgrade purposes.
	cycles := g.Cycles()
	for _, cycle := range cycles {
		group := make(ModulePaths, len(cycle))
		for _, v := range cycle {
			group[v.(ModulePath)] = struct{}{}
		}
		// Calling "Replace" multiple times with the same replacement but
		// different "original" works because the implicit g.Add in this
		// function is a silent no-op when the given vertex already exists,
		// and then it still does all of the necessary edge manipulation.
		for _, v := range cycle {
			// We use a pointer to group here, rather than just plain group,
			// because then the graph membership is based on pointer identity.
			// (ModulePaths itself is not comparable/hashable).
			g.Replace(v, &group)
		}
		// After all of the replacing we just did it's likely that the group
		// node now depends on itself, so we'll delete that edge if present.
		g.RemoveEdge(dag.BasicEdge(&group, &group))
	}

	order := g.TopologicalOrder()
	pendingUpgrades := make([]PendingUpgradeCluster, 0, len(order))
	for _, v := range slices.Backward(order) {
		var modulePaths ModulePaths
		switch v := v.(type) {
		case ModulePath:
			modulePaths = ModulePaths{v: struct{}{}}
		case *ModulePaths:
			modulePaths = *v
		}

		var pendingCluster PendingUpgradeCluster
		for modulePath := range modulePaths {
			candidate := candidates[modulePath]
			pending := PendingUpgrade{
				Module:         candidate.Module,
				CurrentVersion: candidate.CurrentVersion,
				LatestVersion:  candidate.LatestVersion,
				Prereqs:        make(map[ModulePath]Version),
			}
			for depModulePath, depNewVersion := range latestVersionDeps[modulePath] {
				depCandidate, ok := candidates[depModulePath]
				if !ok {
					continue
				}
				if depCandidate.CurrentVersion.LessThan(depNewVersion) {
					pending.Prereqs[depModulePath] = depNewVersion
				}
			}
			pendingCluster = append(pendingCluster, pending)
		}
		// Within a cluster the members will list each other as prereqs,
		// but we already dealt with that by clustering them together so
		// we'll just delete those entries to focus only on the prereqs
		// from outside the group.
		for i := range pendingCluster {
			for _, pending := range pendingCluster {
				delete(pendingCluster[i].Prereqs, pending.Module)
			}
		}
		slices.SortFunc(pendingCluster, func(a, b PendingUpgrade) int {
			return cmp.Compare(a.Module, b.Module)
		})
		pendingUpgrades = append(pendingUpgrades, pendingCluster)
	}

	var readyUpgrades []PendingUpgradeCluster
	var blockedUpgrades []PendingUpgradeCluster
	for _, cluster := range pendingUpgrades {
		ready := true
		for _, upgrade := range cluster {
			if len(upgrade.Prereqs) != 0 {
				ready = false
				break
			}
		}
		if ready {
			readyUpgrades = append(readyUpgrades, cluster)
		} else {
			blockedUpgrades = append(blockedUpgrades, cluster)
		}
	}

	// We'll sort the "ready" clusters into lexical order because without
	// any dependencies their topological order is completely arbitrary.
	// (This intentionally leaves blockedUpgrades untouched because the
	// topological order of that is relatively useful to plan what
	// order to run a series of upgrades in.)
	slices.SortFunc(readyUpgrades, func(a, b PendingUpgradeCluster) int {
		return cmp.Compare(clusterCaption(a), clusterCaption(b))
	})

	printPendingUpgradeClusters(readyUpgrades)
	if len(readyUpgrades) != 0 && len(blockedUpgrades) != 0 {
		fmt.Print("\n---\n\n")
	}
	printPendingUpgradeClusters(blockedUpgrades)
}

func printPendingUpgradeClusters(clusters []PendingUpgradeCluster) {
	for _, cluster := range clusters {
		fmt.Printf("- **%s**\n", clusterCaption(cluster))
		for _, pending := range cluster {
			changesURL := changelogURL(pending.Module, pending.CurrentVersion, pending.LatestVersion)
			if changesURL != "" {
				fmt.Printf("    - [ ] `go get %s@v%s` ([from `v%s`](%s))\n", pending.Module, pending.LatestVersion, pending.CurrentVersion, changesURL)
			} else {
				fmt.Printf("    - [ ] `go get %s@v%s` (from `v%s`)\n", pending.Module, pending.LatestVersion, pending.CurrentVersion)
			}
			prereqs := slices.Collect(maps.Keys(pending.Prereqs))
			slices.Sort(prereqs)
			for _, depModulePath := range prereqs {
				fmt.Printf("        - requires `%s@v%s`\n", depModulePath, pending.Prereqs[depModulePath])
			}
		}
	}
}

func clusterCaption(cluster PendingUpgradeCluster) string {
	if len(cluster) == 0 {
		// Should not make an empty cluster, but we'll tolerate it anyway.
		return "(empty set of modules)"
	}
	if len(cluster) == 1 {
		return "`" + string(cluster[0].Module) + "`"
	}
	// If we have more than one item then we'll try to find a prefix that
	// the module names all have in common, because we commonly end up
	// in this situation with families of modules like golang.org/x/* where
	// the maintainers tend to ratchet their cross-dependencies all together.
	// We expect clusters to have small numbers of members and so this is
	// a relatively naive "longest common prefix" implementation that isn't
	// concerned with performance.
	for i := 0; ; i++ {
		var first byte // we just assume a null byte cannot appear in a module path
		if len(cluster[0].Module) != i {
			first = cluster[0].Module[i]
		}
		for _, other := range cluster[1:] {
			if len(other.Module) == i || other.Module[i] != first {
				if i == 0 {
					// There's no common prefix at all
					return "(set of modules with no common name prefix)"
				}
				return "`" + string(cluster[0].Module[:i]) + "...` family"
			}
		}
	}
}

// changelogURL makes a best effort to build a URL for a page containing a
// summary of the changes made to a module between the given current and
// latest versions. Returns an empty string if no URL is available.
//
// This is best-effort because there is no universal way to map a Go module
// path to a summary of changes: this relies on some known conventions for
// how the Go toolchain handles modules hosted on github.com and on GitHub's
// URL schemes for comparing tags.
//
// This is also imprecise because GitHub's compare view only works on a
// whole-tree basis and so cannot filter by only changes in a particular
// module's scope. This works okay for repositories that consist mainly of
// one large module at the root, but will be confusing for repositories
// containing many different modules. To minimize that confusion this refuses
// to generate a comparison URL for any module that isn't at the root of
// a GitHub repository.
func changelogURL(module ModulePath, current, latest Version) string {
	if current.Major == 0 && current.Minor == 0 && current.Patch == 0 {
		// We assume that zero-versions are untagged prereleases and so
		// we can't generate comparison URLs for them.
		return ""
	}

	var githubRepo string
	parts := strings.Split(string(module), "/")
	if len(parts) == 3 && parts[0] == "github.com" {
		githubRepo = "https://" + parts[0] + "/" + parts[1] + "/" + parts[2]
	} else if len(parts) == 3 && parts[0] == "golang.org" && parts[1] == "x" {
		// The golang.org/x/... modules currently follow a predictable mapping
		// scheme to GitHub repositories. (This might not be true forever.)
		githubRepo = "https://github.com/golang/" + parts[2]
	}
	if githubRepo == "" {
		// Nothing we can do, then.
		return ""
	}

	return fmt.Sprintf("%s/compare/v%s...v%s", githubRepo, current.String(), latest.String())
}
