// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2026 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import "sync/atomic"

// RefreshMode specifies how refresh should be handled during planning.
type RefreshMode int

const (
	// RefreshAll refreshes all resources (default behavior).
	RefreshAll RefreshMode = iota
	// RefreshNone skips refresh for all resources (equivalent to -refresh=false).
	RefreshNone
	// RefreshConfig refreshes only resources whose configuration changed.
	RefreshConfig
)

// String returns a human-readable name for the refresh mode.
func (m RefreshMode) String() string {
	switch m {
	case RefreshAll:
		return "all"
	case RefreshNone:
		return "none"
	case RefreshConfig:
		return "config"
	default:
		return "unknown"
	}
}

// RefreshStats tracks refresh behavior for managed resources and data sources.
type RefreshStats struct {
	managedTotal     atomic.Int64
	managedRefreshed atomic.Int64
	dataTotal        atomic.Int64
	dataRefreshed    atomic.Int64
}

// NewRefreshStats creates an empty RefreshStats instance.
func NewRefreshStats() *RefreshStats {
	return &RefreshStats{}
}

// RecordManaged records whether a managed resource was refreshed.
func (s *RefreshStats) RecordManaged(refreshed bool) {
	s.managedTotal.Add(1)
	if refreshed {
		s.managedRefreshed.Add(1)
	}
}

// RecordDataSource records whether a data source was executed.
func (s *RefreshStats) RecordDataSource(executed bool) {
	s.dataTotal.Add(1)
	if executed {
		s.dataRefreshed.Add(1)
	}
}

// ManagedCounts returns total/refreshed/skipped for managed resources.
func (s *RefreshStats) ManagedCounts() (total, refreshed, skipped int64) {
	total = s.managedTotal.Load()
	refreshed = s.managedRefreshed.Load()
	skipped = total - refreshed
	return total, refreshed, skipped
}

// DataSourceCounts returns total/executed/skipped for data sources.
func (s *RefreshStats) DataSourceCounts() (total, executed, skipped int64) {
	total = s.dataTotal.Load()
	executed = s.dataRefreshed.Load()
	skipped = total - executed
	return total, executed, skipped
}
