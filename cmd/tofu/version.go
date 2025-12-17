// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"log"
	"runtime/metrics"
	"strings"

	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/version"
)

var Version = version.Version

var VersionPrerelease = version.Prerelease

// logGodebugUsage produces extra DEBUG log lines if the Go runtime's metrics
// suggest that code that was run so far relied on any non-default "GODEBUG"
// settings, which could be helpful in reproducing a bug report if the
// behavior differs based on such a setting.
//
// For this to be useful it must be run just before we're about to exit, after
// we've already performed all of the requested work.
func logGodebugUsage() {
	// These constants reflect the documented conventions from the
	// runtime/metrics package, so we can filter for only the metrics that
	// are relevant to this function.
	const godebugMetricPrefix = "/godebug/non-default-behavior/"
	const godebugMetricSuffix = ":events"

	if !logging.IsDebugOrHigher() {
		// No point in doing any of this work if the log lines are going to
		// be filtered out anyway.
		return
	}

	metricDescs := metrics.All()
	for _, metric := range metricDescs {
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
			log.Printf("[DEBUG] Relied on GODEBUG %q %d times during this execution; behavior might change in future OpenTofu versions", name, count)
		}
	}
}
