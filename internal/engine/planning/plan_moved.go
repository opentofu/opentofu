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

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type prevStateInfo struct {
	From, To     addrs.AbsResourceInstance
	State        *states.ResourceInstanceObjectFullSrc
	Moved        bool
	ImplicitMove bool
}

func (p *planGlue) LocatePreviousState(ctx context.Context, addr addrs.AbsResourceInstance) (prevStateInfo, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Start out unmoved
	ret := prevStateInfo{
		From:  addr,
		To:    addr,
		State: p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(addr.CurrentObject()),
	}

	// Query potentially active moved blocks that pertain to our resource
	// Question: do we need to re-query when dealing with implicit moves?
	moveStatements := p.oracle.MoveStatementsFor(ctx, addr)

	// We need to check the following:
	// * Is there state at any of the previous moves
	// * Do any of the moves conflict?
	// * Do we have both state at a previous move and a future move?
	// * How do we take "implicit" moves into account here?

	currentIteration := addrs.MakeSet(addr)
	nextIteration := addrs.MakeSet[addrs.AbsResourceInstance]()
	potentialAddresses := addrs.MakeSet[addrs.AbsResourceInstance]()

	invertInstanceKey := func(key addrs.InstanceKey) addrs.InstanceKey {
		if key == addrs.NoKey {
			return addrs.IntKey(0)
		}
		if ik, ok := key.(addrs.IntKey); ok && ik == 0 {
			return addrs.NoKey
		}
		return key
	}

	var implicitAddrsFor func(addr addrs.AbsResourceInstance) iter.Seq[addrs.AbsResourceInstance]
	implicitAddrsFor = func(addr addrs.AbsResourceInstance) iter.Seq[addrs.AbsResourceInstance] {
		if len(addr.Module) > 0 {
			part := addr.Module[0]
			rest := addr.Module[1:]
			partInvert := addrs.ModuleInstanceStep{
				Name:        part.Name,
				InstanceKey: invertInstanceKey(part.InstanceKey),
			}
			subImplicit := implicitAddrsFor(addrs.AbsResourceInstance{
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

	// TODO detect move cycles

	for len(currentIteration) > 0 {
		for _, addr := range currentIteration {
			for implicitAddr := range implicitAddrsFor(addr) {
				if implicitAddr.Equal(addr) {
					continue
				}
				if potentialAddresses.Has(implicitAddr) {
					continue
				}
				state := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(implicitAddr.CurrentObject())
				if state != nil {
					// We have found an active move
					log.Printf("[TRACE] Detected implicit move of %s to %s", implicitAddr, ret.To)
					if ret.State != nil {
						p.planCtx.moveMu.Lock()
						p.planCtx.blockedMoves.Put(implicitAddr, ret.To)
						p.planCtx.moveMu.Unlock()
						continue
					}
					ret.From = implicitAddr
					ret.State = state
					ret.Moved = true
					ret.ImplicitMove = true

					potentialAddresses.Add(implicitAddr)
					nextIteration.Add(implicitAddr)
				}
				// TODO potentialAddresses / nextIteration?
			}

			for _, move := range moveStatements {
				if prevAddr, moved := addr.MoveDestination(move.To, move.From); moved {
					if potentialAddresses.Has(prevAddr) {
						p.planCtx.moveMu.Lock()
						p.planCtx.blockedMoves.Put(prevAddr, ret.To)
						p.planCtx.moveMu.Unlock()
						continue
					}

					state := p.planCtx.prevRoundState.SyncWrapper().ResourceInstanceObjectFull(prevAddr.CurrentObject())
					if state != nil {
						// We have found an active move
						log.Printf("[TRACE] Detected explicit move of %s to %s", prevAddr, ret.To)
						if ret.State != nil {
							p.planCtx.moveMu.Lock()
							p.planCtx.blockedMoves.Put(prevAddr, ret.To)
							p.planCtx.moveMu.Unlock()
							continue
						}
						ret.From = prevAddr
						ret.State = state
						ret.Moved = true
					}

					potentialAddresses.Add(prevAddr)
					nextIteration.Add(prevAddr)
				}
			}
		}

		// Swap current and next
		currentIteration, nextIteration = nextIteration, currentIteration
		// Clear next
		clear(nextIteration)
	}

	if !ret.To.Equal(ret.From) {
		// Active move
		p.planCtx.moveMu.Lock()
		// TODO ambiguous moves
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

func (p *planGlue) LocateUnexecutedMove(ctx context.Context, addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	// TODO
	return nil
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
