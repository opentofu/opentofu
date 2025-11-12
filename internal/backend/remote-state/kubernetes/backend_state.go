// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Workspaces returns a list of names for the workspaces found in k8s. The default
// workspace is always returned as the first element in the slice.
func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	secretClient, err := b.getKubernetesSecretClient()
	if err != nil {
		return nil, err
	}

	secrets, err := secretClient.List(
		ctx,
		metav1.ListOptions{
			LabelSelector: tfstateKey + "=true",
		},
	)
	if err != nil {
		return nil, err
	}

	// Use a map so there aren't duplicate workspaces
	m := make(map[string]struct{})
	for _, secret := range secrets.Items {
		sl := secret.GetLabels()
		ws, ok := sl[tfstateWorkspaceKey]
		if !ok {
			continue
		}

		key, ok := sl[tfstateSecretSuffixKey]
		if !ok {
			continue
		}

		// Make sure it isn't default and the key matches
		if ws != backend.DefaultStateName && key == b.nameSuffix {
			m[ws] = struct{}{}
		}
	}

	states := []string{backend.DefaultStateName}
	for k := range m {
		states = append(states, k)
	}

	sort.Strings(states[1:])
	return states, nil
}

func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	client, err := b.remoteClient(name)
	if err != nil {
		return err
	}

	return client.Delete(ctx)
}

func (b *Backend) StateMgr(_ context.Context, name string) (statemgr.Full, error) {
	c, err := b.remoteClient(name)
	if err != nil {
		return nil, err
	}

	stateMgr := remote.NewState(c, b.encryption)

	// Grab the value
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		return nil, err
	}

	// If we have no state, we have to create an empty state
	if v := stateMgr.State(); v == nil {

		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockID, err := stateMgr.Lock(context.TODO(), lockInfo)
		if err != nil {
			return nil, err
		}

		secretName, err := c.createSecretName()
		if err != nil {
			return nil, err
		}

		// Local helper function so we can call it multiple places
		unlock := func(baseErr error) error {
			if err := stateMgr.Unlock(context.TODO(), lockID); err != nil {
				const unlockErrMsg = `%v
				Additionally, unlocking the state in Kubernetes failed:

				Error message: %w
				Lock ID (gen): %v
				Secret Name: %v

				You may have to force-unlock this state in order to use it again.
				The Kubernetes backend acquires a lock during initialization to ensure
				the initial state file is created.`
				return fmt.Errorf(unlockErrMsg, baseErr, err, lockID, secretName)
			}

			return baseErr
		}

		if err := stateMgr.WriteState(states.NewState()); err != nil {
			return nil, unlock(err)
		}
		if err := stateMgr.PersistState(context.TODO(), nil); err != nil {
			return nil, unlock(err)
		}

		// Unlock, the state should now be initialized
		if err := unlock(nil); err != nil {
			return nil, err
		}

	}

	return stateMgr, nil
}

// get a remote client configured for this state
func (b *Backend) remoteClient(name string) (*RemoteClient, error) {
	if name == "" {
		return nil, errors.New("missing state name")
	}

	secretClient, err := b.getKubernetesSecretClient()
	if err != nil {
		return nil, err
	}

	leaseClient, err := b.getKubernetesLeaseClient()
	if err != nil {
		return nil, err
	}

	client := &RemoteClient{
		kubernetesSecretClient: secretClient,
		kubernetesLeaseClient:  leaseClient,
		namespace:              b.namespace,
		labels:                 b.labels,
		nameSuffix:             b.nameSuffix,
		workspace:              name,
	}

	return client, nil
}
