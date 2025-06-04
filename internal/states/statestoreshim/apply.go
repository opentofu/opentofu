// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestoreshim

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statekeys"
	"github.com/opentofu/opentofu/internal/states/statestore"
	"github.com/opentofu/opentofu/internal/tofu"
)

// PrepareToApplyPlan acquires the state locks needed to apply the given plan,
// returning a set of keys that were acquired so that the caller can unlock
// them again once the apply phase is complete.
//
// Before returning, this function also verifies that the hashes recorded for
// each state key in the plan file are still consistent with the stored values.
// If any inconsistencies are found then this returns an error after making a
// best effort to unlock all of the acquired locks.
func PrepareToApplyPlan(ctx context.Context, plan *plans.Plan, stateStore statestore.Storage) (statestore.KeySet, error) {
	sharedLockKeys := plan.StateLocksShared.Keys()
	exclusiveLockKeys := plan.StateLocksExclusive.Keys()
	allKeys := make(statestore.KeySet, len(sharedLockKeys)+len(exclusiveLockKeys))
	for k := range sharedLockKeys {
		allKeys[k] = struct{}{}
	}
	for k := range exclusiveLockKeys {
		allKeys[k] = struct{}{}
	}

	err := stateStore.Lock(ctx, sharedLockKeys, exclusiveLockKeys)
	if err != nil {
		return nil, fmt.Errorf("acquiring locks: %w", err)
	}

	// Before we return we need to fetch all of the objects we've just locked
	// and verify that they still have the values that they had when the
	// plan was created. If not, then the plan has been invalidated by applying
	// some other plan first.
	values, err := stateStore.Read(ctx, allKeys)
	if err != nil {
		_ = stateStore.Unlock(ctx, allKeys) // best effort to return with everything unlocked
		return nil, fmt.Errorf("reading state data: %w", err)
	}

	for key, wantHash := range plan.StateLocksShared {
		gotHash := values[key].Hash()
		if wantHash != gotHash {
			_ = stateStore.Unlock(ctx, allKeys) // best effort to return with everything unlocked
			return nil, fmt.Errorf("another plan has been applied that has invalidated this one")
		}
	}
	for key, wantHash := range plan.StateLocksExclusive {
		gotHash := values[key].Hash()
		if wantHash != gotHash {
			_ = stateStore.Unlock(ctx, allKeys) // best effort to return with everything unlocked
			return nil, fmt.Errorf("another plan has been applied that has invalidated this one")
		}
	}

	return allKeys, nil
}

// NewStateUpdateHook returns a [tofu.Hook] implementation which reacts to
// [tofu.Hook.StateValueChanged] by writing the updated object to the given
// state storage.
//
// Before using the resulting hook the caller must acquire all of the needed
// exclusive locks to allow the affected objects to be written. Use
// [PrepareToApplyPlan] to acquire all of the needed locks.
func NewStateUpdateHook(stateStore statestore.Storage) tofu.Hook {
	return stateUpdateHook{stateStore, nil}
}

type stateUpdateHook struct {
	store statestore.Storage
	*tofu.NilHook
}

func (h stateUpdateHook) StateValueChanged(key statekeys.Key, state *states.State) error {
	storeKey := key.ForStorage()
	log.Printf("[TRACE] statestoreshim: state value has changed for %q", storeKey.Name())
	switch key := key.(type) {
	case statekeys.Resource:
		// We're currently using statekeys.Resource only for locking purposes
		// and don't have any real need to store values against it, so we'll
		// just ignore this case. (At the time of writing there's no caller
		// that would pass a key of this type in here anyway.)
		return nil
	case statekeys.ResourceInstance:
		r := state.Resource(key.Address().ContainingResource())
		instKey := key.Address().Resource.Key
		storeValue, err := encodeStateResourceInstance(key, r, instKey)
		if err != nil {
			return err
		}
		return h.store.Write(context.TODO(), map[statestore.Key]statestore.Value{
			storeKey: storeValue,
		})
	case statekeys.RootModuleOutputValue:
		ov := state.OutputValue(key.Address())
		storeValue, err := encodeStateRootOutputValue(key, ov)
		if err != nil {
			return err
		}
		return h.store.Write(context.TODO(), map[statestore.Key]statestore.Value{
			storeKey: storeValue,
		})
	default:
		return nil
	}
}
