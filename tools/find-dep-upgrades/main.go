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
	"fmt"
	"log"
	"os"
	"slices"

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
	for _, v := range slices.Backward(order) {
		modulePath := v.(ModulePath)
		candidate := candidates[modulePath]
		fmt.Printf("- [ ] `go get %s@v%s` (currently `v%s`)\n", modulePath, candidate.LatestVersion, candidate.CurrentVersion)
		for depModulePath, depNewVersion := range latestVersionDeps[modulePath] {
			depCandidate, ok := candidates[depModulePath]
			if !ok {
				continue
			}
			if depCandidate.CurrentVersion.LessThan(depNewVersion) {
				fmt.Printf("  - requires `%s@v%s`\n", depModulePath, depNewVersion)
			}
		}
	}
}
