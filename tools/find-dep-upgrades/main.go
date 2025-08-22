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
	// one module at a time. Since this whole thing is a best-effort process
	// anyway, we'll just heuristically prune edges from the graph until we
	// reach a true acyclic graph, which will still give us at least _some_
	// guidance about what order we might approach upgrades in.
	//
	// For example, the golang.org/x/* family of modules tends to often get
	// caught up in cycles here because they love to ratchet up the requirements
	// between them periodically regardless of whether an upgrade is actually
	// needed for the functionality of the source of the dependency. It's
	// often impossible to upgrade any one of them without also upgrading
	// at least one other.
	for {
		cycles := g.Cycles()
		if len(cycles) == 0 {
			break // We've pruned enough to proceed!
		}
		for _, cycle := range cycles {
			// Our heuristic here is to prune an edge starting at the node
			// with the smallest number of total dependencies, so that we're
			// hopefully minimizing the number of forced-coupled-upgrades
			// this change introduces.
			slices.SortFunc(cycle, func(a, b dag.Vertex) int {
				// dag.Graph doesn't have a method to get the number of
				// outgoing edges from a vertex without building a slice
				// of the edges, so this is pretty wasteful but we're
				// not going to modify package dag just for this ancillary
				// tool, and our graphs here will always be small.
				aDeps := len(g.EdgesFrom(a))
				bDeps := len(g.EdgesFrom(b))
				if aDeps == bDeps {
					return 0
				}
				if aDeps < bDeps {
					return -1
				}
				return 1
			})
			// We're going to prune an edge whose source is now the first
			// vertex in the sorted "cycle". The remainder are all candidates
			// to be the destination, in order of preference; we'll choose
			// the one with the lowest number of outgoing edges that is already
			// connected to source.
			source := cycle[0]
			var deleteEdge dag.Edge
			for _, dest := range cycle[1:] {
				candidateEdge := dag.BasicEdge(source, dest)
				if g.HasEdge(candidateEdge) {
					deleteEdge = candidateEdge
					break
				}
			}
			if deleteEdge == nil {
				// We shouldn't get here because this suggests that the reported
				// cycle wasn't actually a cycle after all?!
				log.Fatalf("can't find edge to delete to resolve cycle between %s", cycle)
			}
			g.RemoveEdge(deleteEdge)
		}
	}

	order := g.TopologicalOrder()
	pendingUpgrades := make([]PendingUpgrade, 0, len(order))
	for _, v := range slices.Backward(order) {
		modulePath := v.(ModulePath)
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
		pendingUpgrades = append(pendingUpgrades, pending)
	}

	// We want a topological-ish order for the items that have prerequisites,
	// but we'll pull all of the ones without any prerequisites at all to
	// the top of the list because they can always go first.
	slices.SortStableFunc(pendingUpgrades, func(a, b PendingUpgrade) int {
		if len(a.Prereqs) != 0 && len(b.Prereqs) != 0 {
			return 0
		}
		if len(a.Prereqs) == 0 && len(b.Prereqs) == 0 {
			// Within the set of no-prereq modules we'll order them lexically,
			// because we have no particular preference order.
			return cmp.Compare(a.Module, b.Module)
		}
		if len(a.Prereqs) == 0 {
			return -1
		}
		// Otherwise, b.Prereqs must be empty.
		return 1
	})
	seenPrereqs := false
	for _, pending := range pendingUpgrades {
		if !seenPrereqs && len(pending.Prereqs) != 0 {
			// We'll include a horizontal rule between the isolated upgrades
			// and those which have prerequisites just because that makes it
			// a little easier to scan the list and focus on the easy cases
			// first.
			seenPrereqs = true
			fmt.Print("\n---\n\n")
		}

		changesURL := changelogURL(pending.Module, pending.CurrentVersion, pending.LatestVersion)
		if changesURL != "" {
			fmt.Printf("- [ ] `go get %s@v%s` ([from `v%s`](%s))\n", pending.Module, pending.LatestVersion, pending.CurrentVersion, changesURL)
		} else {
			fmt.Printf("- [ ] `go get %s@v%s` (from `v%s`)\n", pending.Module, pending.LatestVersion, pending.CurrentVersion)
		}
		prereqs := slices.Collect(maps.Keys(pending.Prereqs))
		slices.Sort(prereqs)
		for _, depModulePath := range prereqs {
			fmt.Printf("  - requires `%s@v%s`\n", depModulePath, pending.Prereqs[depModulePath])
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
