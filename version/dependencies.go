// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package version

import "runtime/debug"

// See the docs for InterestingDependencies to understand what "interesting" is
// intended to mean here. We should keep this set relatively small to avoid
// bloating the logs too much.
var interestingDependencies = map[string]struct{}{
	"github.com/opentofu/provider-client":     {},
	"github.com/opentofu/registry-address/v2": {},
	"github.com/opentofu/svchost":             {},
	"github.com/hashicorp/go-getter":          {},
	"github.com/hashicorp/hcl":                {},
	"github.com/hashicorp/hcl/v2":             {},
	"github.com/zclconf/go-cty":               {},
}

// InterestingDependencies returns the compiled-in module version info for
// a small number of dependencies that OpenTofu uses broadly and which we
// tend to upgrade relatively often as part of improvements to OpenTofu.
//
// The set of dependencies this reports might change over time if our
// opinions change about what's "interesting". This is here only to create
// a small number of extra annotations in a debug log to help us more easily
// cross-reference bug reports with dependency changelogs.
func InterestingDependencies() []*debug.Module {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		// Weird to not be built in module mode, but not a big deal.
		return nil
	}

	ret := make([]*debug.Module, 0, len(interestingDependencies))

	for _, mod := range info.Deps {
		if _, ok := interestingDependencies[mod.Path]; !ok {
			continue
		}
		if mod.Replace != nil {
			mod = mod.Replace
		}
		ret = append(ret, mod)
	}

	return ret
}
