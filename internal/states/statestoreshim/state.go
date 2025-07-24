// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestoreshim

import (
	"context"
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
func LoadPriorState(ctx context.Context, storage statestore.Storage, haveLocks statestore.KeySet) (*states.State, statestore.ValueHashes, error) {
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
			continue
		}
		key, keyErr := statekeys.KeyFromStorage(storageKey)
		if keyErr != nil {
			err = errors.Join(err, fmt.Errorf("unsupported state content: %w", keyErr))
			continue
		}
		switch key := key.(type) {
		case statekeys.Resource:
			_, decErr := decodeStateResource(key, rawValue)
			if decErr != nil {
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
			if decErr != nil {
				err = errors.Join(err, fmt.Errorf("invalid state entry for %s: %w", key.Address(), decErr))
				continue
			}
			ss.SetResourceProvider(key.Address().ContainingResource(), providerAddr)
			ss.SetResourceInstanceCurrent(key.Address(), ri.Current, providerAddr, ri.ProviderKey)
		case statekeys.RootModuleOutputValue:
			ov, decErr := decodeStateRootOutputValue(key, rawValue)
			if decErr != nil {
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

	hashes := make(statestore.ValueHashes, len(rawEntries))
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

type outputValueEncoded struct {
	EncodedValue []byte `msgpack:"value"`
	Sensitive    bool   `msgpack:"sensitive"`
	Deprecated   string `msgpack:"deprecated"`
}

func decodeStateRootOutputValue(key statekeys.RootModuleOutputValue, raw statestore.Value) (*states.OutputValue, error) {
	var encoded outputValueEncoded
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

func encodeStateRootOutputValue(_ statekeys.RootModuleOutputValue, state *states.OutputValue) (statestore.Value, error) {
	if state == nil || state.Value == cty.NilVal || state.Value.IsNull() {
		// We represent the absense of the output value by writing
		// no raw value at all, which the storage layer can choose
		// to implement either by deleting the entry entirely or
		// by storing an explicit "not present" marker, at its option.
		// FIXME: Probably would be better to leave the metadata in place
		// with a null cty.Value stored if the output value is still
		// declared in the configuration, so we can retain the metadata
		// about it being deprecated/sensitive, but we won't worry about
		// that for this initial prototype.
		return statestore.NoValue, nil
	}
	valSrc, err := ctymsgpack.Marshal(state.Value, cty.DynamicPseudoType)
	if err != nil {
		return statestore.NoValue, fmt.Errorf("encoding value: %w", err)
	}

	encoded := outputValueEncoded{
		EncodedValue: valSrc,
		Sensitive:    state.Sensitive,
		Deprecated:   state.Deprecated,
	}
	raw, err := msgpack.Marshal(&encoded)
	if err != nil {
		return statestore.NoValue, fmt.Errorf("encoding metadata: %w", err)
	}
	return statestore.Value(raw), nil
}

type resourceInstanceEncoded struct {
	CurrentObject  *resourceInstanceObjectEncoded           `msgpack:"current"`
	DeposedObjects map[string]resourceInstanceObjectEncoded `msgpack:"deposed"`

	// Tech debt from the provider for_each project: we still don't actually
	// have a proper address type for a fully-qualified provider instance. :(
	ProviderAddr        string `msgpack:"provider"`
	ProviderInstanceKey any    `msgpack:"provider_instance_key"`
}

type resourceInstanceObjectEncoded struct {
	SchemaVersion       uint64            `msgpack:"schema_version"`
	AttrsJSON           []byte            `msgpack:"attributes"`
	AttrsFlat           map[string]string `msgpack:"legacy_attributes"`
	Dependencies        []string          `msgpack:"dependencies"`
	Status              string            `msgpack:"status"`
	CreateBeforeDestroy bool              `msgpack:"create_before_destroy"`
	Private             []byte            `msgpack:"private"`
	// TODO: Sensitive attribute paths
}

func decodeStateResourceInstance(_ statekeys.ResourceInstance, raw statestore.Value) (*states.ResourceInstance, addrs.AbsProviderConfig, error) {
	var encoded resourceInstanceEncoded
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

func encodeStateResourceInstance(_ statekeys.ResourceInstance, resourceState *states.Resource, instanceKey addrs.InstanceKey) (statestore.Value, error) {
	if resourceState == nil {
		return statestore.NoValue, nil
	}
	instState, ok := resourceState.Instances[instanceKey]
	if !ok {
		return statestore.NoValue, nil
	}
	if instState.ProviderKey != nil {
		return statestore.NoValue, fmt.Errorf("provider for_each is not supported in this prototype")
	}
	if len(instState.Deposed) != 0 {
		return statestore.NoValue, fmt.Errorf("deposed objects are not supported in this prototype")
	}

	encoded := resourceInstanceEncoded{
		ProviderAddr: resourceState.ProviderConfig.String(),
	}
	if instState.Current != nil {
		encoded.CurrentObject = &resourceInstanceObjectEncoded{
			SchemaVersion:       instState.Current.SchemaVersion,
			AttrsJSON:           instState.Current.AttrsJSON,
			AttrsFlat:           instState.Current.AttrsFlat,
			CreateBeforeDestroy: instState.Current.CreateBeforeDestroy,
			Private:             instState.Current.Private,
		}
		switch instState.Current.Status {
		case states.ObjectReady:
			encoded.CurrentObject.Status = "ready"
		case states.ObjectTainted:
			encoded.CurrentObject.Status = "tainted"
		default:
			return statestore.NoValue, fmt.Errorf("unexpected current object status %s", instState.Current.Status)
		}
		if deps := instState.Current.Dependencies; len(deps) != 0 {
			encoded.CurrentObject.Dependencies = make([]string, len(deps))
			for i, addr := range deps {
				encoded.CurrentObject.Dependencies[i] = addr.String()
			}
		}
	}

	raw, err := msgpack.Marshal(&encoded)
	if err != nil {
		return statestore.NoValue, err
	}
	return statestore.Value(raw), nil
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
