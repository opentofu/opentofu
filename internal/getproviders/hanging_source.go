// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"

	"github.com/terramate-io/opentofulib/internal/addrs"
)

// HangingSource is an implementation of Source which hangs until the given
// context is cancelled. This is useful only for unit tests of user-controlled
// cancels.
type HangingSource struct {
}

var _ Source = (*HangingSource)(nil)

func (s *HangingSource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	<-ctx.Done()
	return nil, nil, nil
}

func (s *HangingSource) PackageMeta(ctx context.Context, provider addrs.Provider, version Version, target Platform) (PackageMeta, error) {
	<-ctx.Done()
	return PackageMeta{}, nil
}

func (s *HangingSource) ForDisplay(provider addrs.Provider) string {
	return "hanging source"
}
