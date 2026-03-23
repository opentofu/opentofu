// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package state_store

import (
	"context"
	"fmt"
	"maps"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func New(enc encryption.StateEncryption, library plugins.Library, providerAddr addrs.Provider, stateType string) (backend.Backend, tfdiags.Diagnostics) {
	manager := library.NewProviderManager()
	// For this simple function, context.Background is good enough
	providerSchema, diags := manager.GetProviderSchema(context.Background(), providerAddr)
	if diags.HasErrors() {
		return nil, diags
	}
	return &Backend{encryption: enc, manager: manager, providerAddr: providerAddr, providerSchema: providerSchema, stateType: stateType}, diags
}

type Backend struct {
	encryption     encryption.StateEncryption
	manager        plugins.ProviderManager
	providerAddr   addrs.Provider
	providerSchema providers.ProviderSchema
	stateType      string

	client *pluginClient
}

// ConfigSchema returns a description of the expected configuration
// structure for the receiving backend.
//
// This method does not have any side-effects for the backend and can
// be safely used before configuring.
func (b *Backend) ConfigSchema() *configschema.Block {
	base := b.providerSchema.StateStores[b.stateType].Block

	blockTypes := map[string]*configschema.NestedBlock{}
	maps.Copy(blockTypes, base.BlockTypes)
	blockTypes["provider"] = &configschema.NestedBlock{Block: *b.providerSchema.Provider.Block, Nesting: configschema.NestingMap}

	return &configschema.Block{
		Attributes:      base.Attributes,
		BlockTypes:      blockTypes,
		Description:     base.Description,
		DescriptionKind: base.DescriptionKind,
		Ephemeral:       base.Ephemeral,
	}
}

// PrepareConfig checks the validity of the values in the given
// configuration, and inserts any missing defaults, assuming that its
// structure has already been validated per the schema returned by
// ConfigSchema.
//
// This method does not have any side-effects for the backend and can
// be safely used before configuring. It also does not consult any
// external data such as environment variables, disk files, etc. Validation
// that requires such external data should be deferred until the
// Configure call.
//
// If error diagnostics are returned then the configuration is not valid
// and must not subsequently be passed to the Configure method.
//
// This method may return configuration-contextual diagnostics such
// as tfdiags.AttributeValue, and so the caller should provide the
// necessary context via the diags.InConfigBody method before returning
// diagnostics to the user.
func (b *Backend) PrepareConfig(cfgVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	// TODO make sure that there's only one provider block in the labeled map
	return cfgVal, nil
}

// Configure uses the provided configuration to set configuration fields
// within the backend.
//
// The given configuration is assumed to have already been validated
// against the schema returned by ConfigSchema and passed validation
// via PrepareConfig.
//
// This method may be called only once per backend instance, and must be
// called before all other methods except where otherwise stated.
//
// If error diagnostics are returned, the internal state of the instance
// is undefined and no other methods may be called.
func (b *Backend) Configure(ctx context.Context, cfgVal cty.Value) tfdiags.Diagnostics {
	cfgMap := cfgVal.AsValueMap()

	// TODO safe map access
	providerVal := cfgMap["provider"].AsValueMap()[b.providerAddr.Type]
	// Remove provider block
	delete(cfgMap, "provider")
	configVal := cty.ObjectVal(cfgMap)

	provider, diags := b.manager.NewConfiguredProvider(ctx, b.providerAddr, providerVal)
	if diags.HasErrors() {
		return diags
	}

	validateResp := provider.ValidateStateStoreConfig(ctx, providers.ValidateStateStoreConfigRequest{
		TypeName: b.stateType,
		Config:   configVal,
	})
	diags = diags.Append(validateResp.Diagnostics)
	if diags.HasErrors() {
		return diags
	}

	configureResp := provider.ConfigureStateStore(ctx, providers.ConfigureStateStoreRequest{
		TypeName: b.stateType,
		Config:   configVal,
		Capabilities: providers.StateStoreClientCapabilities{
			ChunkSize: 4 * 1024,
		},
	})
	diags = diags.Append(configureResp.Diagnostics)
	if diags.HasErrors() {
		return diags
	}

	b.client = &pluginClient{
		provider:  provider,
		stateType: b.stateType,
		chunkSize: configureResp.Capabilities.ChunkSize,
	}

	if b.client.chunkSize <= 0 {
		diags = diags.Append(fmt.Errorf("StateStorage Provider did not provide a valid value for ChunkSize: %v", b.client.chunkSize))
	}

	return diags
}

func (b *Backend) StateMgr(_ context.Context, name string) (statemgr.Full, error) {
	stateMgr := remote.NewState(b.client.Workspace(name), b.encryption)

	// Grab the value
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		return nil, err
	}
	//if this isn't the default state name, we need to create the object so
	//it's listed by States.
	if v := stateMgr.State(); v == nil {
		// take a lock on this state while we write it
		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockId, err := stateMgr.Lock(context.TODO(), lockInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to lock state_store: %w", err)
		}

		// Local helper function so we can call it multiple places
		lockUnlock := func(parent error) error {
			if err := stateMgr.Unlock(context.TODO(), lockId); err != nil {
				return err
			}
			return parent
		}

		if err := stateMgr.WriteState(states.NewState()); err != nil {
			err = lockUnlock(err)
			return nil, err
		}
		if err := stateMgr.PersistState(context.TODO(), nil); err != nil {
			err = lockUnlock(err)
			return nil, err
		}

		// Unlock, the state should now be initialized
		if err := lockUnlock(nil); err != nil {
			return nil, err
		}
	}

	return stateMgr, nil
}

func (b *Backend) Workspaces(context.Context) ([]string, error) {
	resp := b.client.provider.GetStates(context.TODO(), providers.GetStatesRequest{TypeName: b.client.stateType})
	resp.StateId = append(resp.StateId, backend.DefaultStateName)
	return resp.StateId, resp.Diagnostics.Err()
}

func (b *Backend) DeleteWorkspace(ctx context.Context, workspace string, _ bool) error {
	if workspace == backend.DefaultStateName || workspace == "" {
		return fmt.Errorf("can't delete default state")
	}
	return b.client.Workspace(workspace).Delete(ctx)
}
