// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package containersconfig

import (
	"context"
	"iter"
	"runtime"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/containers"
)

func findTypicalContainerRuntimes(ctx context.Context, env RuntimeDiscoveryEnvironment) iter.Seq2[ociv1.Platform, containers.Runtime] {
	platform := ociv1.Platform{
		OS:           "linux",
		Architecture: runtime.GOARCH,
	}

	return func(yield func(ociv1.Platform, containers.Runtime) bool) {
		if exePath := env.FindCommandExe(ctx, "runc"); exePath != "" {
			yield(platform, makeExecRuntime(env, exePath, func(containerID, bundleDir string) []string {
				// no bundleDir included because runc uses the current working directory
				return []string{"create", containerID}
			}))
			return
		}
		if exePath := env.FindCommandExe(ctx, "systemd-nspawn"); exePath != "" {
			yield(platform, makeExecRuntime(env, exePath, func(containerID, bundleDir string) []string {
				return []string{"--machine=" + containerID, "--oci-layout=" + bundleDir, "--no-pager", "--no-ask-password"}
			}))
			return
		}
	}
}

func makeExecRuntime(env RuntimeDiscoveryEnvironment, exePath string, argsFunc func(containerID, bundleDir string) []string) containers.Runtime {
	return &containers.ExecRuntime{
		ContainerIDBaseFunc: env.ContainerIDBase,
		RunCommandFunc: func(containerID, bundleDir string) (string, []string) {
			args := argsFunc(containerID, bundleDir)
			return exePath, args
		},
	}
}
