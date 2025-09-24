// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package containers

import (
	"context"
	"io"
)

type ActiveContainer interface {
	// ContainerID returns the unique ID of the container within the runtime
	// it is managed by.
	//
	// This is only for use in error messages and debug information. OpenTofu
	// itself must not make any assumptions about it, and in particular does
	// not assume it'll be unique across all containers running for a particular
	// OpenTofu process.
	ContainerID() string

	// Streams returns an object providing access to the stdin, stdout and
	// stderr handles of the entrypoint program of the container.
	Streams() ActiveContainerStreams

	// Close stops execution of the software in the container and removes
	// it from the runtime.
	//
	// This function incorporates the effects of both the "Kill" and "Delete"
	// operations in the OCI Runtime specification, because OpenTofu has no
	// need to stop a container without immediately deleting it:
	//     https://github.com/opencontainers/runtime-spec/blob/main/runtime.md#operations
	//
	// The underlying OCI "kill" operation expects to be provided a signal to
	// send to the process. Supported signals vary by platform and container
	// runtime, but a correct implementation should choose whatever signal most
	// closely represents "terminate", such as SIGTERM on a typical Unix system.
	//
	// Returning an error indicates that the container was probably not
	// completely deleted, and so the operator may need to perform some manual
	// cleanup. In that case, the error message should include enough
	// information for the reader to understand what remnants have been left
	// behind.
	//
	// OpenTofu considers containers to be transient objects that are no longer
	// needed once closed, so implementations should also clean up any other
	// resources that are owned exclusively by the associated container to
	// avoid the need for an administrator to do explicit cleanup after
	// OpenTofu has finished using a container.
	Close(ctx context.Context) error
}

// ActiveContainerStreams describes how to read and write an active container's
// "stdio" handles.
//
// These are typically associated with pipes whose opposing end is associated
// with the corresponding file descriptor in the entry point program of the
// container, but callers must not assume they are any specific implementation
// of the specified interfaces.
type ActiveContainerStreams struct {
	// Stdin is a writer to sending bytes to the container's entrypoint program
	// via its "standard input" file descriptor.
	Stdin io.WriteCloser

	// Stdout is a reader for reading bytes sent by the container's entrypoint
	// program to its "standard output" file descriptor.
	Stdout io.ReadCloser

	// Stderr is a reader for reading bytes sent by the container's entrypoint
	// program to its "standard error" file descriptor.
	Stderr io.ReadCloser
}
