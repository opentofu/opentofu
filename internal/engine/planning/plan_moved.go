// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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

type prevStateInfo struct {
	From, To     addrs.AbsResourceInstance
	State        *states.ResourceInstanceObjectFullSrc
	Moved        bool
	ImplicitMove bool
}

type moveStep struct {
	From      addrs.AbsResourceInstance
	To        *moveStep
	Statement refactoring.MoveStatement
}

func (m *moveStep) FinalTo() addrs.AbsResourceInstance {
	if m.To == nil {
		return m.From
	}
	return m.To.FinalTo()
}

func (p *planGlue) locateMovesFor(ctx context.Context, addr addrs.AbsResourceInstance, forward bool, implicit func(addr addrs.AbsResourceInstance) *refactoring.MoveStatement) ([]*moveStep, tfdiags.Diagnostics) {
	// Build simple lookup for move statements that have spidering traversals
	// TODO replace with with resource config meta (tricky with orphans)
	moveStatementsCache := map[string][]refactoring.MoveStatement{}
	getMoveStatementsFor := func(addr addrs.AbsResourceInstance, isImplicit bool) []refactoring.MoveStatement {
		mod := addr.Module.Module()
		key := mod.String()
		statements, ok := moveStatementsCache[key]
		if !ok {
			statements = p.oracle.MoveStatementsFor(ctx, mod)
			moveStatementsCache[key] = statements
		}
		if !isImplicit {
			iMove := implicit(addr)
			if iMove != nil {
				// if concurrency is ever introduced here, this slice manipulation is inherently unsafe
				statements = append(statements, *iMove)
			}
		}
		return statements
	}

	var diags tfdiags.Diagnostics
	var ret []*moveStep
	potentialAddresses := addrs.MakeMap[addrs.AbsResourceInstance, *moveStep]()

	// We need ret and potentialAddresses to be distinct as list ordering matters
	addStep := func(move *moveStep) {
		ret = append(ret, move)
		potentialAddresses.Put(move.From, move)
	}

	currentIteration := addrs.MakeSet[addrs.AbsResourceInstance]()
	nextIteration := addrs.MakeSet[addrs.AbsResourceInstance]()

	// Start with the initial address
	currentIteration.Add(addr)
	addStep(&moveStep{From: addr})

	for len(currentIteration) > 0 {
		for _, addr := range currentIteration {
			lastMove := potentialAddresses.Get(addr)
			for _, move := range getMoveStatementsFor(addr, lastMove.Statement.Implied) {
				to, from := move.To, move.From
				if !forward {
					to, from = from, to
				}
				if prevAddr, moved := addr.MoveDestination(to, from); moved {
					if prevAddr.Equal(addr) {
						if !move.Implied {
							// TODO Move this diagnostic out of locateMovesFor
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

						for step := potentialAddresses.Get(addr); step != nil; step = step.To {
							// move statement graph nodes are pointers to move statements
							stmt := step.Statement
							stmtStrs = append(stmtStrs, fmt.Sprintf(
								"\n  - %s: %s → %s",
								stmt.DeclRange.StartString(),
								stmt.From.String(),
								stmt.To.String(),
							))

							if step.From.Equal(prevAddr) {
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
						From:      prevAddr,
						To:        lastMove,
						Statement: move,
					})
					nextIteration.Add(prevAddr)
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

func (p *planGlue) LocatePreviousState(ctx context.Context, addr addrs.AbsResourceInstance) (prevStateInfo, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Printf("[TRACE] LocateMove for %s", addr)

	// Start without state
	ret := prevStateInfo{
		From: addr,
		To:   addr,
	}

	potentialMoves, moveDiags := p.locateMovesFor(ctx, addr, true, func(addr addrs.AbsResourceInstance) *refactoring.MoveStatement {
		implicitAddr := p.planCtx.DetectImplicitStateMoveForAddress(addr)
		if implicitAddr == nil {
			return nil
		}
		log.Printf("[TRACE] ImplicitMove with state %s -> %s", *implicitAddr, addr)
		var approxSrcRange tfdiags.SourceRange // TODO
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

	type statefulMove struct {
		*moveStep
		state *states.ResourceInstanceObjectFullSrc
	}

	var statefulMoves []statefulMove

	for _, move := range potentialMoves {
		state := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(move.From.CurrentObject())
		if state != nil {
			log.Printf("[TRACE] PotentialMove with state %s -> %s", move.From, ret.To)
			statefulMoves = append(statefulMoves, statefulMove{move, state})

			// Record move statements for later analysis
			p.planCtx.moveMu.Lock()
			p.planCtx.configuredMoves.Put(move.From, append(p.planCtx.configuredMoves.Get(move.From), move))
			p.planCtx.moveMu.Unlock()
		} else {
			log.Printf("[TRACE] PotentialMove (no state) %s -> %s", move.From, ret.To)
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
	move := statefulMoves[0]
	ret.From = move.From
	ret.State = move.state
	ret.Moved = move.To != nil
	ret.ImplicitMove = move.Statement.Implied

	if ret.Moved {
		p.planCtx.moveMu.Lock()
		// Record all moves that are part of this chain
		for step := move.moveStep; step != nil; step = step.To {
			p.planCtx.recordedMoves.Put(step.From, step.To)
		}
		p.planCtx.moveMu.Unlock()
	}

	return ret, diags
}

func (p *planGlue) LocateExecutedMove(addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	step, ok := p.planCtx.recordedMoves.GetOk(addr)
	if ok {
		return new(step.FinalTo())
	}
	return nil
}

func (p *planGlue) LocateUnexecutedMove(ctx context.Context, addr addrs.AbsResourceInstance) (*addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	log.Printf("[TRACE] LocateUnexecutedMove for %s", addr)

	if p.planCtx.configuredMoves.Has(addr) {
		// Move was configured but not executed for some reason and will be reported as an error elsewhere
		return nil, nil
	}

	potentialMoves, diags := p.locateMovesFor(ctx, addr, false, func(addr addrs.AbsResourceInstance) *refactoring.MoveStatement {
		implicitAddr := p.oracle.DetectImplicitMoveForAddress(ctx, addr)
		if implicitAddr == nil {
			return nil
		}
		log.Printf("[TRACE] ImplicitMove %s -> %s", *implicitAddr, addr)
		var approxSrcRange tfdiags.SourceRange // TODO
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
	/*for _, move := range potentialMoves {
		// Record move statements for later analysis
		p.planCtx.moveMu.Lock()
		p.planCtx.configuredMoves.Put(move.From, append(p.planCtx.configuredMoves.Get(move.From), move))
		p.planCtx.moveMu.Unlock()
	}*/

	// This is inverted so From is the correct field
	return new(potentialMoves[0].From), diags
}

func (p *planContext) ConflictingMoveDiags() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// TODO Redundant move blocks? (see above)

	// Detect moves from the same address
	for _, moves := range p.configuredMoves.Values() {
		// TODO we need to find a way to stable sort moves to ensure self moves always come first
		// this hack works for now
		first := moves[0]
		for _, stepTaken := range moves {
			if stepTaken.To == nil {
				first = stepTaken
				break
			}
		}
		for _, stepTaken := range moves {
			if stepTaken == first {
				continue
			}
			if first.To == nil || first.Statement.Implied {
				move := stepTaken.Statement
				absFrom := move.From.InModuleInstance(first.From.Module)
				absTo := move.To.InModuleInstance(first.From.Module)
				noun := absFrom.Noun()
				shortNoun := absFrom.ShortNoun()

				// TODO determine DeclRange, probably by adding some config information from the caller
				declaredAt := ""

				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Moved object still exists",
					Detail: fmt.Sprintf(
						"This statement declares a move from %s, but that %s is still declared%s.\n\nChange your configuration so that this %s will be declared as %s instead.",
						absFrom, noun, declaredAt, shortNoun, absTo,
					),
					Subject: move.DeclRange.ToHCL().Ptr(),
				})
			} else {
				log.Printf("[TRACE] AmbiguousMove: %s -> (%s||%s)", first.From, first.To.From, stepTaken.To.From)

				absFrom := first.Statement.From.InModuleInstance(first.From.Module)
				absTo := first.Statement.To.InModuleInstance(first.From.Module)
				// This diag input is wrong, but fires in the correct circumstances
				absOtherTo := stepTaken.Statement.To.InModuleInstance(stepTaken.From.Module)

				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Ambiguous move statements",
					Detail: fmt.Sprintf(
						"A statement at %s declared that %s moved to %s, but this statement instead declares that it moved to %s.\n\nEach %s can move to only one destination %s.",
						first.Statement.DeclRange.StartString(), absFrom, absTo, absOtherTo,
						absFrom.Noun(), absFrom.ShortNoun(),
					),
					Subject: stepTaken.Statement.DeclRange.ToHCL().Ptr(),
				})
			}
		}
	}

	// Build up inverse map (could probably do this inline elsewhere, though this is probably more efficient)
	movesTo := addrs.MakeMap[addrs.AbsResourceInstance, []*moveStep]()
	for _, moves := range p.configuredMoves.Values() {
		for _, move := range moves {
			if move.To == nil {
				// Self moves don't count
				continue
			}
			movesTo.Put(move.To.From, append(movesTo.Get(move.To.From), move))
		}
	}

	blocked := addrs.MakeMap[addrs.AbsResourceInstance, addrs.AbsResourceInstance]()

	for _, moves := range movesTo.Values() {
		// This may need to be tweaked a bit, it's relying on the moved.To filter above
		// See check in previous loop about To/Implicit
		if p.prevRoundState.ResourceInstance(moves[0].To.From) != nil {
			for _, move := range moves {
				log.Printf("[TRACE] BlockedMove: (%s || %s) -> %s", move.From, move.To.From, move.To.From)
				blocked.Put(move.From, move.To.From)
			}
		} else if len(moves) >= 2 {
			for i, move := range moves {
				stepTaken := moves[(i+1)%len(moves)]

				log.Printf("[TRACE] AmbiguousMove: (%s || %s) -> %s", move.From, stepTaken.From, stepTaken.To.From)
				absFrom := move.Statement.From.InModuleInstance(move.From.Module)
				absTo := move.Statement.To.InModuleInstance(move.From.Module)
				absOtherFrom := stepTaken.Statement.From.InModuleInstance(stepTaken.From.Module)

				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Ambiguous move statements",
					Detail: fmt.Sprintf(
						"A statement at %s declared that %s moved to %s, but this statement instead declares that %s moved there.\n\nEach %s can have moved from only one source %s.",
						move.Statement.DeclRange.StartString(), absFrom, absTo, absOtherFrom,
						absFrom.Noun(), absFrom.ShortNoun(),
					),
					Subject: stepTaken.Statement.DeclRange.ToHCL().Ptr(),
				})
			}
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

func (p *planContext) DetectImplicitStateMoveForAddress(addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	var currentStep []addrs.ModuleInstance
	var nextStep []addrs.ModuleInstance

	// TODO this could be much more efficient with a recursive iter pattern (steal from a previous commit on this branch)

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
