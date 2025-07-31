// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

// ChangesSync is a wrapper around a Changes that provides a concurrency-safe
// interface to insert new changes and retrieve copies of existing changes.
//
// Each ChangesSync is independent of all others, so all concurrent writers
// to a particular Changes must share a single ChangesSync. Behavior is
// undefined if any other caller makes changes to the underlying Changes
// object or its nested objects concurrently with any of the methods of a
// particular ChangesSync.
type ChangesSync struct {
	lock    sync.Mutex
	changes *Changes
}

// AppendResourceInstanceChange records the given resource instance change in
// the set of planned resource changes.
//
// The caller must ensure that there are no concurrent writes to the given
// change while this method is running, but it is safe to resume mutating
// it after this method returns without affecting the saved change.
func (cs *ChangesSync) AppendResourceInstanceChange(changeSrc *ResourceInstanceChangeSrc) {
	if cs == nil {
		panic("AppendResourceInstanceChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	s := changeSrc.DeepCopy()
	cs.changes.Resources = append(cs.changes.Resources, s)
}

// GetResourceInstanceChange searches the set of resource instance changes for
// one matching the given address and generation, returning it if it exists.
//
// If no such change exists, nil is returned.
//
// The returned object is a deep copy of the change recorded in the plan, so
// callers may mutate it although it's generally better (less confusing) to
// treat planned changes as immutable after they've been initially constructed.
func (cs *ChangesSync) GetResourceInstanceChange(addr addrs.AbsResourceInstance, gen states.Generation) *ResourceInstanceChangeSrc {
	if cs == nil {
		panic("GetResourceInstanceChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if gen == states.CurrentGen {
		return cs.changes.ResourceInstance(addr).DeepCopy()
	}
	if dk, ok := gen.(states.DeposedKey); ok {
		return cs.changes.ResourceInstanceDeposed(addr, dk).DeepCopy()
	}
	panic(fmt.Sprintf("unsupported generation value %#v", gen))
}

// GetChangesForConfigResource searches the set of resource instance
// changes and returns all changes related to a given configuration address.
// This is be used to find possible changes related to a configuration
// reference.
//
// If no such changes exist, nil is returned.
//
// The returned objects are a deep copy of the change recorded in the plan, so
// callers may mutate them although it's generally better (less confusing) to
// treat planned changes as immutable after they've been initially constructed.
func (cs *ChangesSync) GetChangesForConfigResource(addr addrs.ConfigResource) []*ResourceInstanceChangeSrc {
	if cs == nil {
		panic("GetChangesForConfigResource on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()
	var changes []*ResourceInstanceChangeSrc
	for _, c := range cs.changes.InstancesForConfigResource(addr) {
		changes = append(changes, c.DeepCopy())
	}
	return changes
}

// GetChangesForAbsResource searches the set of resource instance
// changes and returns all changes related to a given configuration address.
//
// If no such changes exist, nil is returned.
//
// The returned objects are a deep copy of the change recorded in the plan, so
// callers may mutate them although it's generally better (less confusing) to
// treat planned changes as immutable after they've been initially constructed.
func (cs *ChangesSync) GetChangesForAbsResource(addr addrs.AbsResource) []*ResourceInstanceChangeSrc {
	if cs == nil {
		panic("GetChangesForAbsResource on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()
	var changes []*ResourceInstanceChangeSrc
	for _, c := range cs.changes.InstancesForAbsResource(addr) {
		changes = append(changes, c.DeepCopy())
	}
	return changes
}

// RemoveResourceInstanceChange searches the set of resource instance changes
// for one matching the given address and generation, and removes it from the
// set if it exists.
func (cs *ChangesSync) RemoveResourceInstanceChange(addr addrs.AbsResourceInstance, gen states.Generation) {
	if cs == nil {
		panic("RemoveResourceInstanceChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	dk := states.NotDeposed
	if realDK, ok := gen.(states.DeposedKey); ok {
		dk = realDK
	}

	for i, r := range cs.changes.Resources {
		if !r.Addr.Equal(addr) || r.DeposedKey != dk {
			continue
		}
		copy(cs.changes.Resources[i:], cs.changes.Resources[i+1:])
		cs.changes.Resources = cs.changes.Resources[:len(cs.changes.Resources)-1]
		return
	}
}

// AppendOutputChange records the given output value change in the set of
// planned value changes.
//
// The caller must ensure that there are no concurrent writes to the given
// change while this method is running, but it is safe to resume mutating
// it after this method returns without affecting the saved change.
func (cs *ChangesSync) AppendOutputChange(changeSrc *OutputChangeSrc) {
	if cs == nil {
		panic("AppendOutputChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	s := changeSrc.DeepCopy()
	cs.changes.Outputs = append(cs.changes.Outputs, s)
}

// GetOutputChange searches the set of output value changes for one matching
// the given address, returning it if it exists.
//
// If no such change exists, nil is returned.
//
// The returned object is a deep copy of the change recorded in the plan, so
// callers may mutate it although it's generally better (less confusing) to
// treat planned changes as immutable after they've been initially constructed.
func (cs *ChangesSync) GetOutputChange(addr addrs.AbsOutputValue) *OutputChangeSrc {
	if cs == nil {
		panic("GetOutputChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	return cs.changes.OutputValue(addr)
}

// GetRootOutputChanges searches the set of output changes for any that reside
// the root module. If no such changes exist, nil is returned.
//
// The returned objects are a deep copy of the change recorded in the plan, so
// callers may mutate them although it's generally better (less confusing) to
// treat planned changes as immutable after they've been initially constructed.
func (cs *ChangesSync) GetRootOutputChanges() []*OutputChangeSrc {
	if cs == nil {
		panic("GetRootOutputChanges on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	return cs.changes.RootOutputValues()
}

// GetOutputChanges searches the set of output changes for any that reside in
// module instances beneath the given module. If no changes exist, nil
// is returned.
//
// The returned objects are a deep copy of the change recorded in the plan, so
// callers may mutate them although it's generally better (less confusing) to
// treat planned changes as immutable after they've been initially constructed.
func (cs *ChangesSync) GetOutputChanges(parent addrs.ModuleInstance, module addrs.ModuleCall) []*OutputChangeSrc {
	if cs == nil {
		panic("GetOutputChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	return cs.changes.OutputValues(parent, module)
}

// RemoveOutputChange searches the set of output value changes for one matching
// the given address, and removes it from the set if it exists.
func (cs *ChangesSync) RemoveOutputChange(addr addrs.AbsOutputValue) {
	if cs == nil {
		panic("RemoveOutputChange on nil ChangesSync")
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	for i, o := range cs.changes.Outputs {
		if !o.Addr.Equal(addr) {
			continue
		}
		copy(cs.changes.Outputs[i:], cs.changes.Outputs[i+1:])
		cs.changes.Outputs = cs.changes.Outputs[:len(cs.changes.Outputs)-1]
		return
	}
}
