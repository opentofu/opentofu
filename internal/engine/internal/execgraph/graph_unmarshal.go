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
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph/execgraphproto"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
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

		case execgraphproto.Element_ConstantProviderAddr_case:
			addr, err := unmarshalConstantProviderAddr(elem.GetConstantProviderAddr())
			if err != nil {
				return nil, fmt.Errorf("invalid provider address in element %d: %w", idx, err)
			}
			results[idx] = builder.ConstantProviderAddr(addr)

		case execgraphproto.Element_DesiredResourceInstance_case:
			addr, err := unmarshalDesiredResourceInstance(elem.GetDesiredResourceInstance())
			if err != nil {
				return nil, fmt.Errorf("invalid desired resource instance address in element %d: %w", idx, err)
			}
			results[idx] = builder.DesiredResourceInstance(addr)

		case execgraphproto.Element_ResourceInstancePriorState_case:
			addr, err := unmarshalResourceInstancePriorState(elem.GetResourceInstancePriorState())
			if err != nil {
				return nil, fmt.Errorf("invalid prior state resource instance address in element %d: %w", idx, err)
			}
			results[idx] = builder.ResourceInstancePriorState(addr)

		case execgraphproto.Element_ResourceInstanceDeposedObjectState_case:
			objRef, err := unmarshalResourceInstanceDeposedObjectState(elem.GetResourceInstanceDeposedObjectState())
			if err != nil {
				return nil, fmt.Errorf("invalid deposed resource instance object reference in element %d: %w", idx, err)
			}
			results[idx] = builder.ResourceDeposedObjectState(objRef.ResourceInstance, objRef.DeposedKey)

		case execgraphproto.Element_ProviderInstanceConfig_case:
			addr, err := unmarshalProviderInstanceConfig(elem.GetProviderInstanceConfig())
			if err != nil {
				return nil, fmt.Errorf("invalid provider instance address in element %d: %w", idx, err)
			}
			results[idx] = builder.ProviderInstanceConfig(addr)

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
		resultRef, err := unmarshalGetPrevResultOf[*states.ResourceInstanceObject](results, resultIdx)
		if err != nil {
			return nil, fmt.Errorf("invalid result element for %s: %w", instAddr, err)
		}
		builder.SetResourceInstanceFinalStateResult(instAddr, resultRef)
	}

	return builder.Finish(), nil
}

func unmarshalOperationElem(protoOp *execgraphproto.Operation, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	switch c := protoOp.GetOpcode(); opCode(c) {
	case opManagedFinalPlan:
		return unmarshalOpManagedFinalPlan(protoOp.GetOperands(), prevResults, builder)
	case opManagedApplyChanges:
		return unmarshalOpManagedApplyChanges(protoOp.GetOperands(), prevResults, builder)
	case opDataRead:
		return unmarshalOpDataRead(protoOp.GetOperands(), prevResults, builder)
	case opOpenProvider:
		return unmarshalOpOpenProvider(protoOp.GetOperands(), prevResults, builder)
	case opCloseProvider:
		return unmarshalOpCloseProvider(protoOp.GetOperands(), prevResults, builder)

		// TODO: All of the other opcodes
	default:
		// The above cases should cover all valid values of [opCode], so we
		// should not get here unless the serialized graph was tampered
		// with outside of OpenTofu.
		return nil, fmt.Errorf("unrecognized opcode %d", c)
	}
}

func unmarshalOpManagedFinalPlan(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 5 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opManagedFinalPlan", len(rawOperands))
	}
	desiredInst, err := unmarshalGetPrevResultOf[*eval.DesiredResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan desiredInst: %w", err)
	}
	priorState, err := unmarshalGetPrevResultOf[*states.ResourceInstanceObject](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan priorState: %w", err)
	}
	plannedVal, err := unmarshalGetPrevResultOf[cty.Value](prevResults, rawOperands[2])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan plannedVal: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[providers.Configured](prevResults, rawOperands[3])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan providerClient: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[4])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedFinalPlan waitFor: %w", err)
	}
	return builder.ManagedResourceObjectFinalPlan(desiredInst, priorState, plannedVal, providerClient, waitFor), nil
}

func unmarshalOpManagedApplyChanges(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opManagedApplyChanges", len(rawOperands))
	}
	finalPlan, err := unmarshalGetPrevResultOf[*ManagedResourceObjectFinalPlan](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedApplyChanges finalPlan: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[providers.Configured](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opManagedApplyChanges providerClient: %w", err)
	}
	return builder.ApplyManagedResourceObjectChanges(finalPlan, providerClient), nil
}

func unmarshalOpDataRead(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 3 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opDataRead", len(rawOperands))
	}
	desiredInst, err := unmarshalGetPrevResultOf[*eval.DesiredResourceInstance](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead desiredInst: %w", err)
	}
	providerClient, err := unmarshalGetPrevResultOf[providers.Configured](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead providerClient: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[2])
	if err != nil {
		return nil, fmt.Errorf("invalid opDataRead waitFor: %w", err)
	}
	return builder.DataRead(desiredInst, providerClient, waitFor), nil
}

func unmarshalOpOpenProvider(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 3 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opOpenProvider", len(rawOperands))
	}
	providerAddr, err := unmarshalGetPrevResultOf[addrs.Provider](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opOpenProvider providerAddr: %w", err)
	}
	config, err := unmarshalGetPrevResultOf[cty.Value](prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opOpenProvider config: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[2])
	if err != nil {
		return nil, fmt.Errorf("invalid opOpenProvider waitFor: %w", err)
	}
	return builder.OpenProviderClient(providerAddr, config, waitFor), nil
}

func unmarshalOpCloseProvider(rawOperands []uint64, prevResults []AnyResultRef, builder *Builder) (AnyResultRef, error) {
	if len(rawOperands) != 2 {
		return nil, fmt.Errorf("wrong number of operands (%d) for opCloseProvider", len(rawOperands))
	}
	client, err := unmarshalGetPrevResultOf[providers.Configured](prevResults, rawOperands[0])
	if err != nil {
		return nil, fmt.Errorf("invalid opCloseProvider client: %w", err)
	}
	waitFor, err := unmarshalGetPrevResultWaiter(prevResults, rawOperands[1])
	if err != nil {
		return nil, fmt.Errorf("invalid opCloseProvider waitFor: %w", err)
	}
	return builder.CloseProviderClient(client, waitFor), nil
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

func unmarshalConstantProviderAddr(addrStr string) (addrs.Provider, error) {
	// This address parser returns diagnostics rather than just an error,
	// which is inconvenient. :(
	ret, diags := addrs.ParseProviderSourceString(addrStr)
	return ret, diags.Err()
}

func unmarshalDesiredResourceInstance(addrStr string) (addrs.AbsResourceInstance, error) {
	// This address parser returns diagnostics rather than just an error,
	// which is inconvenient. :(
	ret, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
	return ret, diags.Err()
}

func unmarshalResourceInstancePriorState(addrStr string) (addrs.AbsResourceInstance, error) {
	// This address parser returns diagnostics rather than just an error,
	// which is inconvenient. :(
	ret, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
	return ret, diags.Err()
}

func unmarshalProviderInstanceConfig(addrStr string) (addrs.AbsProviderInstanceCorrect, error) {
	// This address parser returns diagnostics rather than just an error,
	// which is inconvenient. :(
	//
	// FIXME: AbsProviderInstanceCorrect doesn't currently have its own string
	// parsing function, we'll borrow the one we introduced for the world where
	// instance keys are tracked externally from the address. This means that
	// we're limited to only the address forms that the old system could handle,
	// including not supporting provider instances inside multi-instance modules,
	// but that's okay for now.
	var ret addrs.AbsProviderInstanceCorrect
	configAddr, instKey, diags := addrs.ParseAbsProviderConfigInstanceStr(addrStr)
	if diags.HasErrors() {
		return ret, fmt.Errorf("invalid provider instance address: %w", diags.Err())
	}
	ret.Config = configAddr.Correct()
	ret.Key = instKey
	return ret, diags.Err()
}

func unmarshalResourceInstanceDeposedObjectState(protoRef *execgraphproto.DeposedResourceInstanceObject) (resourceInstanceStateRef, error) {
	var ret resourceInstanceStateRef
	instAddrStr := protoRef.GetInstanceAddr()
	deposedKeyStr := protoRef.GetDeposedKey()

	// This address parser returns diagnostics rather than just an error,
	// which is inconvenient. :(
	instAddr, diags := addrs.ParseAbsResourceInstanceStr(instAddrStr)
	if diags.HasErrors() {
		return ret, fmt.Errorf("invalid resource instance address: %w", diags.Err())
	}
	ret.ResourceInstance = instAddr
	ret.DeposedKey = states.DeposedKey(deposedKeyStr)
	return ret, nil
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
		return nil, fmt.Errorf("refers to later element %d", resultIdx)
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
