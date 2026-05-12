// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"iter"
	"runtime/debug"
	"runtime/metrics"
	"strings"
)

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

var godebugPhaseoutURLs = map[string]string{
	// We added this because Go fixed what was arguably a bug -- interpreting
	// any NTFS reparse point as a symlink, rather than only symlink reparse
	// points -- which we worried that existing OpenTofu users might nonetheless
	// be relying on. However, we're not sure if anyone is actually relying
	// on it so we'd like folks to tell us if they are.
	"winsymlink": "https://github.com/opentofu/opentofu/issues/3415",

	// "tlsmlkem" is another GODEBUG setting we've explicitly overridden in
	// OpenTofu releases, but that one doesn't generate any usage metrics
	// because it just reduces the set of algorithms offered to the remote
	// party during TLS negotiation and so we don't get any signal about
	// whether the request could've worked without that reduced algorithm set.
}

// GodebugActivations returns a sequence the keys of GODEBUG values that were
// set to non-default values and then relied on at any point in the current
// execution before calling this function.
//
// Only the subset of GODEBUG keys that have usage metrics tracked by the Go
// runtime can be returned by this function. Each one is returned with an
// optional string containing a URL where we'd like anyone relying on that
// key to tell us about it so that we can work with them to no longer depend
// on keys that we're intending to remove in a future release. Items whose
// second result is empty are still unexpected, but are not expected unless
// the operator has used the GODEBUG environment variable to change the Go
// runtime behavior in ways that we don't officially support.
func GodebugActivations() iter.Seq2[string, string] {
	// These constants reflect the documented conventions from the
	// runtime/metrics package, so we can filter for only the metrics that
	// are relevant to this function.
	const godebugMetricPrefix = "/godebug/non-default-behavior/"
	const godebugMetricSuffix = ":events"

	return func(yield func(string, string) bool) {
		for _, metric := range metrics.All() {
			if !metric.Cumulative || metric.Kind != metrics.KindUint64 {
				// godebug counters are always cumulative uint64, so this quickly
				// filters some irrelevant metrics before we do any string
				// comparisons.
				continue
			}
			if !strings.HasPrefix(metric.Name, godebugMetricPrefix) {
				continue // irrelevant metric
			}
			if !strings.HasSuffix(metric.Name, godebugMetricSuffix) {
				continue // irrelevant metric
			}
			// The metrics API is designed for applications that want to periodically
			// re-extract the same set of metrics in a long-running application by
			// reusing a preallocated buffer, but we don't have that need here and so
			// we'll just read one metric at a time into a single-element array.
			var samples [1]metrics.Sample
			samples[0].Name = metric.Name
			metrics.Read(samples[:])
			if count := samples[0].Value.Uint64(); count != 0 {
				name := metric.Name[len(godebugMetricPrefix) : len(metric.Name)-len(godebugMetricSuffix)]
				phaseoutURL := godebugPhaseoutURLs[name]
				if !yield(name, phaseoutURL) {
					return
				}
			}
		}
	}
}
