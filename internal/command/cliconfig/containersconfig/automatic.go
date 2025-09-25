// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package containersconfig

import (
	"context"
	"iter"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/containers"
)

func FindTypicalContainerRuntimes(ctx context.Context, env RuntimeDiscoveryEnvironment) iter.Seq2[ociv1.Platform, containers.Runtime] {
	// we'll delegate directly to OS-specific implementations in other
	// conditionally-compiled files.
	return findTypicalContainerRuntimes(ctx, env)
}

type RuntimeDiscoveryEnvironment interface {
	// FindCommandExe attempts to find the path to an executable that matches
	// the given command name. It returns an empty string if no such executable
	// file is available.
	//
	// [FindTypicalContainerRuntimes] uses this to try to automatically discover
	// various different OCI-compatible language runtimes by testing whether
	// their usual command names correspond to programs installed on the system.
	FindCommandExe(ctx context.Context, name string) string

	// ContainerIDBase returns a string to use as part of a container ID for
	// a container built from an image obtained from the given location and
	// with the given manifest.
	ContainerIDBase(registryName, repositoryName string, manifest *ociv1.Manifest) string
}
