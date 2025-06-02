// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestoreshim

import (
	"iter"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/states/statekeys"
)

// LockKeysForConfig returns an iterable sequence of state storage keys that
// evaluation of the given configuration would definitely rely on.
//
// This gives a starting point for acquiring shared locks during the planning
// phase, but the result is incomplete because the current configuration gives
// only a partial representation of the desired state and doesn't represent
// the prior state at all. After acquiring shared locks for all of the
// keys returned here a caller will need to then enumerate all of the keys
// currently stored and acquire additional locks on those before proceeding.
//
// Acquiring locks based on the configuration before enumerating the keys
// currently stored ensures that the presence or absence of the state keys
// associated with each object remains constant while we list what's currently
// stored, to avoid another process writing an object that we're expecting would
// remain absent as we plan to create it.
func LockKeysForConfig(config *configs.Config) iter.Seq[statekeys.Key] {
	return func(yield func(statekeys.Key) bool) {
		if !yieldLockKeysForResources(config, yield) {
			return
		}
		if !yieldLockKeysForRootOutputValues(config, yield) {
			return
		}
	}
}

func yieldLockKeysForResources(config *configs.Config, yield func(statekeys.Key) bool) bool {
	moduleAddr := config.Path
	for _, rsrc := range config.Module.ManagedResources {
		addr := rsrc.Addr().InModule(moduleAddr)
		key := statekeys.NewResource(addr)
		if !yield(key) {
			return false
		}
	}
	for _, rsrc := range config.Module.DataResources {
		addr := rsrc.Addr().InModule(moduleAddr)
		key := statekeys.NewResource(addr)
		if !yield(key) {
			return false
		}
	}
	return true
}

func yieldLockKeysForRootOutputValues(config *configs.Config, yield func(statekeys.Key) bool) bool {
	for name := range config.Module.Outputs {
		key := statekeys.NewRootModuleOutputValue(addrs.AbsOutputValue{
			Module: addrs.RootModuleInstance,
			OutputValue: addrs.OutputValue{
				Name: name,
			},
		})
		if !yield(key) {
			return false
		}
	}
	return true
}
