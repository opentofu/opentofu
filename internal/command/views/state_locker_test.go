// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStateLockerViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(view StateLocker)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"locking": {
			viewCall: func(view StateLocker) {
				view.Locking()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Acquiring state lock. This may take a few moments...",
					"@module":  "tofu.ui",
					"type":     "state_lock_acquire",
				},
			},
			wantStdout: `Acquiring state lock. This may take a few moments...
`,
		},
		"unlocking": {
			viewCall: func(view StateLocker) {
				view.Unlocking()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Releasing state lock. This may take a few moments...",
					"@module":  "tofu.ui",
					"type":     "state_lock_release",
				},
			},
			wantStdout: `Releasing state lock. This may take a few moments...
`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testStateLockerHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testStateLockerJson(t, tc.viewCall, tc.wantJson)
			testStateLockerMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testStateLockerHuman(t *testing.T, call func(view StateLocker), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := &StateLockerHuman{view: view}
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testStateLockerJson(t *testing.T, call func(view StateLocker), want []map[string]any) {
	// New type just to assert the fields that we are interested in
	view, done := testView(t)
	v := &StateLockerJSON{view.streams.Stdout.File}
	call(v)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testStateLockerMulti(t *testing.T, call func(view StateLocker), wantStdout string, wantStderr string, want []map[string]any) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	jsonV := &StateLockerJSON{output: jsonInto}
	humanV := &StateLockerHuman{view: view}
	v := StateLockerMulti{humanV, jsonV}
	call(v)
	{
		if err := jsonInto.Close(); err != nil {
			t.Fatalf("failed to close the jsonInto file: %s", err)
		}
		// check the fileInto content
		fileContent, err := os.ReadFile(jsonInto.Name())
		if err != nil {
			t.Fatalf("failed to read the file content with the json output: %s", err)
		}
		testJSONViewOutputEquals(t, string(fileContent), want)
	}
	{
		// check the human output
		output := done(t)
		if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
			t.Errorf("invalid stderr (-want, +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
			t.Errorf("invalid stdout (-want, +got):\n%s", diff)
		}
	}
}
