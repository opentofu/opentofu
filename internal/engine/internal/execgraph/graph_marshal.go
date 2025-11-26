// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
	ctymsgpack "github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/protobuf/proto"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph/execgraphproto"
	"github.com/opentofu/opentofu/internal/states"
)

// Marshal produces an opaque byte slice representing the given graph,
// which can then be passed to [UnmarshalGraph] to produce a
// functionally-equivalent graph.
func (g *Graph) Marshal() []byte {
	m := &graphMarshaler{
		graph:   g,
		indices: make(map[AnyResultRef]uint64),
	}

	// The operations are the main essence of any graph, so we let those
	// drive the process here and then append anything else they refer to
	// as we analyze their arguments.
	//
	// This approach forces the elements into a topological order as we
	// go along, which [UnmarshalGraph] relies on so it can validate the
	// elements gradually during loading by referring back to elements
	// that it had previously loaded.
	for idx := range g.ops {
		m.EnsureOperationPresent(idx)
	}
	// We also record which element produces the final state result for
	// each resource instance.
	m.EnsureResourceInstanceResultsPresent(g.resourceInstanceResults)

	return m.Bytes()
}

type graphMarshaler struct {
	graph                   *Graph
	elems                   []*execgraphproto.Element
	indices                 map[AnyResultRef]uint64
	resourceInstanceResults map[string]uint64
}

func (m *graphMarshaler) EnsureOperationPresent(idx int) uint64 {
	// We have a little problem here because we don't retain information
	// about the result type of each operation directly in the graph -- that's
	// encoded in the references instead -- but we really only need the
	// raw operation indices here anyway and so as a private detail for
	// graphMarshaler alone we'll just use a type-erased operation ref
	// type, and then the unmarshal code will check that the result types
	// are actually consistent as part of its validation process.
	erasedRef := m.operationRefFromIdx(idx)
	return m.ensureRefTarget(erasedRef)
}

func (m *graphMarshaler) EnsureResourceInstanceResultsPresent(results addrs.Map[addrs.AbsResourceInstance, ResultRef[*states.ResourceInstanceObjectFull]]) {
	m.resourceInstanceResults = make(map[string]uint64)
	for _, mapElem := range results.Elems {
		instAddr := mapElem.Key
		resultRef := mapElem.Value
		resultIdx := m.ensureRefTarget(resultRef)
		m.resourceInstanceResults[instAddr.String()] = resultIdx
	}
}

func (m *graphMarshaler) ensureRefTarget(ref AnyResultRef) uint64 {
	if ref == nil {
		panic("ensureRefTarget with nil ref")
	}
	if opRef, ok := ref.(anyOperationResultRef); ok {
		// Our lookup table doesn't care about the result type of each
		// operation, so we'll erase any type information to obtain
		// the key that's actually used in the map.
		ref = m.typeErasedOperationRef(opRef)
	}
	if existing, ok := m.indices[ref]; ok {
		return existing
	}

	switch ref := ref.(type) {
	case valueResultRef:
		return m.addConstantValue(ref, m.graph.constantVals[ref.index])
	case providerAddrResultRef:
		return m.addProviderAddr(ref, m.graph.providerAddrs[ref.index])
	case desiredResourceInstanceResultRef:
		return m.addDesiredStateRef(ref, m.graph.desiredStateRefs[ref.index])
	case resourceInstancePriorStateResultRef:
		return m.addPriorStateRef(ref, m.graph.priorStateRefs[ref.index])
	case providerInstanceConfigResultRef:
		return m.addProviderInstanceConfigRef(ref, m.graph.providerInstConfigRefs[ref.index])
	case operationResultRef[struct{}]:
		return m.addOperationWithDependencies(ref, m.graph.ops[ref.index])
	case waiterResultRef:
		return m.addWaiter(ref, m.graph.waiters[ref.index])
	default:
		// Should not get here because the cases above should cover all of
		// the variants of AnyResultRef.
		panic(fmt.Sprintf("graphMarshaler doesn't know how to handle %#v", ref))
	}
}

func (m *graphMarshaler) addConstantValue(ref valueResultRef, v cty.Value) uint64 {
	raw, err := ctymsgpack.Marshal(v, cty.DynamicPseudoType)
	if err != nil {
		// Here we assume that any value we're given is one either produced by
		// OpenTofu itself or at least previously unmarshaled by OpenTofu, and
		// thus we should never encounter an error trying to serialize it.
		panic(fmt.Sprintf("constant value %d is not MessagePack-compatible: %s", ref.index, err))
	}
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		elem.SetConstantValue(raw)
	})
}

func (m *graphMarshaler) addProviderAddr(ref providerAddrResultRef, addr addrs.Provider) uint64 {
	addrStr := addr.String()
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		elem.SetConstantProviderAddr(addrStr)
	})
}

func (m *graphMarshaler) addDesiredStateRef(ref desiredResourceInstanceResultRef, addr addrs.AbsResourceInstance) uint64 {
	addrStr := addr.String()
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		elem.SetDesiredResourceInstance(addrStr)
	})
}

func (m *graphMarshaler) addPriorStateRef(ref resourceInstancePriorStateResultRef, target resourceInstanceStateRef) uint64 {
	// We serialize these ones a little differently depending on whether there's
	// a deposed key, because deposed objects in prior state are relatively
	// rare and we'd prefer a more compact representation of the more common
	// case of describing a "current" resource instance object.
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		if target.DeposedKey == states.NotDeposed {
			elem.SetResourceInstancePriorState(target.ResourceInstance.String())
		} else {
			var req execgraphproto.DeposedResourceInstanceObject
			req.Reset()
			req.SetInstanceAddr(target.ResourceInstance.String())
			req.SetDeposedKey(target.DeposedKey.String())
			elem.SetResourceInstanceDeposedObjectState(&req)
		}
	})
}

func (m *graphMarshaler) addProviderInstanceConfigRef(ref providerInstanceConfigResultRef, addr addrs.AbsProviderInstanceCorrect) uint64 {
	addrStr := addr.String()
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		elem.SetProviderInstanceConfig(addrStr)
	})
}

func (m *graphMarshaler) addOperationWithDependencies(ref operationResultRef[struct{}], desc operationDesc) uint64 {
	// During marshaling we just assume the graph was correctly constructed
	// and save the operation items in their raw form. The unmarshal code
	// then uses the opcode to decide which method of [Builder] to call
	// for each operation as part of its validation logic.
	var rawRefIdxs []uint64
	if len(desc.operands) != 0 {
		rawRefIdxs = make([]uint64, len(desc.operands))
	}
	for i, operandRef := range desc.operands {
		rawRefIdxs[i] = m.ensureRefTarget(operandRef)
	}
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		opCode := uint64(desc.opCode)
		elem.SetOperation(execgraphproto.Operation_builder{
			Opcode:   &opCode,
			Operands: rawRefIdxs,
		}.Build())
	})
}

func (m *graphMarshaler) addWaiter(ref waiterResultRef, deps []AnyResultRef) uint64 {
	rawRefIdxs := make([]uint64, len(deps))
	for i, depRef := range deps {
		rawRefIdxs[i] = m.ensureRefTarget(depRef)
	}
	return m.newElement(ref, func(elem *execgraphproto.Element) {
		elem.SetWaiter(execgraphproto.Waiter_builder{
			Results: rawRefIdxs,
		}.Build())
	})
}

// typeErasedOperationRef erases the specific result type information from an
// operation result ref because for serialization purposes we only really
// care about the indices: the unmarshal code will recover the type information
// again through its validation logic.
func (m *graphMarshaler) typeErasedOperationRef(ref anyOperationResultRef) operationResultRef[struct{}] {
	return operationResultRef[struct{}]{ref.operationResultIndex()}
}

// operationRefFromIdx is similar to [typeErasedOperationRef] but starts with
// a raw index into the slice of operations, without any type information at all.
func (m *graphMarshaler) operationRefFromIdx(idx int) operationResultRef[struct{}] {
	return operationResultRef[struct{}]{idx}
}

// newElement is the low-level primitive beneath all of the methods that add
// elements to the marshaled version of the graph.
//
// Constructing protobuf messages through the "opaque"-style API is somewhat
// awkward, so we use a callback here so that the caller can focus just on
// populating an empty-but-valid [execgraphproto.Element] using one of the
// setters on that type. The provided function MUST call exactly one of the
// setters from the oneOf message, since the method chosen also decides which
// element type gets recorded in the serialized messages.
func (m *graphMarshaler) newElement(ref AnyResultRef, build func(*execgraphproto.Element)) uint64 {
	var elem execgraphproto.Element
	elem.Reset()
	build(&elem)
	idx := uint64(len(m.elems))
	m.elems = append(m.elems, &elem)
	m.indices[ref] = idx
	return idx
}

func (m *graphMarshaler) Bytes() []byte {
	var root execgraphproto.ExecutionGraph
	root.Reset()
	root.SetElements(m.elems)
	root.SetResourceInstanceResults(m.resourceInstanceResults)
	ret, err := proto.Marshal(&root)
	// We constructed everything in this overall message, so if it isn't
	// marshalable then that's always a bug somewhere in the code in this file.
	if err != nil {
		panic(fmt.Sprintf("produced unmarshalable protobuf message: %s", err))
	}
	return ret
}
