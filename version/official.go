// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package version

// officialBuild is set to a non-empty value when we build executables for
// the OpenTofu project's official release packages, and we assume it'll
// be empty for all other builds.
var officialBuild string

// IsOfficialBuild returns true if the current executable was built with
// an argument like the following, which is set by the OpenTofu project's main
// release process:
//
//	-X 'github.com/opentofu/opentofu/version.officialBuild=1'
//
// We use this ONLY to generate warnings where we're intending to stop producing
// official builds for a certain configuration in a future release series and
// so want to warn about it. We generate such warnings only when this is set
// because we want third-parties to be able to create and support their own
// builds for platforms that are not officially supported, and it would be
// very annoying if those builds generated irrelevant warnings about what is
// supported in the official set of release packages.
func IsOfficialBuild() bool {
	return officialBuild != ""
}

// WithFakedOfficialBuild runs the given function with the global "official
// build" flag temporarily forced to the given value, so that [IsOfficialBuild]
// will return that value for the duration of that function's runtime.
//
// This is intended for unit testing only. Because it affects global state, it
// must not be used in parallel tests.
func WithFakedOfficialBuild(official bool, f func()) {
	oldOfficialBuild := officialBuild
	defer func() {
		officialBuild = oldOfficialBuild
	}()
	if official {
		officialBuild = "1"
	} else {
		officialBuild = ""
	}
	f()
}
