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
	"slices"
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

func (p *planGlue) locateExplicitMovesFor(ctx context.Context, addr addrs.AbsResourceInstance, forward bool) ([]*moveStep, tfdiags.Diagnostics) {
	// Build simple lookup for move statements that have spidering traversals
	moveStatementsCache := map[string][]refactoring.MoveStatement{}
	getMoveStatementsFor := func(addr addrs.AbsResourceInstance) []refactoring.MoveStatement {
		key := addr.Module.Module().String()
		statements, ok := moveStatementsCache[key]
		if !ok {
			// TODO rewrite this in terms of the module address and/or make the lookup func simpler
			statements = p.oracle.MoveStatementsFor(ctx, addr)
			moveStatementsCache[key] = statements
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
			for _, move := range getMoveStatementsFor(addr) {
				to, from := move.To, move.From
				if !forward {
					to, from = from, to
				}
				if prevAddr, moved := addr.MoveDestination(to, from); moved {
					if prevAddr.Equal(addr) {
						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Redundant move statement",
							Detail: fmt.Sprintf(
								"This statement declares a move from %s to the same address, which is the same as not declaring this move at all.",
								prevAddr,
							),
							Subject: move.DeclRange.ToHCL().Ptr(),
						})
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
						To:        potentialAddresses.Get(addr),
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

	// Start without state
	ret := prevStateInfo{
		From: addr,
		To:   addr,
	}

	potentialMoves, moveDiags := p.locateExplicitMovesFor(ctx, addr, true)
	diags = diags.Append(moveDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	// Add implicit move
	if implicitAddr := p.planCtx.DetectImplicitStateMoveForAddress(addr); implicitAddr != nil {
		// TODO We only want to use an implicit move if there is no explicit move already defined
		if todo := true; todo {
			var approxSrcRange tfdiags.SourceRange // TODO
			potentialMoves = append(potentialMoves, &moveStep{
				From: *implicitAddr,
				To:   potentialMoves[0], // NOP is always first
				Statement: refactoring.MoveStatement{
					From:      addrs.ImpliedMoveStatementEndpoint(implicitAddr, approxSrcRange),
					To:        addrs.ImpliedMoveStatementEndpoint(addr, approxSrcRange),
					Implied:   true,
					DeclRange: approxSrcRange,
				},
			})
		}
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
		} else {
			log.Printf("[TRACE] PotentialMove (no state) %s -> %s", move.From, ret.To)
		}
	}

	// Assertions:
	// * NOP move (self -> self) detection is always first (if state exists)
	// * Implicit moves are always last
	// * Order is deterministic
	var stepTaken *moveStep
	for _, move := range statefulMoves {
		if ret.State != nil {
			if ret.From.Equal(ret.To) {
				log.Printf("[TRACE] BlockedMove: (%s || %s) -> %s", move.From, ret.From, ret.To)
				p.planCtx.moveMu.Lock()
				p.planCtx.blockedMoves.Put(move.From, ret.To)
				p.planCtx.moveMu.Unlock()
			} else {
				log.Printf("[TRACE] AmbiguousMove: (%s || %s) -> %s", move.From, ret.From, ret.To)
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
			continue
		}
		ret.From = move.From
		ret.State = move.state
		ret.Moved = move.To != nil
		ret.ImplicitMove = move.Statement.Implied
		stepTaken = move.moveStep
	}

	if stepTaken != nil {
		for step := stepTaken; step.To != nil; step = step.To {
			if p.oracle.HasAddress(ctx, step.From) {
				move := step.Statement
				absFrom := move.From.InModuleInstance(addr.Module)
				absTo := move.To.InModuleInstance(addr.Module)
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
			}
		}
	}

	if !ret.To.Equal(ret.From) {
		// Active move
		p.planCtx.moveMu.Lock()
		if first, ok := p.planCtx.recordedMoves.GetOk(ret.From); ok {
			absFrom := first.Statement.From.InModuleInstance(first.From.Module)
			absTo := first.Statement.To.InModuleInstance(first.From.Module)
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
		p.planCtx.recordedMoves.Put(ret.From, stepTaken)
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
	potentialMoves, diags := p.locateExplicitMovesFor(ctx, addr, false)
	if diags.HasErrors() {
		return nil, diags
	}

	// Add implicit move
	if implicitAddr := p.oracle.DetectImplicitMoveForAddress(ctx, addr); implicitAddr != nil {
		// TODO We only want to use an implicit move if there is no explicit move already defined
		if todo := true; todo {
			var approxSrcRange tfdiags.SourceRange // TODO
			potentialMoves = append(potentialMoves, &moveStep{
				From: *implicitAddr,
				To:   potentialMoves[0], //NOP is always first
				Statement: refactoring.MoveStatement{
					From:      addrs.ImpliedMoveStatementEndpoint(implicitAddr, approxSrcRange),
					To:        addrs.ImpliedMoveStatementEndpoint(addr, approxSrcRange),
					Implied:   true,
					DeclRange: approxSrcRange,
				},
			})
		}
	}

	// Calculate the potential end addresses
	toAddresses := addrs.MakeSet[addrs.AbsResourceInstance]()

	for _, move := range potentialMoves {
		if move.From.Equal(addr) {
			// Ignore nop move
			continue
		}
		// This is inverted so From is the correct field
		toAddresses.Add(move.From)
	}

	if len(toAddresses) == 0 {
		return nil, diags
	}
	if len(toAddresses) == 1 {
		toAddr := slices.Collect(toAddresses.All())[0]
		// Check if this move was previously blocked
		if p.planCtx.blockedMoves.Has(addr) {
			return nil, diags
		}

		return &toAddr, diags
	}
	panic("TODO untested")
	return nil, diags.Append(fmt.Errorf("Multiple destinations!"))
}

func (p *planContext) BlockedMoveDiags() tfdiags.Diagnostics {
	var itemsBuf bytes.Buffer
	empty := true

	for _, blocked := range p.blockedMoves.Elements() {
		fmt.Fprintf(&itemsBuf, "\n  - %s could not move to %s", blocked.Key, blocked.Value)
		empty = false
	}

	if empty {
		return nil
	}

	return tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Warning,
		"Unresolved resource instance address changes",
		fmt.Sprintf(
			"OpenTofu tried to adjust resource instance addresses in the prior state based on change information recorded in the configuration, but some adjustments did not succeed due to existing objects already at the intended addresses:%s\n\nOpenTofu has planned to destroy these objects. If OpenTofu's proposed changes aren't appropriate, you must first resolve the conflicts using the \"tofu state\" subcommands and then create a new plan.",
			itemsBuf.String(),
		),
	)}
}

func (p *planContext) DetectImplicitStateMoveForAddress(addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
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
