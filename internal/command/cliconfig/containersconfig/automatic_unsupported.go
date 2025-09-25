// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !linux && !windows

package containersconfig

import (
	"context"
	"iter"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/containers"
)

func findTypicalContainerRuntimes(_ context.Context, _ RuntimeDiscoveryEnvironment) iter.Seq2[ociv1.Platform, containers.Runtime] {
	// On unsupported platforms we just return nothing at all. Container
	// runtimes must be explicitly configured.
	return func(yield func(ociv1.Platform, containers.Runtime) bool) {}
}
