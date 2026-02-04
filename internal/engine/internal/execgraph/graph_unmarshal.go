// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"fmt"
	"math"

	"github.com/zclconf/go-cty/cty"
	ctymsgpack "github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/protobuf/proto"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph/execgraphproto"
	"github.com/opentofu/opentofu/internal/lang/eval"
)

// UnmarshalGraph takes some bytes previously returned by [Graph.Marshal] and
// returns a graph that is functionally-equivalent to (but not necessarily
// identical to) the original graph.
//
// Because this is working with data loaded from outside OpenTofu it returns
// errors when encountering problems, but if it fails when unmarshaling an
// unmodified result from [Graph.Marshal] then that represents a bug in either
// this or that function: they should always be updated together so they are
// implementing the same file format.
func UnmarshalGraph(src []byte) (*Graph, error) {
	var root execgraphproto.ExecutionGraph
	err := proto.Unmarshal(src, &root)
	if err != nil {
		return nil, fmt.Errorf("invalid wire format: %w", err)
	}

	elems := root.GetElements()
	// During decoding we'll track the typed result ref corresponding to
	// each element from the serialized graph, which then allows us to
	// make sure that operands are actually of the types they ought to be
	// during our validation work.
	results := make([]AnyResultRef, len(elems))
	builder := NewBuilder()

	for idx, elem := range elems {
		if !elem.HasRequest() {
			// As a special case, a totally-unpopulated result is allowed to
			// coerce to any type when we're decoding operation arguments,
			// becoming the zero value of the target type.
			results[idx] = nil
			continue
		}
		switch reqType := elem.WhichRequest(); reqType {
		case execgraphproto.Element_Operation_case:
			resultRef, err := unmarshalOperationElem(elem.GetOperation(), results, builder)
			if err != nil {
				return nil, fmt.Errorf("invalid operation in element %d: %w", idx, err)
			}
			results[idx] = resultRef

		case execgraphproto.Element_Waiter_case:
			resultRef, err := unmarshalWaiterElem(elem.GetWaiter(), results, builder)
			if err != nil {
				return nil, fmt.Errorf("invalid waiter in element %d: %w", idx, err)
			}
			results[idx] = resultRef

		case execgraphproto.Element_ConstantValue_case:
			val, err := unmarshalConstantValueElem(elem.GetConstantValue())
			if err != nil {
				return nil, fmt.Errorf("invalid constant value in element %d: %w", idx, err)
			}
			results[idx] = builder.ConstantValue(val)

		case execgraphproto.Element_ConstantResourceInstAddr_case:
			addr, err := unmarshalConstantResourceInstAddr(elem.GetConstantResourceInstAddr())
			if err != nil {
				return nil, fmt.Errorf("invalid resource instance address in element %d: %w", idx, err)
			}
			results[idx] = builder.ConstantResourceInstAddr(addr)

		case execgraphproto.Element_ConstantProviderInstAddr_case:
			addr, err := unmarshalConstantProviderInstAddr(elem.GetConstantProviderInstAddr())
			if err != nil {
				return nil, fmt.Errorf("invalid provider instance address in element %d: %w", idx, err)
			}
			results[idx] = builder.ConstantProviderInstAddr(addr)

		default:
			// The above cases should cover all of the valid values of
			// execgraphproto.case_Element_Request, so we should not get here
			// for any serialized graph that was produced by this version
			// of OpenTofu.
			return nil, fmt.Errorf("unrecognized request type %#v for element %d", reqType, idx)
		}
	}

	for instAddrStr, resultIdx := range root.GetResourceInstanceResults() {
		instAddr, diags := addrs.ParseAbsResourceInstanceStr(instAddrStr)
		if diags.HasErrors() {
			return nil, fmt.Errorf("invalid resource instance address %q: %w", instAddrStr, diags.Err())
		}
		resultRef, err := unmarshalGetPrevResultOf[*exec.ResourceInstanceObject](results, resultIdx)
		if err != nil {
			return nil, fmt.Errorf("invalid result element for %s: %w", instAddr, err)
		}
		builder.SetResourceInstanceFinalStateResult(instAddr, resultRef)
	}

	return builder.Finish(), nil
}

func unmarshalOperationElem(protoOp *execgraphproto.Operation, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	switch c := protoOp.GetOpcode(); opCode(c) {
	case opProviderInstanceConfig:
		return unmarshalOpProviderInstanceConfig(protoOp.GetOperands(), prevResults, builder)
	case opProviderInstanceOpen:
		return unmarshalOpProviderInstanceOpen(protoOp.GetOperands(), prevResults, builder)
	case opProviderInstanceClose:
		return unmarshalOpProviderInstanceClose(protoOp.GetOperands(), prevResults, builder)
	case opResourceInstanceDesired:
		return unmarshalOpResourceInstanceDesired(protoOp.GetOperands(), prevResults, builder)
	case opResourceInstancePrior:
		return unmarshalOpResourceInstancePrior(protoOp.GetOperands(), prevResults, builder)
	case opManagedFinalPlan:
		return unmarshalOpManagedFinalPlan(protoOp.GetOperands(), prevResults, builder)
	case opManagedApply:
		return unmarshalOpManagedApply(protoOp.GetOperands(), prevResults, builder)
	case opManagedDepose:
		return unmarshalOpManagedDepose(protoOp.GetOperands(), prevResults, builder)
	case opManagedAlreadyDeposed:
		return unmarshalOpManagedAlreadyDeposed(protoOp.GetOperands(), prevResults, builder)
	case opDataRead:
		return unmarshalOpDataRead(protoOp.GetOperands(), prevResults, builder)
	case opEphemeralOpen:
		return unmarshalOpEphemeralOpen(protoOp.GetOperands(), prevResults, builder)
	case opEphemeralState:
		return unmarshalOpEphemeralState(protoOp.GetOperands(), prevResults, builder)
	case opEphemeralClose:
		return unmarshalOpEphemeralClose(protoOp.GetOperands(), prevResults, builder)
	default:
		// The above cases should cover all valid values of [opCode], so we
		// should not get here unless the serialized graph was tampered
		// with outside of OpenTofu.
		return nil, fmt.Errorf("unrecognized opcode %d", c)
	}
}

func unmarshalOpProviderInstanceConfig(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opProviderInstanceConfig", len(rawOperands))
	}
	providerInstAddr, err := unmarshalGetPrevResultOf[addrs.AbsProviderInstanceCorrect](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opProviderInstanceConfig providerInstAddr: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opProviderInstanceConfig waitFor: %w", err)
	}
	return builder.ProviderInstanceConfig(providerInstAddr, waitFor), nil
}

func unmarshalOpProviderInstanceOpen(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 1 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opProviderInstanceOpen", len(rawOperands))
	}
	config, err := unmarshalGetPrevResultOf[*exec.ProviderInstanceConfig](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opProviderInstanceOpen config: %w", err)
	}
	return builder.ProviderInstanceOpen(config), nil
}

func unmarshalOpProviderInstanceClose(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opProviderInstanceClose", len(rawOperands))
	}
	client, err := unmarshalGetPrevResultOf[*exec.ProviderClient](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opProviderInstanceClose client: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opProviderInstanceClose waitFor: %w", err)
	}
	return builder.ProviderInstanceClose(client, waitFor), nil
}

func unmarshalOpResourceInstanceDesired(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opResourceInstanceDesired", len(rawOperands))
	}
	addr, err := unmarshalGetPrevResultOf[addrs.AbsResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opResourceInstanceDesired addr: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opResourceInstanceDesired waitFor: %w", err)
	}
	return builder.ResourceInstanceDesired(addr, waitFor), nil
}

func unmarshalOpResourceInstancePrior(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 1 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opResourceInstancePrior", len(rawOperands))
	}
	addr, err := unmarshalGetPrevResultOf[addrs.AbsResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opResourceInstancePrior addr: %w", err)
	}
	return builder.ResourceInstancePrior(addr), nil
}

func unmarshalOpManagedFinalPlan(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 4 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opManagedFinalPlan", len(rawOperands))
	}
	desiredInst, err := unmarshalGetPrevResultOf[*eval.DesiredResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan desiredInst: %w", err)
	}
	priorState, err := unmarshalGetPrevResultOf[*exec.ResourceInstanceObject](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan priorState: %w", err)
	}
	plannedVal, err := unmarshalGetPrevResultOf[cty.Value](prevResults, rawOperands[2])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan plannedVal: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[*exec.ProviderClient](prevResults, rawOperands[3])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan providerClient: %w", err)
	}
	return builder.ManagedFinalPlan(desiredInst, priorState, plannedVal, providerClient), nil
}

func unmarshalOpManagedApply(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 3 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opManagedApplyChanges", len(rawOperands))
	}
	finalPlan, err := unmarshalGetPrevResultOf[*exec.ManagedResourceObjectFinalPlan](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedApplyChanges finalPlan: %w", err)
	}
	fallbackObj, err := unmarshalGetPrevResultOf[*exec.ResourceInstanceObject](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedApplyChanges fallbackObj: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[*exec.ProviderClient](prevResults, rawOperands[2])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedApplyChanges providerClient: %w", err)
	}
	return builder.ManagedApply(finalPlan, fallbackObj, providerClient), nil
}

func unmarshalOpManagedDepose(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 1 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opManagedDepose", len(rawOperands))
	}
	instAddr, err := unmarshalGetPrevResultOf[addrs.AbsResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedDepose instAddr: %w", err)
	}
	return builder.ManagedDepose(instAddr), nil
}

func unmarshalOpManagedAlreadyDeposed(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 1 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opManagedAlreadyDeposed", len(rawOperands))
	}
	instAddr, err := unmarshalGetPrevResultOf[addrs.AbsResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedAlreadyDeposed instAddr: %w", err)
	}
	return builder.ManagedAlreadyDeposed(instAddr), nil
}

func unmarshalOpDataRead(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 3 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opDataRead", len(rawOperands))
	}
	desiredInst, err := unmarshalGetPrevResultOf[*eval.DesiredResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead desiredInst: %w", err)
	}
	plannedVal, err := unmarshalGetPrevResultOf[cty.Value](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead plannedVal: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[*exec.ProviderClient](prevResults, rawOperands[2])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead providerClient: %w", err)
	}
	return builder.DataRead(desiredInst, plannedVal, providerClient), nil
}

func unmarshalOpEphemeralOpen(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opDataRead", len(rawOperands))
	}
	desiredInst, err := unmarshalGetPrevResultOf[*eval.DesiredResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead desiredInst: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[*exec.ProviderClient](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead providerClient: %w", err)
	}
	return builder.EphemeralOpen(desiredInst, providerClient), nil
}

func unmarshalOpEphemeralState(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 1 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opDataRead", len(rawOperands))
	}
	ephemeralInst, err := unmarshalGetPrevResultOf[*exec.OpenEphemeralResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead desiredInst: %w", err)
	}
	return builder.EphemeralState(ephemeralInst), nil
}

func unmarshalOpEphemeralClose(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opDataRead", len(rawOperands))
	}
	ephemeralInst, err := unmarshalGetPrevResultOf[*exec.OpenEphemeralResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead desiredInst: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead waitFor: %w", err)
	}
	return builder.EphemeralClose(ephemeralInst, waitFor), nil
}

func unmarshalWaiterElem(protoWaiter *execgraphproto.Waiter, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	waitForIdxs := protoWaiter.GetResults()
	waitFor := make([]AnyResultRef, len(waitForIdxs))
	for i, prevResultIdx := range waitForIdxs {
		result := unmarshalGetPrevResult(prevResults, prevResultIdx)
		if result == nil {
			return nil, fmt.Errorf("refers to later element %d", prevResultIdx)
		}
		waitFor[i] = result
	}
	waiter := builder.Waiter(waitFor...)
	return waiter, nil
}

func unmarshalConstantValueElem(src []byte) (cty.Value, error) {
	v, err := ctymsgpack.Unmarshal(src, cty.DynamicPseudoType)
	if err != nil {
		return cty.NilVal, fmt.Errorf("invalid MessagePack encoding: %w", err)
	}
	return v, nil
}

func unmarshalConstantResourceInstAddr(addrStr string) (addrs.AbsResourceInstance, error) {
	// This address parser also returns diagnostics rather than just an error,
	// which is inconvenient. :(
	ret, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
	return ret, diags.Err()
}

func unmarshalConstantProviderInstAddr(addrStr string) (addrs.AbsProviderInstanceCorrect, error) {
	// FIXME: We don't yet have a parser for addrs.AbsProviderInstanceCorrect,
	// so we'll borrow the legacy one and then adapt its result. Note that this
	// means that there are certain shapes of address that we'll fail to
	// parse here, because this legacy parser doesn't support module instances
	// that have instance keys.
	// This address parser also returns diagnostics rather than just an error,
	// which is inconvenient. :(
	pc, k, diags := addrs.ParseAbsProviderConfigInstanceStr(addrStr)
	if diags.HasErrors() {
		return addrs.AbsProviderInstanceCorrect{}, diags.Err()
	}
	ret := addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Module: pc.Module.UnkeyedInstanceShim(),
			Config: addrs.ProviderConfigCorrect{
				Provider: pc.Provider,
				Alias:    pc.Alias,
			},
		},
		Key: k,
	}
	return ret, diags.Err()
}

func unmarshalGetPrevResult(prevResults []AnyResultRef, resultIdx uint64) AnyResultRef {
	if resultIdx > math.MaxInt || int(resultIdx) > len(prevResults) {
		return nil
	}
	return prevResults[int(resultIdx)]
}

func unmarshalGetPrevResultOf[T any](prevResults []AnyResultRef, resultIdx uint64) (ResultRef[T], error) {
	dynResult := unmarshalGetPrevResult(prevResults, resultIdx)
	if dynResult == nil {
		// We're assuming here that nil represents NilResultRef. This could
		// also potentially represent a reference to a result that hasn't
		// appeared yet, but that would only be possible if the input is
		// invalid.
		return NilResultRef[T](), nil
	}
	ret, ok := dynResult.(ResultRef[T])
	if !ok {
		return nil, fmt.Errorf("refers to %T, but need %T", dynResult, ret)
	}
	return ret, nil
}

func unmarshalGetPrevResultWaiter(prevResults []AnyResultRef, resultIdx uint64) (AnyResultRef, error) {
	ret := unmarshalGetPrevResult(prevResults, resultIdx)
	if ret == nil {
		return nil, fmt.Errorf("refers to later element %d", resultIdx)
	}
	return ret, nil
}
