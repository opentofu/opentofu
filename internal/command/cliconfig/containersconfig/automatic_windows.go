// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package containersconfig

import (
	"context"
	"iter"
	"runtime"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/containers"
)

func findTypicalContainerRuntimes(ctx context.Context, env RuntimeDiscoveryEnvironment) iter.Seq2[ociv1.Platform, containers.Runtime] {
	// Windows has support for both Windows-native containers and for
	// Linux-based containers run in a Hyper-V based virtual machine.
	//
	// We'll therefore report runtimes for both of these.
	// This means that when we find an index manifest that has both a
	// Windows and a Linux descriptor we'll prefer the Windows one,
	// but we can still use Linux-only images when that's all we have.
	windowsPlatform := ociv1.Platform{
		OS:           "windows",
		Architecture: runtime.GOARCH,

		// OpenTofu only suppports hosts that have the "win32k" feature,
		// which is the Windows API subsystem, so we'll include that so we
		// can match descriptors which require it.
		// (Only very cut-down environments, such as Windows Nano Server
		// used as a base image for Windows-native containers, leave out
		// this subsystem. But this feature is talking about the capabilities
		// of the host, rather than what's installed in the container, so
		// that would be relevant only if OpenTofu were run inside such a
		// container, which we don't support.)
		OSFeatures: []string{"win32k"},
	}
	linuxPlatform := ociv1.Platform{
		OS:           "linux",
		Architecture: runtime.GOARCH,
	}

	return func(yield func(ociv1.Platform, containers.Runtime) bool) {
		if exePath := env.FindCommandExe(ctx, "runhcs"); exePath != "" {
			// "runhcs" was forked from "runc", so its command line usage
			// patterns are similar.
			runtime := makeExecRuntime(env, exePath, func(containerID, bundleDir string) []string {
				// no bundleDir included because runc uses the current working directory
				return []string{"create", containerID}
			})
			if !yield(windowsPlatform, runtime) {
				return
			}
			if !yield(linuxPlatform, runtime) {
				return
			}
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
