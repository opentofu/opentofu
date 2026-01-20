// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph/execgraphproto"
)

func TestGraphMarshalUnmarshalValid(t *testing.T) {
	// This test exercises our graph marshal and unmarshal behavior by
	// feeding the marshal result directly into unmarshal and then testing
	// whether the resulting graph matches our expectations. The fine details
	// of how things get serialized are not particularly important as long
	// as we can get a functionally-equivalent graph back out again, and so
	// this is a pragmatic way to get good enough test coverage while avoiding
	// the need to update lots of fiddly tests each time we change the
	// serialization format.
	//
	// (The specific serialization format is not a compatibility constraint
	// because we explicitly disallow applying a plan created by one version of
	// OpenTofu with a different version of OpenTofu, so it's not justified
	// to unit-test the specific serialization details.)

	tests := map[string]struct {
		// InputGraph is a function that constructs the graph that should
		// be round-tripped through the marshalling code. Implementations
		// of this should typically aim to construct graphs of similar
		// shape to those that the planning engine might construct.
		InputGraph func(*Builder) *Graph
		// WantGraph is a string representation of the expected output graph,
		// using the syntax returned by [Graph.DebugRepr]. This can use a
		// raw string literal indented to align with the surrounding code
		// because we'll trim off the leading and trailing space from each line
		// before comparing.
		WantGraph string

		// We intentionally focus only on valid input here because we only
		// expect to be parsing graphs produced by OpenTofu itself, and so any
		// errors we encounter are either bugs in OpenTofu or caused by
		// something outside of OpenTofu tampering with the serialized graph.
		// The error handling in [UnmarshalGraph] is primarily to help us with
		// debugging, because end-users should never see those errors unless
		// we've made a mistake somewhere.
	}{
		"managed resource instance final plan and apply": {
			// This is intended to mimic how the planning engine would represent
			// the process of final-planning and applying an action on a
			// managed resource instance.
			func(builder *Builder) *Graph {
				instAddr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test",
					Name: "example",
				}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
				instAddrResult := builder.ConstantResourceInstAddr(instAddr)
				desiredInst := builder.ResourceInstanceDesired(instAddrResult, nil)
				priorState := builder.ResourceInstancePrior(instAddrResult)
				plannedVal := builder.ConstantValue(cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				}))
				providerClient, registerUser := builder.ProviderInstance(
					addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Config: addrs.ProviderConfigCorrect{
								Provider: addrs.NewBuiltInProvider("test"),
							},
						},
					},
					nil,
				)
				finalPlan := builder.ManagedFinalPlan(desiredInst, priorState, plannedVal, providerClient)
				newState := builder.ManagedApply(finalPlan, NilResultRef[*exec.ResourceInstanceObject](), providerClient)
				registerUser(newState)
				builder.SetResourceInstanceFinalStateResult(instAddr, newState)
				return builder.Finish()
			},
			`
				v[0] = cty.ObjectVal(map[string]cty.Value{
					"name": cty.StringVal("thingy"),
				});

				r[0] = ResourceInstanceDesired(test.example, await());
				r[1] = ResourceInstancePrior(test.example);
				r[2] = ProviderInstanceConfig(provider["terraform.io/builtin/test"], await());
				r[3] = ProviderInstanceOpen(r[2]);
				r[4] = ManagedFinalPlan(r[0], r[1], v[0], r[3]);
				r[5] = ManagedApply(r[4], nil, r[3]);
				r[6] = ProviderInstanceClose(r[3], await(r[5]));

				test.example = r[5];
			`,
		},
		"data resource instance read": {
			// This is intended to mimic how the planning engine would represent
			// the process of reading the data for a data resource instance.
			func(builder *Builder) *Graph {
				instAddr := addrs.Resource{
					Mode: addrs.DataResourceMode,
					Type: "test",
					Name: "example",
				}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
				instAddrResult := builder.ConstantResourceInstAddr(instAddr)
				desiredInst := builder.ResourceInstanceDesired(instAddrResult, nil)
				providerClient, registerUser := builder.ProviderInstance(
					addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Config: addrs.ProviderConfigCorrect{
								Provider: addrs.NewBuiltInProvider("test"),
							},
						},
					},
					nil,
				)
				plannedVal := builder.ConstantValue(cty.DynamicVal)
				newState := builder.DataRead(desiredInst, plannedVal, providerClient)
				registerUser(newState)
				builder.SetResourceInstanceFinalStateResult(instAddr, newState)
				return builder.Finish()
			},
			`
				v[0] = cty.UnknownVal(cty.DynamicPseudoType);

				r[0] = ResourceInstanceDesired(data.test.example, await());
				r[1] = ProviderInstanceConfig(provider["terraform.io/builtin/test"], await());
				r[2] = ProviderInstanceOpen(r[1]);
				r[3] = DataRead(r[0], v[0], r[2]);
				r[4] = ProviderInstanceClose(r[2], await(r[3]));

				data.test.example = r[3];
			`,
		},
		"data resource instance reads with dependency": {
			func(builder *Builder) *Graph {
				instAddr1 := addrs.Resource{
					Mode: addrs.DataResourceMode,
					Type: "test",
					Name: "example1",
				}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
				instAddr2 := addrs.Resource{
					Mode: addrs.DataResourceMode,
					Type: "test",
					Name: "example2",
				}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)
				providerClient, registerUser := builder.ProviderInstance(
					addrs.AbsProviderInstanceCorrect{
						Config: addrs.AbsProviderConfigCorrect{
							Config: addrs.ProviderConfigCorrect{
								Provider: addrs.NewBuiltInProvider("test"),
							},
						},
					},
					nil,
				)
				plannedVal := builder.ConstantValue(cty.DynamicVal)
				desiredInst1 := builder.ResourceInstanceDesired(builder.ConstantResourceInstAddr(instAddr1), nil)
				newState1 := builder.DataRead(desiredInst1, plannedVal, providerClient)
				desiredInst2 := builder.ResourceInstanceDesired(builder.ConstantResourceInstAddr(instAddr2), builder.Waiter(newState1))
				newState2 := builder.DataRead(desiredInst2, plannedVal, providerClient)
				registerUser(newState1)
				registerUser(newState2)
				builder.SetResourceInstanceFinalStateResult(instAddr1, newState1)
				builder.SetResourceInstanceFinalStateResult(instAddr2, newState2)
				return builder.Finish()
			},
			`
				v[0] = cty.UnknownVal(cty.DynamicPseudoType);

				r[0] = ProviderInstanceConfig(provider["terraform.io/builtin/test"], await());
				r[1] = ProviderInstanceOpen(r[0]);
				r[2] = ResourceInstanceDesired(data.test.example1, await());
				r[3] = DataRead(r[2], v[0], r[1]);
				r[4] = ResourceInstanceDesired(data.test.example2, await(r[3]));
				r[5] = DataRead(r[4], v[0], r[1]);
				r[6] = ProviderInstanceClose(r[1], await(r[3], r[5]));

				data.test.example1 = r[3];
				data.test.example2 = r[5];
			`,
		},

		////////
		// The remaining test cases are covering some weird cases just to
		// make sure we can handle them without crashing or otherwise
		// misbehaving. We don't need to go overboard here because this code
		// only really needs to support graphs that OpenTofu's planning engine
		// could reasonably generate.
		////////
		"empty": {
			func(builder *Builder) *Graph {
				return builder.Finish()
			},
			``,
		},
		"unused values discarded": {
			// The graph marshaling is driven by what is refered to by the
			// operations in the graph, and so anything that isn't actually
			// used by an operation is irrelevant and discarded.
			// (This test case is just here to remind us that this is the
			// behavior. We don't actually rely on this behavior for
			// correctness, because a value that isn't used in any operation
			// is effectively ignored during execution anyway.)
			func(b *Builder) *Graph {
				b.ConstantValue(cty.True)
				return b.Finish()
			},
			``,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			inputGraph := test.InputGraph(NewBuilder())
			marshaled := inputGraph.Marshal()
			// The debug representation of the marshaled graph can be pretty
			// verbose, so we'll print it only when we're reporting a failure
			// so that a reader can look up any element indices that appear
			// in the error messages.
			showMarshaled := func() {
				marshaledText := graphProtoDebugRepr(marshaled)
				t.Log("graph marshaled as:\n" + marshaledText)
			}
			outputGraph, err := UnmarshalGraph(marshaled)
			if err != nil {
				// This test function only deals with valid cases, so we
				// don't expect any errors here.
				showMarshaled()
				t.Fatalf("unexpected unmarshal error: %s", err)
			}
			gotRepr := trimLineSpaces(outputGraph.DebugRepr())
			wantRepr := trimLineSpaces(test.WantGraph)
			if diff := cmp.Diff(wantRepr, gotRepr); diff != "" {
				showMarshaled()
				t.Error("wrong output graph:\n" + diff)
			}
		})
	}
}

// trimLineSpaces returns a modified version of the given string where each
// individual line has its leading and trailing spaces removed, where
// "spaces" is defined the same way as for [strings.TrimSpace].
//
// This is here just to make it easier to compare results from [Graph.DebugRepr]
// with string constants written in test code, while still having those
// string constants indented consistently with the surrounding code. Apply this
// function to both the constant string and the [Graph.DebugRepr] result and
// then compare the two e.g. using [cmp.Diff].
func trimLineSpaces(input string) string {
	var buf strings.Builder

	// Since this function is tailored for Graph.DebugRepr in particular
	// we'll use simplistic string cutting instead of all of the complexity
	// of bufio.Scanner here, which also means we can minimize copying
	// in conversions between string and []byte.
	remain := input
	for len(remain) != 0 {
		line, extra, _ := strings.Cut(remain, "\n")
		remain = extra
		buf.WriteString(strings.TrimSpace(line))
		buf.WriteByte('\n')
	}

	// We also ignore any leading and trailing spaces in the result, which
	// could caused if there's an extra newline at the start or end of the
	// string, as tends to happen when formatting a raw string to match the
	// indentation of its surroundings.
	return strings.TrimSpace(buf.String())
}

// graphProtoDebugRepr produces a human-oriented string representation of
// a serialized execution graph for test debugging purposes only.
func graphProtoDebugRepr(wire []byte) string {
	var buf strings.Builder
	var protoGraph execgraphproto.ExecutionGraph
	err := proto.Unmarshal(wire, &protoGraph)
	if err != nil {
		panic(fmt.Sprintf("invalid protobuf serialization of %T: %s", &protoGraph, err))
	}

	// Various parts of the graph serialization involve indices into the
	// same array of elements, so it's helpful to include the indices
	// explicitly in the output.
	for i, elem := range protoGraph.GetElements() {
		asJSON := protojson.Format(elem)
		fmt.Fprintf(&buf, "%d: %s\n", i, asJSON)
	}

	resourceInstResults := protoGraph.GetResourceInstanceResults()
	if len(resourceInstResults) != 0 {
		resourceInstResultsJSON, err := json.MarshalIndent(resourceInstResults, "", "  ")
		if err != nil {
			panic(fmt.Sprintf("can't marshal resource instance results: %s", err))
		}
		fmt.Fprintf(&buf, "resource instance results: %s\n", resourceInstResultsJSON)
	}

	return buf.String()
}
