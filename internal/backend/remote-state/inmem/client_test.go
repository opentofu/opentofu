// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package inmem

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
)

func TestRemoteClient_impl(t *testing.T) {
	var _ remote.Client = new(RemoteClient)
	var _ remote.ClientLocker = new(RemoteClient)
}

func TestRemoteClient(t *testing.T) {
	defer Reset()
	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), hcl.EmptyBody())

	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, s.(*remote.State).Client)
}

func TestInmemLocks(t *testing.T) {
	defer Reset()
	s, err := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), hcl.EmptyBody()).StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestRemoteLocks(t, s.(*remote.State).Client, s.(*remote.State).Client)
}
