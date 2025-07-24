// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestoreshim

import (
	"iter"

	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states/statekeys"
	"github.com/opentofu/opentofu/internal/states/statestore"
)

// StateLockKeysForPlan returns an interable sequence of state keys that
// the given plan relies on.
//
// The second boolean result of each item represents whether the lock
// should be exclusive (true) or shared (false).
//
// The result may contain duplicate keys, including possibly the same key
// reported as both shared and exclusive. If the same key is reported as
// both shared and exclusive then the caller must treat it as exclusive.
func StateLockKeysForPlan(plan *plans.Plan) iter.Seq2[statekeys.Key, bool] {
	return func(yield func(statekeys.Key, bool) bool) {
		for _, change := range plan.Changes.Resources {
			instKey := statekeys.NewResourceInstance(change.Addr)
			configKey := statekeys.NewResource(change.Addr.ConfigResource())
			exclusive := change.Action != plans.NoOp
			if wantMore := yield(instKey, exclusive); !wantMore {
				return
			}
			if wantMore := yield(configKey, exclusive); !wantMore {
				return
			}
		}
		for _, change := range plan.Changes.Outputs {
			if change.Action == plans.NoOp || !change.Addr.Module.IsRoot() {
				continue
			}
			key := statekeys.NewRootModuleOutputValue(change.Addr)
			if wantMore := yield(key, true); !wantMore {
				return
			}
		}
	}
}

func CollectStateItemValuesForPlan(allValues statestore.ValueHashes, needKeys iter.Seq2[statekeys.Key, bool]) (shared, exclusive statestore.ValueHashes) {
	shared = make(statestore.ValueHashes)
	exclusive = make(statestore.ValueHashes)
	for key, needExclusive := range needKeys {
		storeKey := key.ForStorage()
		hash, ok := allValues[storeKey]
		if !ok {
			hash = statestore.NoValueHash
		}
		if needExclusive {
			delete(shared, storeKey) // exclusive supersedes shared
			exclusive[storeKey] = hash
		} else if _, alreadyExclusive := exclusive[storeKey]; !alreadyExclusive {
			shared[storeKey] = hash
		}
	}
	return shared, exclusive
}
