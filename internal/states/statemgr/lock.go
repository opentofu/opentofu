// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statemgr

import (
	"context"

	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tofu"
)

// LockDisabled implements State and Locker but disables state locking.
// If State doesn't support locking, this is a no-op. This is useful for
// easily disabling locking of an existing state or for tests.
type LockDisabled struct {
	// We can't embed State directly since Go dislikes that a field is
	// State and State interface has a method State
	Inner Full
}

var _ Full = (*LockDisabled)(nil)

func (s *LockDisabled) State() *states.State {
	return s.Inner.State()
}

func (s *LockDisabled) GetRootOutputValues(ctx context.Context) (map[string]*states.OutputValue, error) {
	return s.Inner.GetRootOutputValues(ctx)
}

func (s *LockDisabled) WriteState(v *states.State) error {
	return s.Inner.WriteState(v)
}

func (s *LockDisabled) RefreshState(ctx context.Context) error {
	return s.Inner.RefreshState(ctx)
}

func (s *LockDisabled) PersistState(ctx context.Context, schemas *tofu.Schemas) error {
	return s.Inner.PersistState(ctx, schemas)
}

func (s *LockDisabled) Lock(_ context.Context, info *LockInfo) (string, error) {
	return "", nil
}

func (s *LockDisabled) Unlock(_ context.Context, id string) error {
	return nil
}
