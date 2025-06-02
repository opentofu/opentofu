// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestoreshim

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zclconf/go-cty/cty"
	ctymsgpack "github.com/zclconf/go-cty/cty/msgpack"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statekeys"
	"github.com/opentofu/opentofu/internal/states/statestore"
)

// LoadPriorState constructs a [*states.State] object from the data in the given
// [statestore.Storage], acquiring advisory locks as needed.
//
// Pass any keys for which there are already active shared locks in haveLocks.
//
// The second result is a map of the hashes of all of the entries used to build
// the result, describing what ought to remain unchanged in order for any
// plan created against the current state data to remain valid. If this function
// returns successfully then all locks have been released, including the ones
// provided in haveLocks, since the returned state combined with the map of
// state value hashes is enough to create a plan and recognize at apply time
// whether it remains valid to apply.
//
// This effectively forces loading the entire content of the state storage
// at once and locking all of the keys, which defeats some of the advantages
// of granular state storage but is useful for an initial implementation that
// continues to treat state storage as a concern handled only outside of the
// language runtime in the CLI layer, so that we can pass the entire prior
// state into the language runtime at once as it currently expects.
//
// This function acquires only shared locks and not exclusive locks, so it
// does still potentially allow two plan-like operations to run concurrently
// but will fail if another process is already holding exclusive locks on any
// part of the existing state, suggesting that an apply-like operation is
// in progress.
func LoadPriorState(ctx context.Context, storage statestore.Storage, haveLocks statestore.KeySet) (*states.State, map[statestore.Key][sha256.Size]byte, error) {
	if haveLocks == nil {
		haveLocks = make(statestore.KeySet)
	}

	// We'll start by acquiring shared locks for everything that's listed as
	// currently present in the storage, which will effectively freeze the
	// presence or absense of those objects while we work on loading the
	// associated data, since otherwise the set of keys that are present
	// could change while we're working.
	// (Despite all of this locking it's possible that an entirely new object
	// could appear in the storage concurrent with our work, but that's okay
	// because we're not going to take any actions that would conflict with
	// such an object.)
	needLocks := make(statestore.KeySet)
	for storageKey, err := range storage.Keys(ctx) {
		if err != nil {
			return nil, nil, fmt.Errorf("listing keys: %w", err)
		}
		if !haveLocks.Has(storageKey) {
			needLocks[storageKey] = struct{}{}
		}
	}

	err := storage.Lock(ctx, needLocks, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("acquiring locks: %w", err)
	}
	for storageKey := range needLocks {
		haveLocks[storageKey] = struct{}{}
	}

	rawEntries, err := storage.Read(ctx, haveLocks)
	if err != nil {
		return nil, nil, fmt.Errorf("reading from state: %w", err)
	}

	state := states.NewState()
	ss := state.SyncWrapper() // just because its mutation API is more convenient to use here
	err = nil                 // we'll use errors.Join to potentially collect multiple errors here below
	for storageKey, rawValue := range rawEntries {
		if rawValue.IsNoValue() {
			// We ignore entries that have no associated value. This could
			// happen if an entry were deleted by a concurrent process before
			// we managed to acquire the lock on it, in which case that's
			// fine because we're still holding a lock and so we can assume
			// that it will _stay_ gone for as long as we hold the lock.
		}
		key, keyErr := statekeys.KeyFromStorage(storageKey)
		if keyErr != nil {
			err = errors.Join(err, fmt.Errorf("unsupported state content: %w", keyErr))
			continue
		}
		switch key := key.(type) {
		case statekeys.Resource:
			_, decErr := decodeStateResource(key, rawValue)
			if err != nil {
				err = errors.Join(err, fmt.Errorf("invalid state entry for %s: %w", key.Address(), decErr))
				continue
			}
			// We don't currently do anything with resource keys; we keep them
			// primarily so we can hold a lock even for a resource that currently
			// has zero instances to prevent a race to create the first instance,
			// but maybe we'll store other interesting resource-level metadata
			// here someday.
		case statekeys.ResourceInstance:
			ri, providerAddr, decErr := decodeStateResourceInstance(key, rawValue)
			if err != nil {
				err = errors.Join(err, fmt.Errorf("invalid state entry for %s: %w", key.Address(), decErr))
				continue
			}
			ss.SetResourceProvider(key.Address().ContainingResource(), providerAddr)
			ss.SetResourceInstanceCurrent(key.Address(), ri.Current, providerAddr, ri.ProviderKey)
		case statekeys.RootModuleOutputValue:
			ov, decErr := decodeStateRootOutputValue(key, rawValue)
			if err != nil {
				err = errors.Join(err, fmt.Errorf("invalid state entry for %s: %w", key.Address(), decErr))
				continue
			}
			ss.SetOutputValue(ov.Addr, ov.Value, ov.Sensitive, ov.Deprecated)
		default:
			// Should not get here because the cases above should cover all of
			// the key types supported by [statekeys].
			return nil, nil, fmt.Errorf("unhandled state key type %T", key)
		}
	}

	hashes := make(map[statestore.Key][sha256.Size]byte, len(rawEntries))
	for key, value := range rawEntries {
		hashes[key] = value.Hash()
	}

	// We don't need to continue holding the locks, because we now have enough
	// information in hashes to recognize if anything has changed before the
	// plan is applied. In a more deeply-integrated version of this we'd
	// probably continue holding the locks throughout the plan walk and request
	// state objects individually as we need them, but this simpler approach
	// is sufficient for the non-integrated version since holding the lock
	// throughout the plan phase wouldn't cause a significantly different
	// result: it would just cause a concurrent apply phase to get blocked
	// until we've finished creating the plan and then immediately invalidate
	// our plan anyway.
	unlockErr := storage.Unlock(ctx, haveLocks)
	err = errors.Join(err, unlockErr)

	return state, hashes, err
}

func decodeStateRootOutputValue(key statekeys.RootModuleOutputValue, raw statestore.Value) (*states.OutputValue, error) {
	type Encoded struct {
		EncodedValue []byte `msgpack:"value"`
		Sensitive    bool   `msgpack:"sensitive"`
		Deprecated   string `msgpack:"deprecated"`
	}
	var encoded Encoded
	err := msgpack.Unmarshal(raw, &encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata encoding: %w", err)
	}

	val, err := ctymsgpack.Unmarshal(encoded.EncodedValue, cty.DynamicPseudoType)
	if err != nil {
		return nil, fmt.Errorf("invalid value encoding: %w", err)
	}

	return &states.OutputValue{
		Addr:       key.Address(),
		Value:      val,
		Sensitive:  encoded.Sensitive,
		Deprecated: encoded.Deprecated,
	}, nil
}

func decodeStateResourceInstance(_ statekeys.ResourceInstance, raw statestore.Value) (*states.ResourceInstance, addrs.AbsProviderConfig, error) {
	type EncodedObject struct {
		SchemaVersion       uint64            `msgpack:"schema_version"`
		AttrsJSON           []byte            `msgpack:"attributes"`
		AttrsFlat           map[string]string `msgpack:"legacy_attributes"`
		Dependencies        []string          `msgpack:"dependencies"`
		Status              string            `msgpack:"status"`
		CreateBeforeDestroy bool              `msgpack:"create_before_destroy"`
		Private             []byte            `msgpack:"private"`
		// TODO: Sensitive attribute paths
	}
	type Encoded struct {
		CurrentObject  *EncodedObject           `msgpack:"current"`
		DeposedObjects map[string]EncodedObject `msgpack:"deposed"`

		// Tech debt from the provider for_each project: we still don't actually
		// have a proper address type for a fully-qualified provider instance. :(
		ProviderAddr        string `msgpack:"provider"`
		ProviderInstanceKey any    `msgpack:"provider_instance_key"`
	}
	var encoded Encoded
	err := msgpack.Unmarshal(raw, &encoded)
	if err != nil {
		return nil, addrs.AbsProviderConfig{}, fmt.Errorf("invalid metadata encoding: %w", err)
	}

	ret := &states.ResourceInstance{}
	if encoded.CurrentObject != nil {
		obj := &states.ResourceInstanceObjectSrc{
			SchemaVersion:       encoded.CurrentObject.SchemaVersion,
			AttrsJSON:           encoded.CurrentObject.AttrsJSON,
			AttrsFlat:           encoded.CurrentObject.AttrsFlat,
			Private:             encoded.CurrentObject.Private,
			CreateBeforeDestroy: encoded.CurrentObject.CreateBeforeDestroy,
		}
		switch encoded.CurrentObject.Status {
		case "ready":
			obj.Status = states.ObjectReady
		case "tainted":
			obj.Status = states.ObjectTainted
		default:
			return nil, addrs.AbsProviderConfig{}, fmt.Errorf("unsupported current object status %q", encoded.CurrentObject.Status)
		}
		if len(encoded.CurrentObject.Dependencies) != 0 {
			obj.Dependencies = make([]addrs.ConfigResource, 0, len(encoded.CurrentObject.Dependencies))
			for _, addrStr := range encoded.CurrentObject.Dependencies {
				instAddr, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
				if diags.HasErrors() {
					return nil, addrs.AbsProviderConfig{}, diags.Err()
				}
				addr := instAddr.ConfigResource()
				if addr.String() != addrStr {
					return nil, addrs.AbsProviderConfig{}, fmt.Errorf("extraneous indices in dependency address")
				}
				obj.Dependencies = append(obj.Dependencies, addr)
			}
		}
		ret.Current = obj
	}
	if len(encoded.DeposedObjects) != 0 {
		return nil, addrs.AbsProviderConfig{}, fmt.Errorf("this prototype does not handle deposed objects in state")
	}

	providerAddr, diags := addrs.ParseAbsProviderConfigStr(encoded.ProviderAddr)
	if diags.HasErrors() {
		return nil, addrs.AbsProviderConfig{}, diags.Err()
	}
	if encoded.ProviderInstanceKey != nil {
		return nil, addrs.AbsProviderConfig{}, fmt.Errorf("provider for_each not supported in this prototype")
	}

	return ret, providerAddr, nil
}

func decodeStateResource(_ statekeys.Resource, raw statestore.Value) (addrs.AbsProviderConfig, error) {
	type Encoded struct {
		ProviderConfigStr string `msgpack:"provider_config"`
	}
	var encoded Encoded
	err := msgpack.Unmarshal(raw, &encoded)
	if err != nil {
		return addrs.AbsProviderConfig{}, fmt.Errorf("invalid metadata encoding: %w", err)
	}

	providerAddr, diags := addrs.ParseAbsProviderConfigStr(encoded.ProviderConfigStr)
	return providerAddr, diags.Err()
}
