// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package containers

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// ExecRuntime is an implementation of Runtime that performs all of its
// operations by executing other software available on the host system.
//
// For example, this implementation could be used to run "runc" on a Linux
// system, "runhcs" on Windows, or Apple's "container" tool on macOS. In all
// cases the actual interaction with the operating system's containers features,
// or with other layers such as a virtual machine manager, is delegated to the
// external program.
//
// This is designed for runtimes that can accept "bundles" as defined by the
// OCI runtime spec:
//
//	https://github.com/opencontainers/runtime-spec/blob/main/bundle.md
//
// It may be possible to use other runtimes through some sort of shim program
// that translates the OCI runtime bundle into another format, but that's
// left as an exercise for the caller.
type ExecRuntime struct {
	// ContainerIDBaseFunc is a function representing a rule for chosing a
	// container ID base name for a container based on the image
	// associated with the given repository and manifest.
	//
	// The result does not need to include any pseudorandom element because
	// [RunContainerFunc] is expected to generate a unique container ID itself,
	// using the provided base string as only part of it.
	ContainerIDBaseFunc func(registryName, repositoryName string, manifest *ociv1.Manifest) string

	// RunCommandFunc is a function representing a rule for generating a
	// command line for running a new container in the foreground.
	//
	// The "exePath" returned by the function should typically be a full filepath,
	// such as what would be returned by [exec.LookPath] when querying for
	// a program with a given name. The caller does not attempt to resolve
	// a naked command name itself.
	//
	// The returned command will be run with the current working directory set
	// to the given bundle directory, and so it's okay to return a command line
	// that doesn't include bundleDir if the returned program expects to find
	// a bundle in its current working directory.
	//
	// The returned command must cause the creation of a new container with
	// the given ID and bundle directory, then run that container's entry point
	// program and stay running until it terminates. The entry point program's
	// stdin, stdout, and stderr handles MUST match those used to launch the
	// program. The runtime must not introduce any of its own data to either
	// the stdin or stdout streams, although it is allowed to produce error
	// messages and other diagnostic content on stderr when failing with an
	// error code. Ideally there should not be a pseudoterminal or other
	// similar abstraction on the path to the entrypoint program -- direct
	// pipes are preferred -- but this is not a hard requirement.
	//
	// The program must respond to being sent the host platform's typical
	// "termination" signals (e.g. SIGTERM on Unix) by passing a similar signal
	// to the software running in the container, waiting for the contained
	// process to terminate, and then deleting the container before returning.
	//
	// If the returned command needs to be run as a specific user or with
	// specific privileges that the current process might not have, the returned
	// command line must include some way to obtain those privileges. Ideally
	// though, the returned command should start the container with the same
	// privileges as it was launched with whenever that is possible.
	RunCommandFunc func(containerID string, bundleDir string) (exePath string, args []string)
}

var _ Runtime = (*ExecRuntime)(nil)

// ContainerIDBase implements Runtime.
func (e *ExecRuntime) ContainerIDBase(registryName string, repositoryName string, manifest *ociv1.Manifest) string {
	return e.ContainerIDBaseFunc(registryName, repositoryName, manifest)
}

// RunContainer implements Runtime.
func (e *ExecRuntime) RunContainer(ctx context.Context, containerIDBase string, bundleDir string) (ActiveContainer, error) {
	// This implementation cannot actually guarantee to select a unique
	// container ID, so we just generate an ID using a fixed naming pattern
	// and hope for the best, letting the command fail with an error if
	// what we generated wasn't actually unique.
	pid := os.Getpid()
	rnd := rand.Uint64()
	containerID := fmt.Sprintf("tofu-%d-%s-%d", pid, containerIDBase, rnd)

	exePath, exeArgs := e.RunCommandFunc(containerID, bundleDir)
	fullArgs := make([]string, len(exeArgs)+1)
	fullArgs[0] = exePath
	copy(fullArgs[1:], exeArgs)
	cmd := &exec.Cmd{
		Path: exePath,
		Args: fullArgs,
		Dir:  bundleDir,
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("can't create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("can't create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("can't create stderr pipe: %w", err)
	}
	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to launch external container runtime: %w", err)
	}

	ret := &execRuntimeContainer{
		containerID: containerID,
		streams: ActiveContainerStreams{
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
		},
		inner: &execRuntimeContainerInner{
			cmd: cmd,
		},
	}
	// We'll also add a cleanup to the pointer to make a best effort to close
	// it when it's garbage collected. A correct caller should not rely on this,
	// but this gives us more chance of cleaning things up properly if
	// the program exits early due to an error, etc.
	runtime.AddCleanup(ret, func(inner *execRuntimeContainerInner) {
		if inner.closed.Load() {
			// Don't try cleanup if someone already explicitly called Close
			return
		}
		// We can't handle errors in automatic cleanup; this is just a
		// best-effort thing because a correct caller should always explicitly
		// call Close, which would then allow it to detect and report errors here.
		_ = inner.cleanup()
	}, ret.inner)

	return ret, nil
}

// execRuntimeContainer is the implementation of [ActiveContainer] returned
// by [ExecRuntime.RunContainer].
type execRuntimeContainer struct {
	containerID string
	streams     ActiveContainerStreams
	inner       *execRuntimeContainerInner
}

// Close implements ActiveContainer.
func (e *execRuntimeContainer) Close(ctx context.Context) error {
	if !e.inner.closed.CompareAndSwap(false, true) {
		return fmt.Errorf("already closed")
	}
	return e.inner.cleanup()
}

// ContainerID implements ActiveContainer.
func (e *execRuntimeContainer) ContainerID() string {
	return e.containerID
}

// Streams implements ActiveContainer.
func (e *execRuntimeContainer) Streams() ActiveContainerStreams {
	return e.streams
}

// execRuntimeContainerInner is the part of [execRuntimeContainer] used by
// our cleanup function, separated so that it can survive even when its
// parent object has already been garbage collected.
type execRuntimeContainerInner struct {
	closed atomic.Bool
	cmd    *exec.Cmd
}

func (i *execRuntimeContainerInner) cleanup() error {
	if err := i.cmd.Process.Kill(); err != nil {
		return err
	}
	return i.cmd.Wait()
}
