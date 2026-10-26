// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

type PersistState func(*states.State) error

// PersistingState is a wrapper around SyncState that
// persists the working copy to whatever persistence callback
// is defined.
//
// Critically, this assumes that the persist callback does
// not modify the state while the apply engine is running.
// In the current shims to the local backend, the callback
// overwrites the StateManager's current state and calls
// Persist. Nothing else during the apply process reads or
// modifies the state through the state manager, which means
// that this approach is safe.
//
// This additional complexity and potential fragility here
// is in service of preventing as many state.DeepCopy() calls
// as possible. If we did not share the lock between state sync
// and persisting below, persist would need to perform it's own
// deep copy of the state as modifications here proceed.
//
// A potentially better approach would be to share the StateManager
// from the local backend with this implementation, but that would blur
// some of the intentional separation of concerns. Ideally, this
// all goes away once we start work on granular state storage.
// That will necessitate re-engineering all of these abstractions
// anyways, and is not worth over-complicating at this point in time.
type PersistingState struct {
	sync *states.SyncState

	persist func() error
}

func NewPersistingState(working *states.State, persist PersistState) *PersistingState {
	sync := working.SyncWrapper()
	return &PersistingState{
		sync: sync,
		persist: func() error {
			sync.Lock()
			defer sync.Unlock()
			// We can safely access the original working state while the sync lock is held
			return persist(working)
		},
	}
}

func (s *PersistingState) SetResourceInstanceObjectFull(addr addrs.AbsResourceInstanceObject, obj *states.ResourceInstanceObjectFullSrc) error {
	s.sync.SetResourceInstanceObjectFull(addr, obj)
	return s.persist()
}

func (s *PersistingState) RemoveResourceInstanceObjectFull(addr addrs.AbsResourceInstanceObject, providerInstAddr addrs.AbsProviderInstanceCorrect) error {
	s.sync.RemoveResourceInstanceObjectFull(addr, providerInstAddr)
	return s.persist()
}

func (s *PersistingState) MaybeRestoreResourceInstanceDeposed(addr addrs.AbsResourceInstance, key states.DeposedKey) (bool, error) {
	ret := s.sync.MaybeRestoreResourceInstanceDeposed(addr, key)
	return ret, s.persist()
}

func (s *PersistingState) DeposeResourceInstanceObjectForceKey(addr addrs.AbsResourceInstance, forcedKey states.DeposedKey) error {
	s.sync.DeposeResourceInstanceObjectForceKey(addr, forcedKey)
	return s.persist()
}

func (s *PersistingState) MaybeMoveResourceInstance(src, dst addrs.AbsResourceInstance) (bool, error) {
	ret := s.sync.MaybeMoveResourceInstance(src, dst)
	return ret, s.persist()
}

func (s *PersistingState) Close() *states.State {
	return s.sync.Close()
}
