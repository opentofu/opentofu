package vault

import (
	"fmt"
	"strings"

	"github.com/hashicorp/vault-client-go"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func (b *Backend) Workspaces() ([]string, error) {
	// List our raw path
	prefix := b.configData.Get("path").(string)
	respData, err := GetMetadata(b.ctx, b.client,
		b.configData.Get("mount").(string), prefix, true)
	if err != nil {
		return nil, err
	}

	// 	"data": {
	// 	  "keys": [
	// 		"workspace1",
	// 		"workspace2"
	// 	  ]
	// 	},

	result := []string{backend.DefaultStateName}

	if keys, ok := respData["keys"]; ok {
		for _, key := range keys.([]any) {
			result = append(result, key.(string))
		}
	}

	return result, nil
}

func (b *Backend) DeleteWorkspace(name string, _ bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	// Determine the path of the data
	path := b.path(name)

	// Delete it. We just delete it without any locking since
	// the DeleteState API is documented as such.
	_, err := b.client.Secrets.KvV2DeleteMetadataAndAllVersions(
		b.ctx,
		path, vault.WithMountPath(b.configData.Get("mount").(string)),
	)
	return err
}

func (b *Backend) StateMgr(name string) (statemgr.Full, error) {
	// Build the state client
	var stateMgr = &remote.State{
		Client: &RemoteClient{
			Client:    b.client,
			Name:      name,
			ctx:       b.ctx,
			Path:      b.path(name), // Determine the path of the data
			GZip:      b.configData.Get("gzip").(bool),
			mountPath: b.configData.Get("mount").(string),
		},
	}
	if !b.lock {
		stateMgr.DisableLocks()
	}

	// the default state always exists
	if name == backend.DefaultStateName {
		return stateMgr, nil
	}

	// Grab a lock, we use this to write an empty state if one doesn't
	// exist already. We have to write an empty state as a sentinel value
	// so States() knows it exists.
	lockInfo := statemgr.NewLockInfo()
	lockInfo.Operation = "init"
	lockId, err := stateMgr.Lock(lockInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to lock state in Vault: %s", err)
	}

	// Local helper function so we can call it multiple places
	lockUnlock := func(parent error) error {
		if err := stateMgr.Unlock(lockId); err != nil {
			return fmt.Errorf(strings.TrimSpace(errStateUnlock), lockId, err)
		}

		return parent
	}

	// Grab the value
	if err := stateMgr.RefreshState(); err != nil {
		err = lockUnlock(err)
		return nil, err
	}

	// If we have no state, we have to create an empty state
	if v := stateMgr.State(); v == nil {
		if err := stateMgr.WriteState(states.NewState()); err != nil {
			err = lockUnlock(err)
			return nil, err
		}
		if err := stateMgr.PersistState(nil); err != nil {
			err = lockUnlock(err)
			return nil, err
		}
	}

	// Unlock, the state should now be initialized
	if err := lockUnlock(nil); err != nil {
		return nil, err
	}

	return stateMgr, nil
}

func (b *Backend) path(name string) string {
	path := b.configData.Get("path").(string)
	if name != backend.DefaultStateName {
		path += fmt.Sprintf("/%s", name)
	}

	return path
}

const errStateUnlock = `
Error unlocking the Vault secret state. Lock ID: %s

Error: %s

You may have to force-unlock this state in order to use it again.
The Vaylt backend acquires a lock during initialization to ensure
the initial state file is created.
`
