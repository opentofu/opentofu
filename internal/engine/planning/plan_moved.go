// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Origin: This file is a menagerie of the contents of the refactoring package,
// as well as a previous implementation built during the engine rewrite.

package planning

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// moveStep represents a step in the path a resource's
// state might take in it's journey to a new address.
// This is designed as a singly linked list and allows
// from a potential state location to a
// final proposed destination.
type moveStep struct {
	from      addrs.AbsResourceInstance
	to        *moveStep
	statement refactoring.MoveStatement
}

// destination is the final location that this chain of moves
// points to.
func (m *moveStep) destination() addrs.AbsResourceInstance {
	if m.to == nil {
		return m.from
	}
	return m.to.destination()
}

// isMove determines if this step actually represents a state move,
// or just a nop.
func (m *moveStep) isMove() bool {
	return m.to != nil
}

// isImplicit determines if this step is implicit or not. It does
// not check down the linked-list as implicits can only exist at
// the first element in the linked list.
func (m *moveStep) isImplicit() bool {
	return m.statement.Implied
}

// locateMovesFor starts at a given address and works "backwards" to discover any potential moveSteps that could contribute to
// this sub-move graph.  Cycles are detected and rejected, which effectively produces a flattened tree of moveSteps.
// If configToState == false, iteration direction is reversed to allow for reverse lookups from state -> config
// There are a bunch of different algostructures and datarythms that would make sense here, this one made the most sense
// to me during initial implementation and seems to be reasonably performant
func (p *planGlue) locateMovesFor(ctx context.Context, addr addrs.AbsResourceInstance, configToState bool, implicit func(addr addrs.AbsResourceInstance) *refactoring.MoveStatement) ([]*moveStep, tfdiags.Diagnostics) {
	// Build simple lookup for move statements that have spidering traversals
	// TODO this cache could live in planContext
	moveStatementsCache := map[string][]refactoring.MoveStatement{}
	getMoveStatementsFor := func(addr addrs.AbsResourceInstance) []refactoring.MoveStatement {
		mod := addr.Module.Module()
		key := mod.String()
		statements, ok := moveStatementsCache[key]
		if !ok {
			// TODO replace with with resource config meta (tricky with orphans)
			statements = p.oracle.MoveStatementsFor(ctx, mod)
			moveStatementsCache[key] = statements
		}
		iMove := implicit(addr)
		if iMove != nil {
			// if concurrency is ever introduced here, this slice manipulation is inherently unsafe
			statements = append(statements, *iMove)
		}
		return statements
	}

	var diags tfdiags.Diagnostics
	var ret []*moveStep
	potentialAddresses := addrs.MakeMap[addrs.AbsResourceInstance, *moveStep]()

	// We need ret and potentialAddresses to be distinct as list ordering matters
	addStep := func(move *moveStep) {
		ret = append(ret, move)
		potentialAddresses.Put(move.from, move)
	}

	currentIteration := addrs.MakeSet[addrs.AbsResourceInstance]()
	nextIteration := addrs.MakeSet[addrs.AbsResourceInstance]()

	// Start with the initial address
	currentIteration.Add(addr)
	addStep(&moveStep{from: addr})

	for len(currentIteration) > 0 {
		for _, addr := range currentIteration {
			lastMove := potentialAddresses.Get(addr)
			for _, move := range getMoveStatementsFor(addr) {
				from, to := move.From, move.To
				if configToState {
					to, from = from, to
				}
				if prevAddr, moved := addr.MoveDestination(from, to); moved {
					if prevAddr.Equal(addr) {
						if !move.Implied {
							diags = diags.Append(&hcl.Diagnostic{
								Severity: hcl.DiagError,
								Summary:  "Redundant move statement",
								Detail: fmt.Sprintf(
									"This statement declares a move from %s to the same address, which is the same as not declaring this move at all.",
									prevAddr,
								),
								Subject: move.DeclRange.ToHCL().Ptr(),
							})
						}
						continue
					}

					_, ok := potentialAddresses.GetOk(prevAddr)
					if ok {
						// Detect if this is a duplicate path or a true cycle

						// Reporting cycles is awkward because there isn't any definitive
						// way to decide which of the objects in the cycle is the cause of
						// the problem. Therefore we'll just list them all out and leave
						// the user to figure it out. :(
						var stmtStrs []string

						for step := potentialAddresses.Get(addr); step != nil; step = step.to {
							// move statement graph nodes are pointers to move statements
							stmt := step.statement
							stmtStrs = append(stmtStrs, fmt.Sprintf(
								"\n  - %s: %s → %s",
								stmt.DeclRange.StartString(),
								stmt.From.String(),
								stmt.To.String(),
							))

							if step.from.Equal(prevAddr) {
								sort.Strings(stmtStrs) // just to make the order deterministic

								diags = diags.Append(tfdiags.Sourceless(
									tfdiags.Error,
									"Cyclic dependency in move statements",
									fmt.Sprintf(
										"The following chained move statements form a cycle, and so there is no final location to move objects to:%s\n\nA chain of move statements must end with an address that doesn't appear in any other statements, and which typically also refers to an object still declared in the configuration.",
										strings.Join(stmtStrs, ""),
									),
								))
								break
							}
						}
						continue
					}

					addStep(&moveStep{
						from:      prevAddr,
						to:        lastMove,
						statement: move,
					})
					if !move.Implied {
						nextIteration.Add(prevAddr)
					}
				}
			}
		}

		// Swap current and next
		currentIteration, nextIteration = nextIteration, currentIteration
		// Clear next
		clear(nextIteration)
	}

	return ret, diags
}

// moveWithState represents a resource state entry paired
// with a moveChain that describes it's start and end location.
type moveWithState struct {
	*moveStep
	state *states.ResourceInstanceObjectFullSrc
}

// locateStateForConfig takes a desired resource instance address and locates it's previous location in state
// This function has the side effect of populating planContext's discovered moves for later validation.
func (p *planGlue) locateStateForConfig(ctx context.Context, addr addrs.AbsResourceInstance) (moveWithState, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Printf("[TRACE] LocateStateForConfig %s", addr)

	// Start a nop and no state
	ret := moveWithState{moveStep: &moveStep{from: addr}}

	potentialMoves, moveDiags := p.locateMovesFor(ctx, addr, true, func(addr addrs.AbsResourceInstance) *refactoring.MoveStatement {
		implicitAddr := p.planCtx.detectImplicitStateMoveForAddress(addr)
		if implicitAddr == nil {
			return nil
		}
		log.Printf("[TRACE] ImplicitMove with state %s -> %s", *implicitAddr, addr)
		var approxSrcRange tfdiags.SourceRange
		// FIXME: Decide how exactly we're going to deal with DeclRange in
		// meta now that it's a slice of opinions from different modules
		// instead of a single object. Probably we should standardize on only
		// using the overall effective (merged) metadata in the planning engine
		// anyway, and so maybe this becomes a moot point.
		/*
			meta := p.oracle.ResourceInstanceObjectMeta(ctx, implicitAddr.CurrentObject())
			if meta != nil {
				approxSrcRange = meta.DeclRange
			}
		*/
		return &refactoring.MoveStatement{
			From:      addrs.ImpliedMoveStatementEndpoint(*implicitAddr, approxSrcRange),
			To:        addrs.ImpliedMoveStatementEndpoint(addr, approxSrcRange),
			Implied:   true,
			DeclRange: approxSrcRange,
		}
	})
	diags = diags.Append(moveDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	var statefulMoves []moveWithState

	for _, move := range potentialMoves {
		state := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(move.from.CurrentObject())
		if state != nil {
			log.Printf("[TRACE] PotentialMove with state %s -> %s", move.from, addr)
			statefulMoves = append(statefulMoves, moveWithState{move, state})

			// Record move statements for later analysis
			p.planCtx.moveMu.Lock()
			p.planCtx.configuredMoves.Put(move.from, append(p.planCtx.configuredMoves.Get(move.from), move))
			p.planCtx.moveMu.Unlock()
		} else {
			log.Printf("[TRACE] PotentialMove (no state) %s -> %s", move.from, addr)
		}
	}

	if len(statefulMoves) == 0 {
		log.Printf("[TRACE] NoPotentialMoves with state for %s", addr)
		return ret, diags
	}

	// Assertions:
	// * NOP move (self -> self) detection is always first (if state exists)
	// * Implicit moves are always last in a chain
	// * Order is deterministic
	ret = statefulMoves[0]

	if ret.isMove() {
		log.Printf("[TRACE] ExecutedMove %s -> %s", ret.from, addr)
		p.planCtx.moveMu.Lock()
		p.planCtx.recordedMoves.Put(ret.from, addr)
		p.planCtx.moveMu.Unlock()
	}

	return ret, diags
}

// locateExecutedMove returns the address a state entry has already been recorded as being moved to.
func (p *planGlue) locateExecutedMove(addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	destination, ok := p.planCtx.recordedMoves.GetOk(addr)
	if ok {
		return new(destination)
	}
	return nil
}

// locateConfigForState returns the address that a state resource instance may have been moved to.
// This is primarily used for locating config meta for orphaned and deposed state entries.
func (p *planGlue) locateConfigForState(ctx context.Context, addr addrs.AbsResourceInstance, isCurrent bool) (*addrs.AbsResourceInstance, tfdiags.Diagnostics) {

	log.Printf("[TRACE] LocateConfigForState %s", addr)

	p.planCtx.moveMu.Lock()
	hasConfiguredMove := p.planCtx.configuredMoves.Has(addr)
	p.planCtx.moveMu.Unlock()
	if hasConfiguredMove {
		// Move was configured but not executed for some reason and will be reported as an error elsewhere
		return nil, nil
	}

	potentialMoves, diags := p.locateMovesFor(ctx, addr, false, func(addr addrs.AbsResourceInstance) *refactoring.MoveStatement {
		implicitAddr := p.oracle.DetectImplicitMoveForAddress(ctx, addr)
		if implicitAddr == nil {
			return nil
		}
		log.Printf("[TRACE] ImplicitMove %s -> %s", *implicitAddr, addr)
		var approxSrcRange tfdiags.SourceRange
		// FIXME: Decide how exactly we're going to deal with DeclRange in
		// meta now that it's a slice of opinions from different modules
		// instead of a single object. Probably we should standardize on only
		// using the overall effective (merged) metadata in the planning engine
		// anyway, and so maybe this becomes a moot point.
		/*
			meta := p.oracle.ResourceInstanceObjectMeta(ctx, implicitAddr.CurrentObject())
			if meta != nil {
				approxSrcRange = meta.DeclRange
			}
		*/
		return &refactoring.MoveStatement{
			To:        addrs.ImpliedMoveStatementEndpoint(*implicitAddr, approxSrcRange),
			From:      addrs.ImpliedMoveStatementEndpoint(addr, approxSrcRange),
			Implied:   true,
			DeclRange: approxSrcRange,
		}
	})
	if diags.HasErrors() {
		return nil, diags
	}

	// First entry is always the self move (nop here)
	potentialMoves = potentialMoves[1:]

	if len(potentialMoves) == 0 {
		return nil, diags
	}

	// TODO I'm not sure if this is correct or even tested for the old engine
	// Part of the concern is orphaned and deposed entries hitting this multiple times and causing false positives in validation below
	if isCurrent {
		for _, move := range potentialMoves {
			// Record move statements for later analysis
			p.planCtx.moveMu.Lock()
			p.planCtx.configuredMoves.Put(move.from, append(p.planCtx.configuredMoves.Get(move.from), move))
			p.planCtx.moveMu.Unlock()
		}
	}

	// This ensures that orphaned resources that are excluded and moved
	// hit the appropriate error conditions.
	p.planCtx.moveMu.Lock()
	p.planCtx.recordedMoves.Put(addr, potentialMoves[0].from)
	p.planCtx.moveMu.Unlock()

	// This is inverted so From is the correct field
	return new(potentialMoves[0].from), diags
}

// validateMoves inspects all potential moves discovered during the planning process
// and reports and potential issues discovered.
func (p *planGlue) validateMoves(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	p.planCtx.moveMu.Lock()
	defer p.planCtx.moveMu.Unlock()

	// TODO this operates on addrs.AbsResourceInstance, not addrs.MoveEndpoint and will produce a lot more
	// unique diagnostics if a large module with many resource instances is moved incorrectly. It is
	// unclear if this is an issue that needs to be addressed or not.

	for _, moves := range p.planCtx.configuredMoves.Values() {
		var nopMove *moveStep
		filteredMoves := make([]*moveStep, 0, len(moves))
		for _, move := range moves {
			isNop := move.to == nil
			isImplicitNop := move.statement.Implied && move.to.to == nil
			if isNop || isImplicitNop {
				if nopMove != nil {
					panic("Assertion: there should only ever be one nopMove")
				}
				nopMove = move
			} else {
				log.Printf("[TRACE] CheckingMoveFrom %s --> %s", move.from.String(), move.destination().String())
				filteredMoves = append(filteredMoves, move)
			}
		}
		moves = filteredMoves

		if nopMove != nil {
			// It's invalid to have a move statement whose "from" address
			// refers to something that is still declared in the configuration.
			for _, stepTaken := range moves {
				move := stepTaken.statement
				absFrom := move.From.InModuleInstance(nopMove.from.Module)
				absTo := move.To.InModuleInstance(nopMove.from.Module)
				noun := absFrom.Noun()
				shortNoun := absFrom.ShortNoun()

				declaredAt := "TODO"
				// FIXME: Decide how exactly we're going to deal with DeclRange in
				// meta now that it's a slice of opinions from different modules
				// instead of a single object. Probably we should standardize on only
				// using the overall effective (merged) metadata in the planning engine
				// anyway, and so maybe this becomes a moot point.
				/*
					meta := p.oracle.ResourceInstanceObjectMeta(ctx, nopMove.from.CurrentObject())
					if meta != nil {
						// NOTE: It'd be pretty weird to _not_ have a range, since
						// we're only in this codepath because the plan phase
						// thought this object existed in the configuration.
						declaredAt = fmt.Sprintf(" at %s", meta.DeclRange.StartString())
					}
				*/

				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Moved object still exists",
					Detail: fmt.Sprintf(
						"This statement declares a move from %s, but that %s is still declared%s.\n\nChange your configuration so that this %s will be declared as %s instead.",
						absFrom, noun, declaredAt, shortNoun, absTo,
					),
					Subject: move.DeclRange.ToHCL().Ptr(),
				})
			}
		}

		if len(moves) >= 2 {
			// There can only be one destination for each source address.
			for i, stepTaken := range moves {
				first := moves[(i+1)%len(moves)]
				log.Printf("[TRACE] AmbiguousMove: %s -> (%s||%s)", first.from, first.to.from, stepTaken.to.from)

				absFrom := first.statement.From.InModuleInstance(first.from.Module)
				absTo := first.statement.To.InModuleInstance(first.from.Module)
				// This diag input is wrong, but fires in the correct circumstances
				absOtherTo := stepTaken.statement.To.InModuleInstance(stepTaken.from.Module)

				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Ambiguous move statements",
					Detail: fmt.Sprintf(
						"A statement at %s declared that %s moved to %s, but this statement instead declares that it moved to %s.\n\nEach %s can move to only one destination %s.",
						first.statement.DeclRange.StartString(), absFrom, absTo, absOtherTo,
						absFrom.Noun(), absFrom.ShortNoun(),
					),
					Subject: stepTaken.statement.DeclRange.ToHCL().Ptr(),
				})
			}
		}
	}

	// Build up inverse map (could probably do this inline elsewhere, though this is probably more efficient)
	movesTo := addrs.MakeMap[addrs.AbsResourceInstance, []*moveStep]()
	for _, moves := range p.planCtx.configuredMoves.Values() {
		for _, move := range moves {
			key := move.from // Nop move is from -> from
			if move.to != nil {
				key = move.to.from
			}
			movesTo.Put(key, append(movesTo.Get(key), move))
		}
	}

	blocked := addrs.MakeMap[addrs.AbsResourceInstance, addrs.AbsResourceInstance]()

	for _, elem := range movesTo.Elements() {
		_, moves := elem.Key, elem.Value

		// Determine if there already exists state at this address
		var nopMove *moveStep
		filteredMoves := make([]*moveStep, 0, len(moves))
		for _, move := range moves {
			isNop := move.to == nil
			if isNop {
				if nopMove != nil {
					panic("Assertion: there should only ever be one nopMove")
				}
				nopMove = move
			} else {
				log.Printf("[TRACE] CheckingMoveTo %s <-- %s", move.destination().String(), move.from.String())
				filteredMoves = append(filteredMoves, move)
			}
		}
		moves = filteredMoves

		if nopMove != nil {
			// If we have a nopMove (state already exists at the desired location)
			for _, move := range moves {
				log.Printf("[TRACE] BlockedMove: (%s || %s) -> %s", move.from, move.to.from, move.to.from)
				blocked.Put(move.from, move.to.from)
			}
		}

		// Filter out implicit moves, we only care about explicit moves into this address.  An implicit move will have a state move or be blocked by a nop move.
		filteredMoves = make([]*moveStep, 0, len(moves))
		for _, move := range moves {
			if !move.statement.Implied {
				filteredMoves = append(filteredMoves, move)
			}
		}
		moves = filteredMoves

		if len(moves) >= 2 {
			// There can only be one source for each destination address.
			for i, move := range moves {
				stepTaken := moves[(i+1)%len(moves)]

				log.Printf("[TRACE] AmbiguousMove: (%s || %s) -> %s", move.from, stepTaken.from, stepTaken.to.from)
				absFrom := move.statement.From.InModuleInstance(move.from.Module)
				absTo := move.statement.To.InModuleInstance(move.from.Module)
				absOtherFrom := stepTaken.statement.From.InModuleInstance(stepTaken.from.Module)

				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Ambiguous move statements",
					Detail: fmt.Sprintf(
						"A statement at %s declared that %s moved to %s, but this statement instead declares that %s moved there.\n\nEach %s can have moved from only one source %s.",
						move.statement.DeclRange.StartString(), absFrom, absTo, absOtherFrom,
						absFrom.Noun(), absFrom.ShortNoun(),
					),
					Subject: stepTaken.statement.DeclRange.ToHCL().Ptr(),
				})
			}
		}
	}

	// Check for addresses not included in target or are excluded
	isTargeting := p.isTargeting()
	isExcluding := p.isExcluding()
	if isTargeting || isExcluding {
		allEntries := addrs.MakeSet[addrs.AbsResourceInstance]()
		for _, elem := range p.planCtx.recordedMoves.Elements() {
			allEntries.Add(elem.Key)
			allEntries.Add(elem.Value)
		}
		var excluded []addrs.AbsResourceInstance
		for addr := range allEntries.All() {
			if !p.isTargeted(addr) || p.isExcluded(addr) {
				excluded = append(excluded, addr)
			}
		}

		if len(excluded) > 0 {
			sort.Slice(excluded, func(i, j int) bool {
				return excluded[i].Less(excluded[j])
			})

			var flag string
			if isTargeting {
				flag = "-target"
			} else {
				flag = "-exclude"
			}

			var listBuf strings.Builder
			var prevResourceAddr addrs.AbsResource
			for _, instAddr := range excluded {
				// Targeting generally ends up selecting whole resources rather
				// than individual instances, because we don't factor in
				// individual instances until DynamicExpand, so we're going to
				// always show whole resource addresses here, excluding any
				// instance keys. (This also neatly avoids dealing with the
				// different quoting styles required for string instance keys
				// on different shells, which is handy.)
				//
				// To avoid showing duplicates when we have multiple instances
				// of the same resource, we'll remember the most recent
				// resource we rendered in prevResource, which is sufficient
				// because we sorted the list of instance addresses above, and
				// our sort order always groups together instances of the same
				// resource.
				resourceAddr := instAddr.ContainingResource()
				if resourceAddr.Equal(prevResourceAddr) {
					continue
				}
				fmt.Fprintf(&listBuf, "\n  %s=%q", flag, resourceAddr.String())
				prevResourceAddr = resourceAddr
			}

			var msg string
			if isTargeting {
				msg = fmt.Sprintf(
					"Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -target=... options do not fully cover all of those resource instances.\n\nTo create a valid plan, either remove your -target=... options altogether or add the following additional target options:%s\n\nNote that adding these options may include further additional resource instances in your plan, in order to respect object dependencies.",
					listBuf.String(),
				)
			} else {
				msg = fmt.Sprintf(
					"Resource instances in your current state have moved to new addresses in the latest configuration. OpenTofu must include those resource instances while planning in order to ensure a correct result, but your -exclude=... options exclude some of those resource instances.\n\nTo create a valid plan, either remove your -exclude=... options altogether or just specifically remove the following options:%s\n\nNote that removing these options may include further additional resource instances in your plan, in order to respect object dependencies.",
					listBuf.String(),
				)
			}

			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Moved resource instances excluded by targeting",
				msg,
			))
		}
	}

	var itemsBuf bytes.Buffer
	empty := true

	for _, blocked := range blocked.Elements() {
		fmt.Fprintf(&itemsBuf, "\n  - %s could not move to %s", blocked.Key, blocked.Value)
		empty = false
	}

	if !empty {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"Unresolved resource instance address changes",
			fmt.Sprintf(
				"OpenTofu tried to adjust resource instance addresses in the prior state based on change information recorded in the configuration, but some adjustments did not succeed due to existing objects already at the intended addresses:%s\n\nOpenTofu has planned to destroy these objects. If OpenTofu's proposed changes aren't appropriate, you must first resolve the conflicts using the \"tofu state\" subcommands and then create a new plan.",
				itemsBuf.String(),
			),
		))
	}
	return diags
}

// detectImplicitStateMoveForAddress searches the state for a potential implicit address for the given config address.
// This is not super efficient and could probably be cached/optimized in the future.
func (p *planContext) detectImplicitStateMoveForAddress(addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	var currentStep []addrs.ModuleInstance
	var nextStep []addrs.ModuleInstance

	// Start with the root module
	currentStep = append(currentStep, addrs.RootModuleInstance)
	for _, part := range addr.Module {
		for _, currentAddr := range currentStep {
			// Add alternate
			if part.InstanceKey == addrs.NoKey {
				nextStep = append(nextStep, currentAddr.Child(part.Name, addrs.IntKey(0)))
			}
			if ik, ok := part.InstanceKey.(addrs.IntKey); ok && ik == 0 {
				nextStep = append(nextStep, currentAddr.Child(part.Name, addrs.NoKey))
			}

			// Add standard
			nextStep = append(nextStep, currentAddr.Child(part.Name, part.InstanceKey))
		}
		// Swap and clear next
		currentStep, nextStep = nextStep, currentStep
		nextStep = nextStep[:0]
	}

	var modAddr addrs.ModuleInstance
	for _, modAddr = range currentStep {
		if p.prevRoundState.Module(modAddr) != nil {
			// Found it!
			break
		}
	}

	mod := p.prevRoundState.Module(modAddr)
	if mod == nil {
		return nil
	}
	resource := mod.Resource(addr.Resource.Resource)

	if resource == nil {
		return nil
	}

	// Find potential instances
	normal := addr.Resource
	invert := addr.Resource

	if invert.Key == addrs.NoKey {
		// Try NoKey -> 0
		// TODO check iteration type
		invert.Key = addrs.IntKey(0)
	} else if ik, ok := invert.Key.(addrs.IntKey); ok && ik == 0 {
		// Try 0 -> NoKey
		// TODO check iteration type
		invert.Key = addrs.NoKey
	}

	instances := resource.Instances
	if _, ok := instances[invert.Key]; ok {
		return new(resource.Addr.Instance(invert.Key))
	}
	if _, ok := instances[normal.Key]; ok {
		newAddr := resource.Addr.Instance(normal.Key)
		if !newAddr.Equal(addr) {
			return &newAddr
		}
	}

	return nil
}
