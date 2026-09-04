// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"slices"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type moveResults struct {
	Changes addrs.SyncMap[addrs.AbsResourceInstance, refactoring.MoveSuccess]
	Blocked addrs.SyncMap[addrs.AbsMoveable, refactoring.MoveBlocked]
}

// SetUpMoveStatements obtains the complete collection of move statements from the root module
// and all its called sub-modules. Diagnostics from move graph validation are returned,
// which checks for cycles in the graph.
func (o *PlanningOracle) SetUpMoveStatements(ctx context.Context) tfdiags.Diagnostics {
	o.moveResults = moveResults{
		Changes: addrs.MakeSyncMap[addrs.AbsResourceInstance, refactoring.MoveSuccess](),
		Blocked: addrs.MakeSyncMap[addrs.AbsMoveable, refactoring.MoveBlocked](),
	}
	o.moveStatements = slices.Collect(o.root.GetMoveStatements(ctx))
	return refactoring.ValidateMoveStatementGraph(o.moveStatements)
}

// FindAddressesMovedFromHere returns all of the addresses that this address will be moved to,
// that is, it follows the move statements' in their logical way.
//
// Importantly, this function assumes the underlying move statement graph
// has no cycles. Be sure to check for that before calling this method, or
// it may take a while to return.
//
// A diagnostic is returned if the move was ambiguous.
func (o *PlanningOracle) FindAddressesMovedFromHere(ctx context.Context, addr addrs.AbsResourceInstance) ([]addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	return o.findAddressesByMove(ctx, addr, false)
}

// FindAddressesMovedToHere returns all of the addresses that might be moved to this address,
// that is, has a series of move statements with From addresses that eventually lead to
// addr as a To address.
//
// Importantly, this function assumes the underlying move statement graph
// has no cycles. Be sure to check for that before calling this method, or
// it may take a while to return.
//
// A diagnostic is returned if the move was ambiguous.
func (o *PlanningOracle) FindAddressesMovedToHere(ctx context.Context, addr addrs.AbsResourceInstance) ([]addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	return o.findAddressesByMove(ctx, addr, true)
}

type moveInfo struct {
	addr addrs.AbsResourceInstance
	stmt refactoring.MoveStatement
}

func mapToSlice(m map[addrs.UniqueKey]*moveInfo) iter.Seq[addrs.AbsResourceInstance] {
	return func(yield func(addrs.AbsResourceInstance) bool) {
		for _, mi := range m {
			if !yield(mi.addr) {
				return
			}
		}
	}
}

func (o *PlanningOracle) findAddressesByMove(ctx context.Context, addr addrs.AbsResourceInstance, reverse bool) ([]addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	output := make([]addrs.AbsResourceInstance, 1)
	output[0] = addr
	prevAddr := addr

	selfCycles := make(map[addrs.UniqueKey]any)

	// We're never going to "move" more times
	// than there are move statements
	for range len(o.moveStatements) {
		addresses := make(map[addrs.UniqueKey]*moveInfo)
		for _, move := range o.moveStatements {
			from, to := move.From, move.To
			if reverse {
				from, to = move.To, move.From
			}
			if movedAddr, moved := prevAddr.MoveDestination(from, to); moved {
				if _, alreadyFound := selfCycles[prevAddr.UniqueKey()]; !alreadyFound && addrs.Equivalent(prevAddr, movedAddr) {
					// The previous address and current address are equivalent, meaning this statement is redundant
					// we'll shortcut here with this error
					absFrom := move.From.InModuleInstance(prevAddr.Module)

					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Redundant move statement",
						Detail: fmt.Sprintf(
							"This statement declares a move from %s to the same address, which is the same as not declaring this move at all.",
							absFrom,
						),
						Subject: move.DeclRange.ToHCL().Ptr(),
					})
					selfCycles[movedAddr.UniqueKey()] = struct{}{}
					continue
				}
				// Note: using the movedAddr.UniqueKey() is equivalent to checking addrs.Equivalent
				// So all addresses in this map will be uniquely determined
				if _, ok := addresses[movedAddr.UniqueKey()]; !ok {
					addresses[movedAddr.UniqueKey()] = &moveInfo{addr: movedAddr, stmt: move}
				}
			}
		}
		if len(addresses) == 0 {
			break
		}
		if len(addresses) > 1 {
			// more than one address means an ambiguous move
			var first *moveInfo
			for _, mi := range addresses {
				if first == nil {
					// TODO: might have to set "first" above, since the map may not be ordered the way we "expect"
					first = mi
					continue
				}
				ambiguityDiag := oneFromManyTo(first, mi)
				if reverse {
					ambiguityDiag = manyFromOneTo(first, mi)
				}
				diags = diags.Append(ambiguityDiag)

			}
			return slices.Collect(mapToSlice(addresses)), diags
		}
		// There is exactly one address in addresses.
		// TODO is there a more... idk, elegant way to extract it?
		prevAddr = slices.Collect(mapToSlice(addresses))[0]
		output = append(output, prevAddr)
	}
	return output, diags
}

// oneFromManyTo formats an existing piece of movement info and a conflicting ambiguous movement statement
// into a diagnostic error
func oneFromManyTo(first *moveInfo, mi *moveInfo) *hcl.Diagnostic {
	absFrom := first.stmt.From.InModuleInstance(first.addr.Module)
	absTo := first.stmt.To.InModuleInstance(first.addr.Module)
	absOtherTo := mi.stmt.To.InModuleInstance(mi.addr.Module)

	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Ambiguous move statements",
		Detail: fmt.Sprintf(
			"A statement at %s declared that %s moved to %s, but this statement instead declares that it moved to %s.\n\nEach %s can move to only one destination %s.",
			first.stmt.DeclRange.StartString(), absFrom, absTo, absOtherTo,
			absFrom.Noun(), absFrom.ShortNoun(),
		),
		Subject: mi.stmt.DeclRange.ToHCL().Ptr(),
	}
}

// manyFromOneTo formats an existing piece of movement info and a conflicting ambiguous movement statement
// into a diagnostic error
// TODO: maybe we can unify the manyFromOneTo with oneFromManyTo?
func manyFromOneTo(first *moveInfo, mi *moveInfo) *hcl.Diagnostic {
	absFrom := first.stmt.From.InModuleInstance(first.addr.Module)
	absTo := first.stmt.To.InModuleInstance(first.addr.Module)
	absOtherFrom := mi.stmt.From.InModuleInstance(mi.addr.Module)

	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Ambiguous move statements",
		Detail: fmt.Sprintf(
			"A statement at %s declared that %s moved to %s, but this statement instead declares that %s moved there.\n\nEach %s can have moved from only one source %s.",
			first.stmt.DeclRange.StartString(), absFrom, absTo, absOtherFrom,
			absFrom.Noun(), absFrom.ShortNoun(),
		),
		Subject: mi.stmt.DeclRange.ToHCL().Ptr(),
	}
}

func (o *PlanningOracle) RecordSuccessfulMove(newAddr, oldAddr addrs.AbsResourceInstance, implied bool) {
	o.moveResults.Changes.Put(newAddr, refactoring.MoveSuccess{
		From:    oldAddr,
		To:      newAddr,
		Implied: implied,
	})
}

func (o *PlanningOracle) RecordBlockedMove(newAddr, wantedAddr addrs.AbsResourceInstance) {
	o.moveResults.Blocked.Put(newAddr, refactoring.MoveBlocked{
		Wanted: wantedAddr,
		Actual: newAddr,
	})
}

func (o *PlanningOracle) MovedAddress(addr addrs.AbsResourceInstance) (refactoring.MoveSuccess, bool) {
	return o.moveResults.Changes.GetOk(addr)
}

func (o *PlanningOracle) BlockedDiags() tfdiags.Diagnostics {
	var itemsBuf bytes.Buffer
	// Question: Do we actually need these concurrency features?
	// I don't think Range is actually parallel...
	var mux sync.Mutex
	var markNonEmptyOnce sync.Once
	empty := true

	o.moveResults.Blocked.Range(func(_ addrs.AbsMoveable, blocked refactoring.MoveBlocked) bool {
		mux.Lock()
		fmt.Fprintf(&itemsBuf, "\n  - %s could not move to %s", blocked.Actual, blocked.Wanted)
		mux.Unlock()

		markNonEmptyOnce.Do(func() {
			empty = false
		})

		// always return true; we want to go thru every item in the range
		return true
	})

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

func (o *PlanningOracle) CheckMovesFromAddr(addr addrs.AbsResourceInstance) (diags tfdiags.Diagnostics) {
	for _, move := range o.moveStatements {
		// if the move statement can move our address, it matches the "From"
		if _, moved := addr.MoveDestination(move.From, move.To); moved {
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
	return diags
}

func reciprocalKey(a, b addrs.InstanceKey) bool {
	return a == addrs.NoKey && b == addrs.IntKey(0) ||
		a == addrs.IntKey(0) && b == addrs.NoKey
}

// SearchForImplicitMoveableResourceInstance looks through the configuration for a resource instance address
// which is reciprocal, by which we mean one of the keys changes from IntKey(0) to NoKey or vice versa.
// A flag is also returned for whether this is a "pyrrhic move": we may make a move to a non-nil resource
// instance address with a IntKey(0), but the instance does not actually exist. This is a quirk implemented
// for compatibility with the previous runtime.
func (o *PlanningOracle) SearchForImplicitMoveableResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) (*addrs.AbsResourceInstance, bool) {
	// Note: the config graph should already be expanded at this point,
	// so looking through it again to obtain resource instance information
	// should be just fine. But I'm also gonna add this, because it's
	// everywhere else that this function is used:
	// TODO: using evalglue.ResourceInstancesDeep here is probably not right
	// because it immediately starts a new grapheval worker without also
	// starting a new goroutine. We need to figure out how the non-goroutine
	// coroutines started by for loops over iter.Seq fit in to the grapheval
	// rules: they have some characteristics in common with goroutines but
	// are not capable of running asynchronously with the calling goroutine.
	// PS: I know it's evalglue.ModuleInstancesDeep, not evalglue.ResourceInstancesDeep,
	// but that's what the TODO says everywhere soooo......
	for mAddr, mi := range evalglue.ModuleInstancesDeep(ctx, o.root) {
		// Four easy steps!
		// 1. Check the module keys
		modA, modB := mAddr, addr.Module
		if len(modA) != len(modB) {
			continue
		}
		reciprocalModule := false
		incompatibleModule := false
		for i := range len(modA) {
			if modA[i].Name != modB[i].Name {
				incompatibleModule = true
				break
			}
			if reciprocalKey(modA[i].InstanceKey, modB[i].InstanceKey) {
				reciprocalModule = true
				// Do not break:
				// we want to check whether or not these are incompatible.
			} else if modA[i].InstanceKey != modB[i].InstanceKey {
				// these instances are incompatible
				incompatibleModule = true
				break
			}
		}
		if incompatibleModule {
			continue
		}

		// 2. Find the resource
		res := mi.Resource(ctx, addr.Resource.Resource)
		if res == nil {
			continue
		}
		instanceMap := res.Instances(ctx)
		instanceKeyType := res.InstanceSelector.InstanceKeyType()

		// 3. Check the resource instance key
		// Note: resource names are the same due to how we found res
		keyA, keyB := addr.Resource.Key, addr.Resource.Key
		ri := instanceMap[keyA]
		nonPyrrhicMove := true
		reciprocalResourceInstance := false
		switch instanceKeyType {
		case addrs.NoKeyType:
			if keyB == addrs.IntKey(0) {
				keyA = addrs.NoKey
				ri = instanceMap[keyA]
				reciprocalResourceInstance = true
			}
		case addrs.IntKeyType:
			if keyB == addrs.NoKey {
				keyA = addrs.IntKey(0)
				ri, nonPyrrhicMove = instanceMap[keyA]
				reciprocalResourceInstance = true
			}
		}

		// check if the resource instances are incompatible, provided
		// they aren't also reciprocals
		if !reciprocalResourceInstance && keyA != keyB {
			continue
		}

		// 4. If the module and resource keys are compatible, but at least one of them is reciprocal
		// this is a candidate for an implicit move.
		if ri != nil && (reciprocalModule || reciprocalResourceInstance) {
			return &ri.Addr, false
		}
		// If we're moving from NoKey to count = 0,
		// we're making a pyrrhic move and we handle that specially
		if !nonPyrrhicMove {
			fakeAddr := res.Addr.Instance(keyA)
			return &fakeAddr, true
		}
	}
	return nil, false
}
