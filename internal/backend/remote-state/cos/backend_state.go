// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cos

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// Define file suffix
const (
	stateFileSuffix = ".tfstate"
	lockFileSuffix  = ".tflock"
)

// Workspaces returns a list of names for the workspaces
func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	c, err := b.client("tencentcloud")
	if err != nil {
		return nil, err
	}

	obs, err := c.getBucket(ctx, b.prefix)
	log.Printf("[DEBUG] list all workspaces, objects: %v, error: %v", obs, err)
	if err != nil {
		return nil, err
	}

	ws := []string{backend.DefaultStateName}
	for _, vv := range obs {
		// <name>.tfstate
		if !strings.HasSuffix(vv.Key, stateFileSuffix) {
			continue
		}
		// default workspace
		if path.Join(b.prefix, b.key) == vv.Key {
			continue
		}
		// <prefix>/<workspace>/<key>
		prefix := strings.TrimRight(b.prefix, "/") + "/"
		parts := strings.Split(strings.TrimPrefix(vv.Key, prefix), "/")
		if len(parts) > 0 && parts[0] != "" {
			ws = append(ws, parts[0])
		}
	}

	sort.Strings(ws[1:])
	log.Printf("[DEBUG] list all workspaces, workspaces: %v", ws)

	return ws, nil
}

// DeleteWorkspace deletes the named workspaces. The "default" state cannot be deleted.
func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	log.Printf("[DEBUG] delete workspace, workspace: %v", name)

	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("default state is not allow to delete")
	}

	c, err := b.client(name)
	if err != nil {
		return err
	}

	return c.Delete(ctx)
}

// StateMgr manage the state, if the named state not exists, a new file will created
func (b *Backend) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	log.Printf("[DEBUG] state manager, current workspace: %v", name)

	c, err := b.client(name)
	if err != nil {
		return nil, err
	}
	stateMgr := remote.NewState(c, b.encryption)

	ws, err := b.Workspaces(ctx)
	if err != nil {
		return nil, err
	}

	exists := false
	for _, candidate := range ws {
		if candidate == name {
			exists = true
			break
		}
	}

	if !exists {
		log.Printf("[DEBUG] workspace %v not exists", name)

		// take a lock on this state while we write it
		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockId, err := c.Lock(context.TODO(), lockInfo)
		if err != nil {
			return nil, fmt.Errorf("Failed to lock cos state: %w", err)
		}

		// Local helper function so we can call it multiple places
		lockUnlock := func(e error) error {
			if err := stateMgr.Unlock(context.TODO(), lockId); err != nil {
				return fmt.Errorf(unlockErrMsg, err, lockId)
			}
			return e
		}

		// Grab the value
		if err := stateMgr.RefreshState(context.TODO()); err != nil {
			err = lockUnlock(err)
			return nil, err
		}

		// If we have no state, we have to create an empty state
		if v := stateMgr.State(); v == nil {
			if err := stateMgr.WriteState(states.NewState()); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
			if err := stateMgr.PersistState(context.TODO(), nil); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
		}

		// Unlock, the state should now be initialized
		if err := lockUnlock(nil); err != nil {
			return nil, err
		}
	}

	return stateMgr, nil
}

// client returns a remoteClient for the named state.
func (b *Backend) client(name string) (*remoteClient, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("state name not allow to be empty")
	}

	return &remoteClient{
		cosClient: b.cosClient,
		tagClient: b.tagClient,
		bucket:    b.bucket,
		stateFile: b.stateFile(name),
		lockFile:  b.lockFile(name),
		encrypt:   b.encrypt,
		acl:       b.acl,
	}, nil
}

// stateFile returns state file path by name
func (b *Backend) stateFile(name string) string {
	if name == backend.DefaultStateName {
		return path.Join(b.prefix, b.key)
	}
	return path.Join(b.prefix, name, b.key)
}

// lockFile returns lock file path by name
func (b *Backend) lockFile(name string) string {
	return b.stateFile(name) + lockFileSuffix
}

// unlockErrMsg is error msg for unlock failed
const unlockErrMsg = `
Unlocking the state file on TencentCloud cos backend failed:

Error message: %v
Lock ID (gen): %s

You may have to force-unlock this state in order to use it again.
The TencentCloud backend acquires a lock during initialization
to ensure the initial state file is created.
`
