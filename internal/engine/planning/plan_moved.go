// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"log"

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

func (p *planGlue) locateMovesFor(ctx context.Context, addr addrs.AbsResourceInstance, forward bool) ([]*moveStep, tfdiags.Diagnostics) {
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
					_, ok := potentialAddresses.GetOk(prevAddr)
					if ok {
						// Detect if this is a duplicate path or a true cycle
						for step := potentialAddresses.Get(addr); step != nil; step = step.To {
							if step.From.Equal(prevAddr) {
								diags = diags.Append(fmt.Errorf("CYCLE TODO"))
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

	// Add implicit entries to the graph, we only do this at the starting address
	for implicitAddr := range implicitAddrsFor(addr) {
		if potentialAddresses.Has(implicitAddr) {
			// We only want to use an implicit move if there is no explicit move already defined
			continue
		}
		var approxSrcRange tfdiags.SourceRange // TODO
		addStep(&moveStep{
			From: implicitAddr,
			To:   potentialAddresses.Get(addr),
			Statement: refactoring.MoveStatement{
				From:      addrs.ImpliedMoveStatementEndpoint(implicitAddr, approxSrcRange),
				To:        addrs.ImpliedMoveStatementEndpoint(addr, approxSrcRange),
				Implied:   true,
				DeclRange: approxSrcRange,
			},
		})
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

	potentialMoves, moveDiags := p.locateMovesFor(ctx, addr, true)
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
		} else {
			log.Printf("[TRACE] PotentialMove (no state) %s -> %s", move.From, ret.To)
		}
	}

	for _, move := range statefulMoves {
		if ret.State != nil {
			log.Printf("[TRACE] ConflictingMove: (%s || %s) -> %s", move.From, ret.From, ret.To)
			p.planCtx.moveMu.Lock()
			p.planCtx.blockedMoves.Put(move.From, ret.To)
			p.planCtx.moveMu.Unlock()
			continue
		}
		ret.From = move.From
		ret.State = move.state
		ret.Moved = move.To != nil
		ret.ImplicitMove = move.Statement.Implied

		for step := move.moveStep; step.To != nil; step = move.To {
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
		if p.planCtx.recordedMoves.Has(ret.From) {
			diags = diags.Append(fmt.Errorf("Ambiguous Move!"))
		}
		p.planCtx.recordedMoves.Put(ret.From, ret.To)
		p.planCtx.moveMu.Unlock()
	}

	return ret, diags
}

func (p *planGlue) LocateExecutedMove(addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	to, ok := p.planCtx.recordedMoves.GetOk(addr)
	if ok {
		return &to
	}
	return nil
}

func (p *planGlue) LocateUnexecutedMove(ctx context.Context, addr addrs.AbsResourceInstance) (*addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	potentialMoves, diags := p.locateMovesFor(ctx, addr, false)
	if diags.HasErrors() {
		return nil, diags
	}

	// Calculate the potential end addresses
	toAddresses := addrs.MakeSet[addrs.AbsResourceInstance]()

	for _, move := range potentialMoves {
		if move.Statement.Implied {
			// TODO TestContext2Apply_scaleInMultivarRef
			// Implicit moves should apply here, but ony when the resource block exists
			continue
		}
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
		for _, addr := range toAddresses {
			return &addr, diags
		}
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

func invertInstanceKey(key addrs.InstanceKey) addrs.InstanceKey {
	if key == addrs.NoKey {
		return addrs.IntKey(0)
	}
	if ik, ok := key.(addrs.IntKey); ok && ik == 0 {
		return addrs.NoKey
	}
	return key
}

func implicitAddrsFor(addr addrs.AbsResourceInstance) iter.Seq[addrs.AbsResourceInstance] {
	return func(yield func(addrs.AbsResourceInstance) bool) {
		for implicit := range implicitAddrsForInternal(addr) {
			if implicit.Equal(addr) {
				continue
			}
			if !yield(implicit) {
				return
			}
		}
	}
}
func implicitAddrsForInternal(addr addrs.AbsResourceInstance) iter.Seq[addrs.AbsResourceInstance] {
	// TODO should this validate that enabled/count/for_each is available given the configuration?
	if len(addr.Module) > 0 {
		part := addr.Module[0]
		rest := addr.Module[1:]
		partInvert := addrs.ModuleInstanceStep{
			Name:        part.Name,
			InstanceKey: invertInstanceKey(part.InstanceKey),
		}
		subImplicit := implicitAddrsForInternal(addrs.AbsResourceInstance{
			Resource: addr.Resource,
			Module:   rest,
		})
		return func(yield func(addrs.AbsResourceInstance) bool) {
			for implicit := range subImplicit {
				if !yield(addrs.AbsResourceInstance{
					Resource: implicit.Resource,
					Module:   append([]addrs.ModuleInstanceStep{part}, implicit.Module...),
				}) {
					return
				}
				if part.InstanceKey == partInvert.InstanceKey {
					continue
				}
				if !yield(addrs.AbsResourceInstance{
					Resource: implicit.Resource,
					Module:   append([]addrs.ModuleInstanceStep{partInvert}, implicit.Module...),
				}) {
					return
				}
			}
		}
	} else {
		return func(yield func(addrs.AbsResourceInstance) bool) {
			if !yield(addr) {
				return
			}
			inverted := addrs.AbsResourceInstance{Resource: addrs.ResourceInstance{Resource: addr.Resource.Resource, Key: invertInstanceKey(addr.Resource.Key)}}
			if inverted.Resource.Key == addr.Resource.Key {
				return
			}
			_ = yield(inverted)
		}
	}
}
