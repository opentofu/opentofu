// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package containers

import (
	"context"
	"iter"
	"slices"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Runtimes is a collection of container runtimes, identified by which operating
// system and architecture each one is capable of running containers for.
//
// Unlike when running native executables directly, the relationship between
// host platform and supported container platforms is loose: some container
// runtimes can actually run containers intended for another operating system
// inside a virtual machine running that operating system, and possibly even
// containers for a different CPU architecture inside an emulator.
type Runtimes struct {
	runtimes []runtimePlatform
}

// NewRuntimes constructs a new [Runtimes] object containing the given
// runtimes.
//
// When asked for a runtime suitable for a given platform the result will
// be the first element in the sequence that matches the request. Preferred
// runtimes, such as those which are native to the host OS/architecture rather
// that relying on virtual machines or emulation, should be listed earlier in
// the sequence.
func NewRuntimes(byPlatform iter.Seq2[ociv1.Platform, Runtime]) Runtimes {
	return Runtimes{
		runtimes: slices.Collect(func(yield func(runtimePlatform) bool) {
			for platform, runtime := range byPlatform {
				if !yield(runtimePlatform{platform, runtime}) {
					return
				}
			}
		}),
	}
}

// SupportedPlatforms returns all of the platforms that have available runtimes,
// in the order they would be tested by a call to [Runtimes.RuntimeForPlatform].
func (r *Runtimes) SupportedPlatforms() iter.Seq[ociv1.Platform] {
	return func(yield func(ociv1.Platform) bool) {
		for _, rp := range r.runtimes {
			if !yield(rp.Platform) {
				return
			}
		}
	}
}

// RuntimeForPlatform returns a runtime suitable for containers started from
// images targeting the given platform, or nil if no suitable runtime is
// available.
func (r *Runtimes) RuntimeForPlatform(want ociv1.Platform) Runtime {
	for _, rp := range r.runtimes {
		if platformMatches(rp.Platform, want) {
			return rp.Runtime
		}
	}
	return nil
}

// ChooseSupportedDescriptor takes a sequence of descriptors from an index
// manifest and returns a sequence of pairs of descriptors that are supported
// by one of the available runtimes and the runtime that should be used to
// run it.
//
// Note that this function only considers the platform requested in each
// descriptor, and does not consider any other information such as the media
// type of the target. The caller must filter for supported media or artifact
// types either in the input to this function (e.g. using [FilterDescriptors])
// or when processing its results.
//
// Any descriptor with no platform listed is skipped on the assumption that it's
// describing something other than a container image, because container images
// inherently always have a target platform.
//
// The results are in preference order based on the sequence originally given
// to [NewRuntimes], so a typical caller should just take the first result
// and use it, but if the caller has other constraints on what descriptors it's
// allowed to use, e.g. based on artifact type or annotations, then it can keep
// pulling from the sequence until it finds something usable or runs out of
// options.
//
// If none of the available runtimes could support any of the given descriptors
// then the resulting sequence has no items at all.
func (r *Runtimes) ChooseSupportedDescriptor(descs iter.Seq[ociv1.Descriptor]) iter.Seq2[ociv1.Descriptor, Runtime] {
	// Our priority for preference ordering is our internal set of runtimes
	// rather than the order of the given sequence, so we'll just collect
	// all of those into a slice up front so we can scan it multiple times.
	// (No reasonable index manifest should have a large number of descriptors.)
	//
	// We will at least pre-filter the ones with no platform though, since
	// that's easy enough to do up here. We can then assume that all
	// descriptors we visit in the final loop definitely have a platform
	// constraint.
	candidates := slices.Collect(FilterDescriptors(descs, func(d ociv1.Descriptor) bool {
		return d.Platform != nil
	}))
	return func(yield func(ociv1.Descriptor, Runtime) bool) {
		// This approach of re-scanning over the candidates for every platform
		// is not very efficient but we expect the number of runtimes and
		// the number of descriptors to both be small in all reasonable cases,
		// and the caller will stop pulling from this sequence as soon as it
		// gets something usable, so no need to make this complicated.
		for _, rp := range r.runtimes {
			for _, candidate := range candidates {
				if platformMatches(rp.Platform, *candidate.Platform) {
					if !yield(candidate, rp.Runtime) {
						return
					}
				}
			}
		}
	}
}

type runtimePlatform struct {
	Platform ociv1.Platform
	Runtime  Runtime
}

type Runtime interface {
	// ContainerIDBase returns a string that could be used as part of a
	// container ID, based on the repository address and manifest of the
	// image that the container will subsequently be created from.
	//
	// This is intended to allow using container IDs which incorporate some
	// information that a human operator can more easily associate with
	// what is running in the container, but exactly how the resulting string
	// is used is decided by the other method [Runtime.RunContainer].
	ContainerIDBase(registryName, repositoryName string, manifest *ociv1.Manifest) string

	// RunContainer launches a new container using the content of the given
	// bundle directory, which must contain a filesystem bundle as defined
	// in the OCI Runtime Spec:
	//     https://github.com/opencontainers/runtime-spec/blob/main/bundle.md
	//
	// The implementation chooses a final container ID that is at least unique
	// to this container runtime, and possibly also (depending on the underlying
	// technology) unique on the host system on the host system, often using the
	// containerIDBase value as as part of the generated ID but in a way that
	// varies by implementation. Treating the generation of an ID as part of the
	// process of running the container means that the runtime implementation
	// can encapsulate whatever is needed to ensure uniqueness, such as adding a
	// pseudorandom part and retrying until the result is unique.
	//
	// If this method returns successfully then the caller MUST call
	// [ActiveContainer.Close] once the container is no longer needed, or else
	// some or all of the container's resources may not be cleaned up properly.
	//
	// Even if this function succeeds, the container may be killed or otherwise
	// cease operating correctly at any time -- or might not even _begin_
	// operating correctly -- so the caller must be prepared for interactions
	// with the container's streams to fail or return EOF, and for the eventual
	// Close to fail if the container already stopped for some other reason.
	RunContainer(ctx context.Context, containerIDBase string, bundleDir string) (ActiveContainer, error)
}

func platformMatches(got, want ociv1.Platform) bool {
	if got.OS != want.OS || got.Architecture != want.Architecture {
		return false
	}
	// For now we require OSVersion to exactly match, because it's
	// unclear what is a suitable way to treat this as a version
	// constraint. In practice container images intended for OpenTofu
	// should probably avoid requiring a specific OS version at this
	// level and should instead have the software inside check at
	// runtime whether the needed OS features are available, returning
	// an error if not, or use the "OSFeatures" field instead.
	if want.OSVersion != "" && got.OSVersion != want.OSVersion {
		return false
	}
	if want.Variant != "" && got.Variant != want.Variant {
		return false
	}
	for _, wantFeature := range want.OSFeatures {
		if !slices.Contains(got.OSFeatures, wantFeature) {
			return false
		}
	}
	return true
}

// FilterDescriptors takes a sequence of descriptors and produces a sequence of
// only the subset of them for which the given function returns true, preserving
// the given order.
//
// This is here just for convenience when preparing an argument for
// [Runtimes.ChooseSupportedDescriptor]. If a future version of the Go standard
// library includes a generic sequence filter implementation then we should
// remove this and update callers to use that instead.
func FilterDescriptors(all iter.Seq[ociv1.Descriptor], rule func(ociv1.Descriptor) bool) iter.Seq[ociv1.Descriptor] {
	return func(yield func(ociv1.Descriptor) bool) {
		for candidate := range all {
			if rule(candidate) {
				if !yield(candidate) {
					return
				}
			}
		}
	}
}
