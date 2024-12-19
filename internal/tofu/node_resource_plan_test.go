// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

func TestAddResourceInstanceObjectsFromRemovedModuleInstances(t *testing.T) {
	placeholderProvider := ResolvedProvider{
		ProviderConfig: addrs.AbsProviderConfig{
			Provider: addrs.NewBuiltInProvider("placeholder"),
		},
	}
	resourceAddr := addrs.RootModuleInstance.Child("foo", addrs.NoKey).Resource(
		addrs.ManagedResourceMode,
		"foo",
		"bar",
	)
	readyObj1 := &states.ResourceInstanceObjectSrc{
		Status:  states.ObjectReady,
		Private: []byte{1},
	}
	readyObj2 := &states.ResourceInstanceObjectSrc{
		Status:  states.ObjectReady,
		Private: []byte{2},
	}

	tests := map[string]struct {
		currentModuleInstances []addrs.ModuleInstance
		priorStates            []*states.Resource
		wantObjs               resourceInstanceObjectsToPlan
	}{
		"empty": {
			nil,
			nil,
			addrs.MakeMap[addrs.AbsResourceInstance, map[states.DeposedKey]*resourceInstanceObjectToPlan](),
		},
		"no prior states": {
			[]addrs.ModuleInstance{
				addrs.RootModuleInstance.Child("foo", addrs.NoKey),
			},
			nil,
			addrs.MakeMap[addrs.AbsResourceInstance, map[states.DeposedKey]*resourceInstanceObjectToPlan](),
		},
		"module instance still present": {
			[]addrs.ModuleInstance{
				addrs.RootModuleInstance.Child("foo", addrs.NoKey),
			},
			[]*states.Resource{
				{
					Addr: resourceAddr,
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: readyObj1,
						},
					},
				},
			},
			addrs.MakeMap[addrs.AbsResourceInstance, map[states.DeposedKey]*resourceInstanceObjectToPlan](),
		},
		"module instance removed": {
			nil,
			[]*states.Resource{
				{
					Addr: resourceAddr,
					Instances: map[addrs.InstanceKey]*states.ResourceInstance{
						addrs.NoKey: {
							Current: readyObj1,
							//nolint:exhaustive // for some reason this linter wants a states.NotDeposed key to appear in here, which would be nonsense
							Deposed: map[states.DeposedKey]*states.ResourceInstanceObjectSrc{
								states.DeposedKey("PLACEHOLDER"): readyObj2,
							},
						},
					},
				},
			},
			addrs.MakeMap(
				addrs.MakeMapElem(
					resourceAddr.Instance(addrs.NoKey),
					map[states.DeposedKey]*resourceInstanceObjectToPlan{
						states.NotDeposed: {
							Addr:       resourceAddr.Instance(addrs.NoKey),
							PriorState: readyObj1,
							Provider:   placeholderProvider,
						},
						states.DeposedKey("PLACEHOLDER"): {
							Addr:       resourceAddr.Instance(addrs.NoKey),
							PriorState: readyObj2,
							Provider:   placeholderProvider,
						},
					},
				),
			),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotObjs := addrs.MakeMap[addrs.AbsResourceInstance, map[states.DeposedKey]*resourceInstanceObjectToPlan]()
			addResourceInstanceObjectsFromRemovedModuleInstances(
				test.currentModuleInstances,
				test.priorStates,
				placeholderProvider,
				gotObjs,
			)
			if diff := cmp.Diff(test.wantObjs, gotObjs); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}
