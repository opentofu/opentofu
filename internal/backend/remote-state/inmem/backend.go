// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package inmem

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	statespkg "github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// we keep the states and locks in package-level variables, so that they can be
// accessed from multiple instances of the backend. This better emulates
// backend instances accessing a single remote data store.
var (
	states stateMap
	locks  lockMap
)

func init() {
	Reset()
}

// Reset clears out all existing state and lock data.
// This is used to initialize the package during init, as well as between
// tests.
func Reset() {
	states = stateMap{
		m: map[string]*remote.State{},
	}

	locks = lockMap{
		m: map[string]*statemgr.LockInfo{},
	}
}

// New creates a new backend for Inmem remote state.
func New(enc encryption.StateEncryption) backend.Backend {
	// Set the schema
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"lock_id": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Description: "initializes the state in a locked configuration",
			},
		},
	}
	backend := &Backend{Backend: s, encryption: enc}
	backend.Backend.ConfigureFunc = backend.configure
	return backend
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption
}

func (b *Backend) configure(ctx context.Context) error {
	states.Lock()
	defer states.Unlock()

	defaultClient := &RemoteClient{
		Name: backend.DefaultStateName,
	}

	states.m[backend.DefaultStateName] = remote.NewState(defaultClient, b.encryption)

	// set the default client lock info per the test config
	data := schema.FromContextBackendConfig(ctx)
	_, hasDefaultLock := locks.m[backend.DefaultStateName]
	if v, ok := data.GetOk("lock_id"); ok && v.(string) != "" && !hasDefaultLock {
		info := statemgr.NewLockInfo()
		info.ID = v.(string)
		info.Operation = "test"
		info.Info = "test config"

		if _, err := locks.lock(backend.DefaultStateName, info); err != nil {
			return err
		}
	}

	return nil
}

func (b *Backend) Workspaces(context.Context) ([]string, error) {
	states.Lock()
	defer states.Unlock()

	var workspaces []string

	for s := range states.m {
		workspaces = append(workspaces, s)
	}

	sort.Strings(workspaces)
	return workspaces, nil
}

func (b *Backend) DeleteWorkspace(_ context.Context, name string, _ bool) error {
	states.Lock()
	defer states.Unlock()

	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	delete(states.m, name)
	return nil
}

func (b *Backend) StateMgr(_ context.Context, name string) (statemgr.Full, error) {
	states.Lock()
	defer states.Unlock()

	s := states.m[name]
	if s == nil {
		s = remote.NewState(
			&RemoteClient{
				Name: name,
			},
			b.encryption,
		)
		states.m[name] = s

		// to most closely replicate other implementations, we are going to
		// take a lock and create a new state if it doesn't exist.
		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockID, err := s.Lock(context.TODO(), lockInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to lock inmem state: %w", err)
		}

		// Local helper function so we can call it multiple places
		lockUnlock := func(parent error) error {
			if err := s.Unlock(context.TODO(), lockID); err != nil {
				return errors.Join(
					fmt.Errorf("error unlocking inmem state: %w", err),
					parent,
				)
			}
			return parent
		}

		// If we have no state, we have to create an empty state
		if v := s.State(); v == nil {
			if err := s.WriteState(statespkg.NewState()); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
			if err := s.PersistState(context.TODO(), nil); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
		}

		// Unlock, the state should now be initialized
		if err := lockUnlock(nil); err != nil {
			return nil, err
		}
	}

	return s, nil
}

type stateMap struct {
	sync.Mutex
	m map[string]*remote.State
}

// Global level locks for inmem backends.
type lockMap struct {
	sync.Mutex
	m map[string]*statemgr.LockInfo
}

func (l *lockMap) lock(name string, info *statemgr.LockInfo) (string, error) {
	l.Lock()
	defer l.Unlock()

	lockInfo := l.m[name]
	if lockInfo != nil {
		lockErr := &statemgr.LockError{
			Info: lockInfo,
		}

		lockErr.Err = errors.New("state locked")
		// make a copy of the lock info to avoid any testing shenanigans
		*lockErr.Info = *lockInfo
		return "", lockErr
	}

	info.Created = time.Now().UTC()
	l.m[name] = info

	return info.ID, nil
}

func (l *lockMap) unlock(name, id string) error {
	l.Lock()
	defer l.Unlock()

	lockInfo := l.m[name]

	if lockInfo == nil {
		return errors.New("state not locked")
	}

	lockErr := &statemgr.LockError{
		Info: &statemgr.LockInfo{},
	}

	if id != lockInfo.ID {
		lockErr.Err = errors.New("invalid lock id")
		*lockErr.Info = *lockInfo
		return lockErr
	}

	delete(l.m, name)
	return nil
}
