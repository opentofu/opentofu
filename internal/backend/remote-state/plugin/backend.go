// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/zclconf/go-cty/cty"
)

var ProviderSupplier func(addrs.LocalProviderConfig) (providers.Interface, error)

func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"provider": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"type": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"config": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
			},
		},
	}

	b := &Backend{Backend: s, encryption: enc}
	b.Backend.ConfigureFunc = b.configure
	return b
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	client *pluginClient
}

func (b *Backend) configure(ctx context.Context) error {
	data := schema.FromContextBackendConfig(ctx)

	providerIdent := data.Get("provider").(string)
	providerParts := strings.Split(providerIdent, ".")
	addr := addrs.LocalProviderConfig{
		LocalName: providerParts[0],
	}
	if len(providerParts) > 1 {
		addr.Alias = providerParts[1]
	}

	provider, err := ProviderSupplier(addr)
	if err != nil {
		return err
	}

	cfgType := data.Get("type").(string)
	cfgVal := cty.NullVal(cty.DynamicPseudoType)
	rawConfig, ok := data.GetOk("config")
	if ok {
		mapConfig := rawConfig.(map[string]interface{})
		// TODO fancier schema mapping
		ident := mapConfig["ident"].(string)
		cfgVal = cty.MapVal(map[string]cty.Value{
			"ident": cty.StringVal(ident),
		})
	}

	validateResp := provider.ValidateStateStoreConfig(ctx, providers.ValidateStateStoreConfigRequest{
		TypeName: cfgType,
		Config:   cfgVal,
	})

	if validateResp.Diagnostics.HasErrors() {
		return validateResp.Diagnostics.Err()
	}

	configureResp := provider.ConfigureStateStore(ctx, providers.ConfigureStateStoreRequest{
		TypeName: cfgType,
		Config:   cfgVal,
		Capabilities: providers.StateStoreClientCapabilities{
			ChunkSize: 4 * 1024,
		},
	})

	if configureResp.Diagnostics.HasErrors() {
		return configureResp.Diagnostics.Err()
	}

	b.client = &pluginClient{
		provider:  provider,
		cfgType:   cfgType,
		chunkSize: configureResp.Capabilities.ChunkSize,
	}

	if b.client.chunkSize <= 0 {
		return fmt.Errorf("StateStorage Provider did not provide a valid value for ChunkSize: %v", b.client.chunkSize)
	}

	return nil
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
			return nil, fmt.Errorf("failed to lock plugin state: %w", err)
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
	resp := b.client.provider.GetStates(context.TODO(), providers.GetStatesRequest{TypeName: b.client.cfgType})
	resp.StateId = append(resp.StateId, backend.DefaultStateName)
	return resp.StateId, resp.Diagnostics.Err()
}

func (b *Backend) DeleteWorkspace(ctx context.Context, workspace string, _ bool) error {
	if workspace == backend.DefaultStateName || workspace == "" {
		return fmt.Errorf("can't delete default state")
	}
	return b.client.Workspace(workspace).Delete(ctx)
}
