// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

// Test a sequence of hooks associated with creating a resource
func TestJSONHook_create(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	hook := newJSONHook(NewJSONView(NewView(streams), nil))

	var nowMu sync.Mutex
	now := time.Now()
	hook.timeNow = func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return now
	}

	after := make(chan time.Time, 1)
	hook.timeAfter = func(time.Duration) <-chan time.Time { return after }

	addr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_instance",
		Name: "boop",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
	priorState := cty.NullVal(cty.Object(map[string]cty.Type{
		"id":  cty.String,
		"bar": cty.List(cty.String),
	}))
	plannedNewState := cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("test"),
		"bar": cty.ListVal([]cty.Value{
			cty.StringVal("baz"),
		}),
	})

	action, err := hook.PreApply(addr, states.CurrentGen, plans.Create, priorState, plannedNewState)
	testHookReturnValues(t, action, err)

	action, err = hook.PreProvisionInstanceStep(addr, "local-exec")
	testHookReturnValues(t, action, err)

	hook.ProvisionOutput(addr, "local-exec", `Executing: ["/bin/sh" "-c" "touch /etc/motd"]`)

	action, err = hook.PostProvisionInstanceStep(addr, "local-exec", nil)
	testHookReturnValues(t, action, err)

	elapsedChan := hook.applying[addr.String()].elapsed

	// Travel 10s into the future, notify the progress goroutine, and wait
	// for execution via 'elapsed' progress
	nowMu.Lock()
	now = now.Add(10 * time.Second)
	after <- now
	nowMu.Unlock()
	elapsed := <-elapsedChan
	testDurationEqual(t, 10*time.Second, elapsed)

	// Travel 10s into the future, notify the progress goroutine, and wait
	// for execution via 'elapsed' progress
	nowMu.Lock()
	now = now.Add(10 * time.Second)
	after <- now
	nowMu.Unlock()
	elapsed = <-elapsedChan
	testDurationEqual(t, 20*time.Second, elapsed)

	// Travel 2s into the future. We have arrived!
	nowMu.Lock()
	now = now.Add(2 * time.Second)
	nowMu.Unlock()

	action, err = hook.PostApply(addr, states.CurrentGen, plannedNewState, nil)
	testHookReturnValues(t, action, err)

	// Shut down the progress goroutine if still active
	hook.applyingLock.Lock()
	for key, progress := range hook.applying {
		close(progress.done)
		close(progress.elapsed)
		<-progress.heartbeatDone
		delete(hook.applying, key)
	}
	hook.applyingLock.Unlock()

	wantResource := map[string]interface{}{
		"addr":             string("test_instance.boop"),
		"implied_provider": string("test"),
		"module":           string(""),
		"resource":         string("test_instance.boop"),
		"resource_key":     nil,
		"resource_name":    string("boop"),
		"resource_type":    string("test_instance"),
	}
	want := []map[string]interface{}{
		{
			"@level":   "info",
			"@message": "test_instance.boop: Creating...",
			"@module":  "tofu.ui",
			"type":     "apply_start",
			"hook": map[string]interface{}{
				"action":   string("create"),
				"resource": wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: Provisioning with 'local-exec'...",
			"@module":  "tofu.ui",
			"type":     "provision_start",
			"hook": map[string]interface{}{
				"provisioner": "local-exec",
				"resource":    wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": `test_instance.boop: (local-exec): Executing: ["/bin/sh" "-c" "touch /etc/motd"]`,
			"@module":  "tofu.ui",
			"type":     "provision_progress",
			"hook": map[string]interface{}{
				"output":      `Executing: ["/bin/sh" "-c" "touch /etc/motd"]`,
				"provisioner": "local-exec",
				"resource":    wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: (local-exec) Provisioning complete",
			"@module":  "tofu.ui",
			"type":     "provision_complete",
			"hook": map[string]interface{}{
				"provisioner": "local-exec",
				"resource":    wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: Still creating... [10s elapsed]",
			"@module":  "tofu.ui",
			"type":     "apply_progress",
			"hook": map[string]interface{}{
				"action":          string("create"),
				"elapsed_seconds": float64(10),
				"resource":        wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: Still creating... [20s elapsed]",
			"@module":  "tofu.ui",
			"type":     "apply_progress",
			"hook": map[string]interface{}{
				"action":          string("create"),
				"elapsed_seconds": float64(20),
				"resource":        wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: Creation complete after 22s [id=test]",
			"@module":  "tofu.ui",
			"type":     "apply_complete",
			"hook": map[string]interface{}{
				"action":          string("create"),
				"elapsed_seconds": float64(22),
				"id_key":          "id",
				"id_value":        "test",
				"resource":        wantResource,
			},
		},
	}

	testJSONViewOutputEquals(t, done(t).Stdout(), want)
}

func TestJSONHook_errors(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	hook := newJSONHook(NewJSONView(NewView(streams), nil))

	addr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_instance",
		Name: "boop",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
	priorState := cty.NullVal(cty.Object(map[string]cty.Type{
		"id":  cty.String,
		"bar": cty.List(cty.String),
	}))
	plannedNewState := cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("test"),
		"bar": cty.ListVal([]cty.Value{
			cty.StringVal("baz"),
		}),
	})

	action, err := hook.PreApply(addr, states.CurrentGen, plans.Delete, priorState, plannedNewState)
	testHookReturnValues(t, action, err)

	provisionError := fmt.Errorf("provisioner didn't want to")
	action, err = hook.PostProvisionInstanceStep(addr, "local-exec", provisionError)
	testHookReturnValues(t, action, err)

	applyError := fmt.Errorf("provider was sad")
	action, err = hook.PostApply(addr, states.CurrentGen, plannedNewState, applyError)
	testHookReturnValues(t, action, err)

	// Shut down the progress goroutine
	hook.applyingLock.Lock()
	for key, progress := range hook.applying {
		close(progress.done)
		close(progress.elapsed)
		<-progress.heartbeatDone
		delete(hook.applying, key)
	}
	hook.applyingLock.Unlock()

	wantResource := map[string]interface{}{
		"addr":             string("test_instance.boop"),
		"implied_provider": string("test"),
		"module":           string(""),
		"resource":         string("test_instance.boop"),
		"resource_key":     nil,
		"resource_name":    string("boop"),
		"resource_type":    string("test_instance"),
	}
	want := []map[string]interface{}{
		{
			"@level":   "info",
			"@message": "test_instance.boop: Destroying...",
			"@module":  "tofu.ui",
			"type":     "apply_start",
			"hook": map[string]interface{}{
				"action":   string("delete"),
				"resource": wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: (local-exec) Provisioning errored",
			"@module":  "tofu.ui",
			"type":     "provision_errored",
			"hook": map[string]interface{}{
				"provisioner": "local-exec",
				"resource":    wantResource,
			},
		},
		{
			"@level":   "info",
			"@message": "test_instance.boop: Destruction errored after 0s",
			"@module":  "tofu.ui",
			"type":     "apply_errored",
			"hook": map[string]interface{}{
				"action":          string("delete"),
				"elapsed_seconds": float64(0),
				"resource":        wantResource,
			},
		},
	}

	testJSONViewOutputEquals(t, done(t).Stdout(), want)
}

func TestJSONHook_refresh(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	hook := newJSONHook(NewJSONView(NewView(streams), nil))

	addr := addrs.Resource{
		Mode: addrs.DataResourceMode,
		Type: "test_data_source",
		Name: "beep",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
	state := cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("honk"),
		"bar": cty.ListVal([]cty.Value{
			cty.StringVal("baz"),
		}),
	})

	action, err := hook.PreRefresh(addr, states.CurrentGen, state)
	testHookReturnValues(t, action, err)

	action, err = hook.PostRefresh(addr, states.CurrentGen, state, state)
	testHookReturnValues(t, action, err)

	wantResource := map[string]interface{}{
		"addr":             string("data.test_data_source.beep"),
		"implied_provider": string("test"),
		"module":           string(""),
		"resource":         string("data.test_data_source.beep"),
		"resource_key":     nil,
		"resource_name":    string("beep"),
		"resource_type":    string("test_data_source"),
	}
	want := []map[string]interface{}{
		{
			"@level":   "info",
			"@message": "data.test_data_source.beep: Refreshing state... [id=honk]",
			"@module":  "tofu.ui",
			"type":     "refresh_start",
			"hook": map[string]interface{}{
				"resource": wantResource,
				"id_key":   "id",
				"id_value": "honk",
			},
		},
		{
			"@level":   "info",
			"@message": "data.test_data_source.beep: Refresh complete [id=honk]",
			"@module":  "tofu.ui",
			"type":     "refresh_complete",
			"hook": map[string]interface{}{
				"resource": wantResource,
				"id_key":   "id",
				"id_value": "honk",
			},
		},
	}

	testJSONViewOutputEquals(t, done(t).Stdout(), want)
}

func TestJSONHook_ephemeral(t *testing.T) {
	addr := addrs.Resource{
		Mode: addrs.EphemeralResourceMode,
		Type: "test_instance",
		Name: "foo",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)

	cases := []struct {
		name  string
		preF  func(hook tofu.Hook) (tofu.HookAction, error)
		postF func(hook tofu.Hook) (tofu.HookAction, error)
		want  []map[string]interface{}
	}{
		{
			name: "opening",
			preF: func(hook tofu.Hook) (tofu.HookAction, error) {
				return hook.PreOpen(addr)
			},
			postF: func(hook tofu.Hook) (tofu.HookAction, error) {
				return hook.PostOpen(addr, nil)
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "ephemeral.test_instance.foo: Opening...",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Opening...",
						"resource": map[string]any{
							"addr":             "ephemeral.test_instance.foo",
							"implied_provider": "test",
							"module":           "",
							"resource":         "ephemeral.test_instance.foo",
							"resource_key":     nil,
							"resource_name":    "foo",
							"resource_type":    "test_instance",
						},
					},
					"type": "ephemeral_action_started",
				},
				{
					"@level":   "info",
					"@message": "ephemeral.test_instance.foo: Open complete",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Open complete",
						"resource": map[string]any{
							"addr":             "ephemeral.test_instance.foo",
							"implied_provider": "test",
							"module":           "",
							"resource":         "ephemeral.test_instance.foo",
							"resource_key":     nil,
							"resource_name":    "foo",
							"resource_type":    "test_instance",
						},
					},
					"type": "ephemeral_action_complete",
				},
			},
		},
		{
			name: "renewing",
			preF: func(hook tofu.Hook) (tofu.HookAction, error) {
				return hook.PreRenew(addr)
			},
			postF: func(hook tofu.Hook) (tofu.HookAction, error) {
				return hook.PostRenew(addr, nil)
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "ephemeral.test_instance.foo: Renewing...",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Renewing...",
						"resource": map[string]any{
							"addr":             "ephemeral.test_instance.foo",
							"implied_provider": "test",
							"module":           "",
							"resource":         "ephemeral.test_instance.foo",
							"resource_key":     nil,
							"resource_name":    "foo",
							"resource_type":    "test_instance",
						},
					},
					"type": "ephemeral_action_started",
				},
				{
					"@level":   "info",
					"@message": "ephemeral.test_instance.foo: Renew complete",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Renew complete",
						"resource": map[string]any{
							"addr":             "ephemeral.test_instance.foo",
							"implied_provider": "test",
							"module":           "",
							"resource":         "ephemeral.test_instance.foo",
							"resource_key":     nil,
							"resource_name":    "foo",
							"resource_type":    "test_instance",
						},
					},
					"type": "ephemeral_action_complete",
				},
			},
		},
		{
			name: "closing",
			preF: func(hook tofu.Hook) (tofu.HookAction, error) {
				return hook.PreClose(addr)
			},
			postF: func(hook tofu.Hook) (tofu.HookAction, error) {
				return hook.PostClose(addr, nil)
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "ephemeral.test_instance.foo: Closing...",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Closing...",
						"resource": map[string]any{
							"addr":             "ephemeral.test_instance.foo",
							"implied_provider": "test",
							"module":           "",
							"resource":         "ephemeral.test_instance.foo",
							"resource_key":     nil,
							"resource_name":    "foo",
							"resource_type":    "test_instance",
						},
					},
					"type": "ephemeral_action_started",
				},
				{
					"@level":   "info",
					"@message": "ephemeral.test_instance.foo: Close complete",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Close complete",
						"resource": map[string]any{
							"addr":             "ephemeral.test_instance.foo",
							"implied_provider": "test",
							"module":           "",
							"resource":         "ephemeral.test_instance.foo",
							"resource_key":     nil,
							"resource_name":    "foo",
							"resource_type":    "test_instance",
						},
					},
					"type": "ephemeral_action_complete",
				},
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			h := newJSONHook(NewJSONView(NewView(streams), nil))

			action, err := tt.preF(h)
			if err != nil {
				t.Fatal(err)
			}
			if action != tofu.HookActionContinue {
				t.Fatalf("Expected hook to continue, given: %#v", action)
			}

			<-time.After(1100 * time.Millisecond)

			// call postF that will stop the waiting for the action
			action, err = tt.postF(h)
			if err != nil {
				t.Fatal(err)
			}
			if action != tofu.HookActionContinue {
				t.Errorf("Expected hook to continue, given: %#v", action)
			}

			testJSONViewOutputEquals(t, done(t).Stdout(), tt.want)
		})
	}
}

func testHookReturnValues(t *testing.T, action tofu.HookAction, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
	if action != tofu.HookActionContinue {
		t.Fatalf("Expected hook to continue, given: %#v", action)
	}
}

func testDurationEqual(t *testing.T, wantedDuration time.Duration, gotDuration time.Duration) {
	t.Helper()

	if !cmp.Equal(wantedDuration, gotDuration) {
		t.Errorf("unexpected time elapsed:%s\n", cmp.Diff(wantedDuration, gotDuration))
	}
}
