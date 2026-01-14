// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type syncState struct {
	// resourceInstanceObjects tracks the live state for each resource instance
	// object that currently exists.
	//
	// This map may only be shallowly mutated: objects must be replaced
	// wholesale with replacement objects rather than mutating the content
	// of the objects in-place, so that other parts of the system can safely
	// use pointers they retrieved from here without having to coordinate
	// with each other to avoid data races.
	resourceInstanceObjects map[resourceInstanceObjectKey]*states.ResourceInstanceObjectFull
	mu                      sync.Mutex
}

// syncStateFromPriorState converts a [states.State] into a [syncState] by
// proactively unmarshaling all of the objects, thereby allowing us to catch
// any deserialization errors up front at the start and thus the rest of the
// apply engine can work with live objects that are already known to be at
// least valid enough to decode with the provider's schema.
//
// FIXME: This is currently pre-decoding all of the objects in the prior state
// largely because the execution graph model currently thinks of fetching
// prior state as an infallible lookup, and so this deals with all of the
// potential errors up front to honor that expectation. However, that means
// we're potentially wasting time decoding things we don't actually need to
// decode, and also this wouldn't work in a hypothetical future granular state
// storage model where the state for each object is loaded independently from
// the store. In future commits we'll change the execution graph structure so
// that loading prior state is modelled as a true "operation" (a fallible action
// with externally-visible side-effects) and then we can do this decoding
// work gradually as needed during execution.
func syncStateFromPriorState(ctx context.Context, priorState *states.State, providers plugins.Providers) (*syncState, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// FIXME: The logic for building a "full" resource instance object currently
	// lives on SyncState rather than State, but yet we need to use State to
	// enumerate all of the objects that exist, so we currently end up needing
	// to use both at once.
	ss := priorState.SyncWrapper()

	objs := make(map[resourceInstanceObjectKey]*states.ResourceInstanceObjectFull)
	for _, ms := range priorState.Modules {
		for _, rs := range ms.Resources {
			for key, is := range rs.Instances {
				instAddr := rs.Addr.Instance(key)
				for deposedKey := range instanceObjectKeys(is) {
					src := ss.ResourceInstanceObjectFull(instAddr, deposedKey)

					schema, moreDiags := providers.ResourceTypeSchema(ctx,
						src.ProviderInstanceAddr.Config.Config.Provider,
						instAddr.Resource.Resource.Mode,
						instAddr.Resource.Resource.Type,
					)
					diags = diags.Append(moreDiags)
					if moreDiags.HasErrors() {
						continue
					}
					if schema == nil {
						// TODO: a proper error diagnostic for this
						diags = diags.Append(fmt.Errorf(
							"no schema available for %q in %s",
							instAddr.Resource.Resource.Mode,
							src.ProviderInstanceAddr.Config.Config.Provider,
						))
						continue
					}

					obj, err := states.DecodeResourceInstanceObjectFull(src, schema.Block.ImpliedType())
					if err != nil {
						// TODO: a proper error diagnostic for this
						diags = diags.Append(err)
						continue
					}

					key := newResourceInstanceObjectKey(instAddr, deposedKey)
					objs[key] = obj
				}
			}
		}
	}
	return &syncState{
		resourceInstanceObjects: objs,
	}, diags
}

func (s *syncState) ResourceInstanceObject(instAddr addrs.AbsResourceInstance, deposedKey states.DeposedKey) *states.ResourceInstanceObjectFull {
	key := newResourceInstanceObjectKey(instAddr, deposedKey)
	s.mu.Lock()
	ret := s.resourceInstanceObjects[key]
	s.mu.Unlock()
	return ret
}

// Copy produces a new [syncState] which has a distinct map of resource instance
// objects but which initially shares all of the objects in that map with the
// source map.
//
// It's safe to modify the result through its public API as long as everyone is
// careful to preserve the guarantee that [states.ResourceInstanceObjectFull]
// objects are treated as immutable and just swapped out wholesale for separate
// new objects.
func (s *syncState) Copy() *syncState {
	objs := make(map[resourceInstanceObjectKey]*states.ResourceInstanceObjectFull)

	// We'll hold the source object's lock while we copy its map to make sure
	// that nobody is concurrently modifying it.
	s.mu.Lock()
	maps.Copy(objs, s.resourceInstanceObjects)
	s.mu.Unlock()

	return &syncState{
		resourceInstanceObjects: objs,
	}
}

type resourceInstanceObjectKey struct {
	instAddr   addrs.UniqueKey
	deposedkey states.DeposedKey
}

func newResourceInstanceObjectKey(instAddr addrs.AbsResourceInstance, deposedKey states.DeposedKey) resourceInstanceObjectKey {
	return resourceInstanceObjectKey{
		instAddr:   instAddr.UniqueKey(),
		deposedkey: deposedKey,
	}
}

// instanceObjectKeys iterates over all of the [states.DeposedKey] values that
// identify objects belonging to the given resource instance, including
// [states.NotDeposed] if the instance has a "current" object.
func instanceObjectKeys(is *states.ResourceInstance) iter.Seq[states.DeposedKey] {
	return func(yield func(states.DeposedKey) bool) {
		if is.Current != nil {
			if !yield(states.NotDeposed) {
				return
			}
		}
		for dk := range is.Deposed {
			if !yield(dk) {
				return
			}
		}
	}
}
