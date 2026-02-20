// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
)

func TestFindEffectiveReplaceOrders(t *testing.T) {
	// TODO: This test is currently just a stub of some simple cases to
	// illustrate what [findEffectiveReplaceOrders] is intended to do.
	// If this function survives into a shipping form of the new planning
	// engine then we should consider whether there are any other cases we
	// should cover here.

	objAddr := func(k string) addrs.AbsResourceInstanceObject {
		return addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "test",
			Name: "test",
		}.Instance(addrs.StringKey(k)).Absolute(addrs.RootModuleInstance).CurrentObject()
	}

	tests := map[string]struct {
		build        func(*resourceInstanceObjectsBuilder)
		want         addrs.Map[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]
		wantSelfDeps addrs.Set[addrs.AbsResourceInstanceObject]
	}{
		"empty": {
			func(objs *resourceInstanceObjectsBuilder) {},
			addrs.MakeMap[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder](),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"one allowing any order": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceAnyOrder,
				})
			},
			addrs.MakeMap(addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
				Key:   objAddr("a"),
				Value: replaceDestroyThenCreate,
			}),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"one requiring create then destroy": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceCreateThenDestroy,
				})
			},
			addrs.MakeMap(addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
				Key:   objAddr("a"),
				Value: replaceCreateThenDestroy,
			}),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"chain with everything allowing any order": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceAnyOrder,
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("b"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("c"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a"), objAddr("b")),
				})
			},
			addrs.MakeMap(
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("a"),
					Value: replaceDestroyThenCreate,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("b"),
					Value: replaceDestroyThenCreate,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("c"),
					Value: replaceDestroyThenCreate,
				},
			),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"chain with leader requiring create then destroy": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceCreateThenDestroy,
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("b"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("c"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a"), objAddr("b")),
				})
			},
			addrs.MakeMap(
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("a"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("b"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("c"),
					Value: replaceCreateThenDestroy,
				},
			),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"chain with middle requiring create then destroy": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceAnyOrder,
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("b"),
					ReplaceOrder: replaceCreateThenDestroy,
					Dependencies: addrs.MakeSet(objAddr("a")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("c"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a"), objAddr("b")),
				})
			},
			addrs.MakeMap(
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("a"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("b"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("c"),
					Value: replaceCreateThenDestroy,
				},
			),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"chain with trailer requiring create then destroy": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceAnyOrder,
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("b"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("c"),
					ReplaceOrder: replaceCreateThenDestroy,
					Dependencies: addrs.MakeSet(objAddr("a"), objAddr("b")),
				})
			},
			addrs.MakeMap(
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("a"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("b"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("c"),
					Value: replaceCreateThenDestroy,
				},
			),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"unchained object unaffected": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceAnyOrder,
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("b"),
					ReplaceOrder: replaceCreateThenDestroy,
					Dependencies: addrs.MakeSet(objAddr("a")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("unchained"),
					ReplaceOrder: replaceAnyOrder,
				})
			},
			addrs.MakeMap(
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("a"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("b"),
					Value: replaceCreateThenDestroy,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("unchained"),
					Value: replaceDestroyThenCreate,
				},
			),
			addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		},
		"self-dependency": {
			func(objs *resourceInstanceObjectsBuilder) {
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("a"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a"), objAddr("b")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("b"),
					ReplaceOrder: replaceAnyOrder,
					Dependencies: addrs.MakeSet(objAddr("a"), objAddr("b")),
				})
				objs.Put(&resourceInstanceObject{
					Addr:         objAddr("c"),
					ReplaceOrder: replaceAnyOrder,
				})
			},
			addrs.MakeMap(
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("a"),
					Value: replaceDestroyThenCreate,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("b"),
					Value: replaceDestroyThenCreate,
				},
				addrs.MapElem[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder]{
					Key:   objAddr("c"),
					Value: replaceDestroyThenCreate,
				},
			),
			addrs.MakeSet(
				objAddr("a"),
				objAddr("b"),
			),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			builder := newResourceInstanceObjectsBuilder()
			test.build(builder)
			objs := builder.Close()

			got, gotSelfDeps := findEffectiveReplaceOrders(objs)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Error("wrong result\n" + diff)
			}
			if diff := cmp.Diff(test.wantSelfDeps, gotSelfDeps); diff != "" {
				t.Error("wrong self-dependencies\n" + diff)
			}
		})
	}
}
