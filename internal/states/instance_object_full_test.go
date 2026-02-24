// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package states

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

// The tests in here are currently pretty minimal as a compromise while we're
// still trying to learn if this new representation is even a viable approach.
// The focus is mainly on whether we're correcting translating from and to the
// traditional old state representation, since we'll still be using the old
// models as our primary representation for now so we can continue to use
// unmodified state manager implementations, etc.
//
// TODO: If these new codepaths survive beyond the early exploratory work
// for the new language runtime then we should consider what fuller testing
// might be helpful here, and implement it.

func TestSyncStateResourceInstanceObjectFull(t *testing.T) {
	instAddrRel := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "baz",
	}.Instance(addrs.NoKey)
	instAddrAbs := instAddrRel.Absolute(addrs.RootModuleInstance)
	providerInstAddr := addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Module: addrs.RootModuleInstance.Child("mod", addrs.NoKey),
			Config: addrs.ProviderConfigCorrect{
				Provider: addrs.NewBuiltInProvider("foo"),
			},
		},
	}
	depAddr := addrs.ConfigResource{
		Resource: addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "foo",
			Name: "dependency",
		},
	}
	deposedKey := DeposedKey("...")
	legacyProviderConfigAddr := addrs.AbsProviderConfig{
		Module:   providerInstAddr.Config.Module.Module(),
		Provider: providerInstAddr.Config.Config.Provider,
	}
	objTy := cty.Object(map[string]cty.Type{
		"a": cty.Number,
		"b": cty.Number,
	})

	s := NewState()
	s.RootModule().SetResourceInstanceCurrent(instAddrRel, &ResourceInstanceObjectSrc{
		Status:    ObjectReady,
		AttrsJSON: []byte(`{"a":1,"b":2}`),
		AttrSensitivePaths: []cty.PathValueMarks{
			{Path: cty.GetAttrPath("b"), Marks: cty.NewValueMarks(marks.Sensitive)},
		},
		Private:             []byte(`private`),
		CreateBeforeDestroy: true,
		SchemaVersion:       5,
		Dependencies: []addrs.ConfigResource{
			depAddr,
		},
	}, legacyProviderConfigAddr, addrs.NoKey)
	s.RootModule().SetResourceInstanceDeposed(instAddrRel, deposedKey, &ResourceInstanceObjectSrc{
		Status:    ObjectReady,
		AttrsJSON: []byte(`{"a":0,"b":1}`),
	}, legacyProviderConfigAddr, addrs.NoKey)

	ss := s.SyncWrapper()

	t.Run("current object", func(t *testing.T) {
		gotObjSrc := ss.ResourceInstanceObjectFull(instAddrAbs, NotDeposed)
		gotObj, err := DecodeResourceInstanceObjectFull(gotObjSrc, objTy)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		wantObj := &ResourceInstanceObjectFull{
			Status: ObjectReady,
			Value: cty.ObjectVal(map[string]cty.Value{
				"a": cty.NumberIntVal(1),
				"b": cty.NumberIntVal(2).Mark(marks.Sensitive),
			}),
			Private:              []byte(`private`),
			CreateBeforeDestroy:  true,
			SchemaVersion:        5,
			ProviderInstanceAddr: providerInstAddr,
			ResourceType:         "foo",
			Dependencies: []addrs.ConfigResource{
				depAddr,
			},
		}
		if diff := cmp.Diff(wantObj, gotObj, ctydebug.CmpOptions); diff != "" {
			t.Error("wrong result\n" + diff)
		}
	})
	t.Run("deposed object", func(t *testing.T) {
		gotObjSrc := ss.ResourceInstanceObjectFull(instAddrAbs, deposedKey)
		gotObj, err := DecodeResourceInstanceObjectFull(gotObjSrc, objTy)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		wantObj := &ResourceInstanceObjectFull{
			Status: ObjectReady,
			Value: cty.ObjectVal(map[string]cty.Value{
				"a": cty.NumberIntVal(0),
				"b": cty.NumberIntVal(1),
			}),
			ProviderInstanceAddr: providerInstAddr,
			ResourceType:         "foo",
		}
		if diff := cmp.Diff(wantObj, gotObj, ctydebug.CmpOptions); diff != "" {
			t.Error("wrong result\n" + diff)
		}
	})
}

func TestSyncStateSetResourceInstanceObjectFull(t *testing.T) {
	instAddrRel := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "baz",
	}.Instance(addrs.NoKey)
	instAddrAbs := instAddrRel.Absolute(addrs.RootModuleInstance)
	providerInstAddr := addrs.AbsProviderInstanceCorrect{
		Config: addrs.AbsProviderConfigCorrect{
			Module: addrs.RootModuleInstance.Child("mod", addrs.NoKey),
			Config: addrs.ProviderConfigCorrect{
				Provider: addrs.NewBuiltInProvider("foo"),
			},
		},
	}
	depAddr := addrs.ConfigResource{
		Resource: addrs.Resource{
			Mode: addrs.ManagedResourceMode,
			Type: "foo",
			Name: "dependency",
		},
	}
	deposedKey := DeposedKey("...")
	legacyProviderConfigAddr := addrs.AbsProviderConfig{
		Module:   providerInstAddr.Config.Module.Module(),
		Provider: providerInstAddr.Config.Config.Provider,
	}
	objTy := cty.Object(map[string]cty.Type{
		"a": cty.Number,
		"b": cty.Number,
	})

	currentObj := &ResourceInstanceObjectFull{
		Status: ObjectReady,
		Value: cty.ObjectVal(map[string]cty.Value{
			"a": cty.NumberIntVal(1),
			"b": cty.NumberIntVal(2).Mark(marks.Sensitive),
		}),
		Private:              []byte(`private`),
		CreateBeforeDestroy:  true,
		SchemaVersion:        5,
		ProviderInstanceAddr: providerInstAddr,
		ResourceType:         "foo",
		Dependencies: []addrs.ConfigResource{
			depAddr,
		},
	}
	deposedObj := &ResourceInstanceObjectFull{
		Status: ObjectReady,
		Value: cty.ObjectVal(map[string]cty.Value{
			"a": cty.NumberIntVal(0),
			"b": cty.NumberIntVal(1),
		}),
		ProviderInstanceAddr: providerInstAddr,
		ResourceType:         "foo",
	}
	currentObjSrc, err := EncodeResourceInstanceObjectFull(currentObj, objTy)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	deposedObjSrc, err := EncodeResourceInstanceObjectFull(deposedObj, objTy)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	gotState := BuildState(func(ss *SyncState) {
		ss.SetResourceInstanceObjectFull(instAddrAbs, deposedKey, deposedObjSrc)
		ss.SetResourceInstanceObjectFull(instAddrAbs, NotDeposed, currentObjSrc)
	})
	wantState := &State{
		Modules: map[string]*Module{
			"": {
				Addr: addrs.RootModuleInstance,
				Resources: map[string]*Resource{
					"foo.baz": {
						Addr:           instAddrAbs.ContainingResource(),
						ProviderConfig: legacyProviderConfigAddr,
						Instances: map[addrs.InstanceKey]*ResourceInstance{
							addrs.NoKey: {
								Current: &ResourceInstanceObjectSrc{
									Status:    ObjectReady,
									AttrsJSON: []byte(`{"a":1,"b":2}`),
									AttrSensitivePaths: []cty.PathValueMarks{
										{Path: cty.GetAttrPath("b"), Marks: cty.NewValueMarks(marks.Sensitive)},
									},
									Private:             []byte(`private`),
									CreateBeforeDestroy: true,
									SchemaVersion:       5,
									Dependencies: []addrs.ConfigResource{
										depAddr,
									},
								},
								Deposed: map[DeposedKey]*ResourceInstanceObjectSrc{
									deposedKey: {
										Status:    ObjectReady,
										AttrsJSON: []byte(`{"a":0,"b":1}`),
									},
								},
							},
						},
					},
				},
				LocalValues:  make(map[string]cty.Value),
				OutputValues: make(map[string]*OutputValue),
			},
		},
	}
	if diff := cmp.Diff(wantState, gotState); diff != "" {
		t.Error("wrong result\n" + diff)
	}

}
