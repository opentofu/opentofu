// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"sync"
	"testing"
)

func TestRefreshMode_String(t *testing.T) {
	tests := []struct {
		mode RefreshMode
		want string
	}{
		{RefreshAll, "all"},
		{RefreshNone, "none"},
		{RefreshConfig, "config"},
		{RefreshMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("RefreshMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestRefreshStats_ManagedCounts(t *testing.T) {
	s := NewRefreshStats()

	s.RecordManaged(true)
	s.RecordManaged(true)
	s.RecordManaged(false)
	s.RecordManaged(false)
	s.RecordManaged(false)

	total, refreshed, skipped := s.ManagedCounts()
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if refreshed != 2 {
		t.Errorf("refreshed = %d, want 2", refreshed)
	}
	if skipped != 3 {
		t.Errorf("skipped = %d, want 3", skipped)
	}
}

func TestRefreshStats_DataSourceCounts(t *testing.T) {
	s := NewRefreshStats()

	s.RecordDataSource(true)
	s.RecordDataSource(false)
	s.RecordDataSource(true)

	total, executed, skipped := s.DataSourceCounts()
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if executed != 2 {
		t.Errorf("executed = %d, want 2", executed)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestRefreshStats_Empty(t *testing.T) {
	s := NewRefreshStats()

	total, refreshed, skipped := s.ManagedCounts()
	if total != 0 || refreshed != 0 || skipped != 0 {
		t.Errorf("empty managed counts: total=%d, refreshed=%d, skipped=%d", total, refreshed, skipped)
	}

	total, executed, skipped := s.DataSourceCounts()
	if total != 0 || executed != 0 || skipped != 0 {
		t.Errorf("empty data source counts: total=%d, executed=%d, skipped=%d", total, executed, skipped)
	}
}

func TestRefreshStats_ConcurrentSafety(t *testing.T) {
	s := NewRefreshStats()
	const goroutines = 100
	const recordsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half goroutines record managed, half record data sources
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				s.RecordManaged(j%2 == 0)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				s.RecordDataSource(j%3 == 0)
			}
		}()
	}

	wg.Wait()

	managedTotal, managedRefreshed, managedSkipped := s.ManagedCounts()
	dataTotal, dataExecuted, dataSkipped := s.DataSourceCounts()

	expectedManagedTotal := int64(goroutines * recordsPerGoroutine)
	expectedDataTotal := int64(goroutines * recordsPerGoroutine)

	if managedTotal != expectedManagedTotal {
		t.Errorf("managed total = %d, want %d", managedTotal, expectedManagedTotal)
	}
	if managedRefreshed+managedSkipped != managedTotal {
		t.Errorf("managed refreshed(%d) + skipped(%d) != total(%d)", managedRefreshed, managedSkipped, managedTotal)
	}
	if dataTotal != expectedDataTotal {
		t.Errorf("data total = %d, want %d", dataTotal, expectedDataTotal)
	}
	if dataExecuted+dataSkipped != dataTotal {
		t.Errorf("data executed(%d) + skipped(%d) != total(%d)", dataExecuted, dataSkipped, dataTotal)
	}
}
