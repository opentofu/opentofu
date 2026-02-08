// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
)

// TestContext2Plan_refreshConfigUnchangedSkipsRefresh verifies that when
// RefreshMode is RefreshConfig and the resource configuration hasn't changed,
// the provider's ReadResource is not called.
func TestContext2Plan_refreshConfigUnchangedSkipsRefresh(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
  ami = "bar"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"a","ami":"bar","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	if p.ReadResourceCalled {
		t.Fatal("ReadResource should NOT have been called when config is unchanged")
	}

	for _, c := range plan.Changes.Resources {
		if c.Action != plans.NoOp {
			t.Fatalf("expected NoOp, got %s for %q", c.Action, c.Addr)
		}
	}
}

// TestContext2Plan_refreshConfigChangedTriggersRefresh verifies that when
// RefreshMode is RefreshConfig and the resource configuration HAS changed,
// the provider's ReadResource IS called.
func TestContext2Plan_refreshConfigChangedTriggersRefresh(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
  ami = "new-ami"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"a","ami":"old-ami","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	if !p.ReadResourceCalled {
		t.Fatal("ReadResource SHOULD have been called when config has changed")
	}
}

// TestContext2Plan_refreshConfigDataSourceUnchangedNoManagedDep verifies that
// a data source with no managed resource dependencies and unchanged config
// is skipped under RefreshConfig mode.
func TestContext2Plan_refreshConfigDataSourceUnchangedNoManagedDep(t *testing.T) {
	p := testProvider("test")

	var readDataCalled atomic.Bool
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		readDataCalled.Store(true)
		return providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"id":  cty.StringVal("data-id"),
				"foo": cty.StringVal("bar"),
			}),
		}
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
data "test_data_source" "d" {
  foo = "bar"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("data.test_data_source.d").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data-id","foo":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	if readDataCalled.Load() {
		t.Fatal("ReadDataSource should NOT have been called for unchanged data source with no managed deps")
	}
}

// TestContext2Plan_refreshConfigDataSourceChangedExecutes verifies that
// a data source whose config has changed IS executed even in RefreshConfig mode.
func TestContext2Plan_refreshConfigDataSourceChangedExecutes(t *testing.T) {
	p := testProvider("test")

	var readDataCalled atomic.Bool
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		readDataCalled.Store(true)
		return providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"id":  cty.StringVal("data-id"),
				"foo": cty.StringVal("new-value"),
			}),
		}
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
data "test_data_source" "d" {
  foo = "new-value"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("data.test_data_source.d").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data-id","foo":"old-value"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	if !readDataCalled.Load() {
		t.Fatal("ReadDataSource SHOULD have been called when data source config changed")
	}
}

// TestContext2Plan_refreshConfigDataSourceWithManagedDepAlwaysExecutes verifies
// that a data source with a dependency on a managed resource always executes,
// even if its own configuration is unchanged.
func TestContext2Plan_refreshConfigDataSourceWithManagedDepAlwaysExecutes(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	var readDataCalled atomic.Bool
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		readDataCalled.Store(true)
		return providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"id":  cty.StringVal("data-id"),
				"foo": cty.StringVal("bar"),
			}),
		}
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
  ami = "bar"
}

data "test_data_source" "d" {
  foo = "bar"
  depends_on = [test_instance.a]
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"a","ami":"bar","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("data.test_data_source.d").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data-id","foo":"bar"}`),
			Dependencies: []addrs.ConfigResource{
				{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_instance",
						Name: "a",
					},
				},
			},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	if !readDataCalled.Load() {
		t.Fatal("ReadDataSource SHOULD have been called for data source with managed resource dependency")
	}
}

// TestContext2Plan_refreshConfigStatsCounters verifies the RefreshStats
// correctly counts managed resources and data sources.
func TestContext2Plan_refreshConfigStatsCounters(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		return providers.ReadDataSourceResponse{
			State: cty.ObjectVal(map[string]cty.Value{
				"id":  cty.StringVal("data-id"),
				"foo": cty.NullVal(cty.String),
			}),
		}
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "unchanged" {
  ami = "bar"
}

resource "test_instance" "changed" {
  ami = "new-ami"
}

data "test_data_source" "d" {
  foo = "bar"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.unchanged").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"u","ami":"bar","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.changed").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"c","ami":"old-ami","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("data.test_data_source.d").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"data-id","foo":"bar"}`),
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	stats := NewRefreshStats()
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:         plans.NormalMode,
		RefreshMode:  RefreshConfig,
		RefreshStats: stats,
	})
	assertNoErrors(t, diags)

	managedTotal, managedRefreshed, managedSkipped := stats.ManagedCounts()
	if managedTotal != 2 {
		t.Errorf("managed total = %d, want 2", managedTotal)
	}
	if managedRefreshed != 1 {
		t.Errorf("managed refreshed = %d, want 1 (the changed resource)", managedRefreshed)
	}
	if managedSkipped != 1 {
		t.Errorf("managed skipped = %d, want 1 (the unchanged resource)", managedSkipped)
	}

	dataTotal, _, _ := stats.DataSourceCounts()
	if dataTotal != 1 {
		t.Errorf("data source total = %d, want 1", dataTotal)
	}
}

// TestContext2Plan_refreshConfigWithRefreshOnlyError verifies that combining
// RefreshConfig mode with RefreshOnly mode produces an error.
func TestContext2Plan_refreshConfigWithRefreshOnlyError(t *testing.T) {
	p := testProvider("test")

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
  ami = "bar"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"a","ami":"bar","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.RefreshOnlyMode,
		RefreshMode: RefreshConfig,
	})

	if !diags.HasErrors() {
		t.Fatal("expected error for RefreshConfig + RefreshOnlyMode, got none")
	}

	found := false
	for _, d := range diags {
		desc := d.Description()
		if strings.Contains(desc.Detail, "refresh=config") || strings.Contains(desc.Detail, "refresh-only") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about refresh=config incompatible with refresh-only, got: %s", diags.Err())
	}
}

// TestContext2Plan_refreshConfigStatsWarning verifies that the stats warning
// is emitted when using RefreshConfig mode.
func TestContext2Plan_refreshConfigStatsWarning(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
  ami = "bar"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"a","ami":"bar","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})

	// Should not have errors
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	// Should have a warning about selective refresh
	foundWarning := false
	for _, d := range diags {
		desc := d.Description()
		if strings.Contains(desc.Summary, "Selective refresh") ||
			strings.Contains(desc.Detail, "Selective refresh mode") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected 'Selective refresh' warning, but none was found in diagnostics")
	}
}

// TestContext2Plan_refreshConfigNewResourceRefreshes verifies that a new
// resource (not in state) is always refreshed even in RefreshConfig mode.
func TestContext2Plan_refreshConfigNewResourceRefreshes(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "a" {
  ami = "bar"
}
`,
	})

	// Empty state - the resource doesn't exist yet
	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	// Should plan a Create action for the new resource
	foundCreate := false
	for _, c := range plan.Changes.Resources {
		if c.Addr.String() == "test_instance.a" && c.Action == plans.Create {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Fatal("expected Create action for new resource test_instance.a")
	}
}

// TestContext2Plan_refreshConfigOrphanRefreshes verifies that orphan resources
// (in state but not in config) always refresh in RefreshConfig mode.
func TestContext2Plan_refreshConfigOrphanRefreshes(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	// Config has no resources
	m := testModuleInline(t, map[string]string{
		"main.tf": ``,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.a").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"a","ami":"bar","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	plan, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	// Orphans should be planned for deletion
	foundDelete := false
	for _, c := range plan.Changes.Resources {
		if c.Addr.String() == "test_instance.a" && c.Action == plans.Delete {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Fatal("expected Delete action for orphan resource test_instance.a")
	}

	// ReadResource should be called for the orphan since it has no config
	if !p.ReadResourceCalled {
		t.Fatal("ReadResource should have been called for orphan resource (no config = always refresh)")
	}
}

// TestContext2Plan_refreshConfigMultipleResources verifies a mixed scenario
// with multiple managed resources where some change and some don't.
func TestContext2Plan_refreshConfigMultipleResources(t *testing.T) {
	p := testProvider("test")
	p.PlanResourceChangeFn = testDiffFn

	var readCount atomic.Int32
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		readCount.Add(1)
		return providers.ReadResourceResponse{
			NewState: req.PriorState,
		}
	}

	m := testModuleInline(t, map[string]string{
		"main.tf": `
resource "test_instance" "unchanged1" {
  ami = "same"
}

resource "test_instance" "unchanged2" {
  ami = "same2"
}

resource "test_instance" "changed" {
  ami = "new-value"
}
`,
	})

	state := states.NewState()
	root := state.EnsureModule(addrs.RootModuleInstance)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.unchanged1").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"u1","ami":"same","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.unchanged2").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"u2","ami":"same2","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)
	root.SetResourceInstanceCurrent(
		mustResourceInstanceAddr("test_instance.changed").Resource,
		&states.ResourceInstanceObjectSrc{
			Status:       states.ObjectReady,
			AttrsJSON:    []byte(`{"id":"c","ami":"old-value","type":"test_instance"}`),
			Dependencies: []addrs.ConfigResource{},
		},
		mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`),
		addrs.NoKey,
	)

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	})

	_, diags := ctx.Plan(context.Background(), m, state, &PlanOpts{
		Mode:        plans.NormalMode,
		RefreshMode: RefreshConfig,
	})
	assertNoErrors(t, diags)

	// Only the changed resource should have triggered a ReadResource call
	count := readCount.Load()
	if count != 1 {
		t.Errorf("ReadResource called %d times, want 1 (only the changed resource)", count)
	}
}
