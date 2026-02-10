// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package clistate

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

func TestReadState_EmptyFile(t *testing.T) {
	reader := bytes.NewReader([]byte{})
	_, err := ReadState(reader)
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}

func TestReadState_NilFile(t *testing.T) {
	var f *os.File
	_, err := ReadState(f)
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("expected ErrNoState, got %T", err)
	}
}

func TestReadState_ValidState(t *testing.T) {
	state := &CLIState{
		Version: StateVersion,
		Serial:  1,
		Lineage: "test-lineage",
	}

	buf := &bytes.Buffer{}
	if err := WriteState(state, buf); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	result, err := ReadState(buf)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}

	if result.Version != StateVersion {
		t.Errorf("expected version %d, got %d", StateVersion, result.Version)
	}
	if result.Serial != 1 {
		t.Errorf("expected serial 1, got %d", result.Serial)
	}
	if result.Lineage != "test-lineage" {
		t.Errorf("expected lineage 'test-lineage', got '%s'", result.Lineage)
	}
}

func TestReadState_InvalidJSON(t *testing.T) {
	reader := strings.NewReader("{invalid json")
	_, err := ReadState(reader)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decoding CLI state file version failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadState_MissingVersion(t *testing.T) {
	reader := strings.NewReader(`{"serial": 1, "lineage": "test"}`)
	_, err := ReadState(reader)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
	if !strings.Contains(err.Error(), "does not support state version 0") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadState_UnsupportedVersion(t *testing.T) {
	reader := strings.NewReader(`{"version": 999, "serial": 1, "lineage": "test"}`)
	_, err := ReadState(reader)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if !strings.Contains(err.Error(), "does not support state version 999") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteState_NilState(t *testing.T) {
	buf := &bytes.Buffer{}
	err := WriteState(nil, buf)
	if err != nil {
		t.Fatalf("WriteState with nil should not error, got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty buffer for nil state, got %d bytes", buf.Len())
	}
}

func TestWriteState_ValidState(t *testing.T) {
	state := &CLIState{
		Version: StateVersion,
		Serial:  42,
		Lineage: "test-lineage",
	}

	buf := &bytes.Buffer{}
	err := WriteState(state, buf)
	if err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	var parsed CLIState
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse written state: %v", err)
	}

	if parsed.Version != StateVersion {
		t.Errorf("expected version %d, got %d", StateVersion, parsed.Version)
	}
	if parsed.Serial != 42 {
		t.Errorf("expected serial 42, got %d", parsed.Serial)
	}
}

func TestWriteState_SetsVersion(t *testing.T) {
	state := &CLIState{
		Version: 99, // Wrong version intentionally
		Serial:  1,
		Lineage: "test",
	}

	buf := &bytes.Buffer{}
	err := WriteState(state, buf)
	if err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	var parsed CLIState
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse written state: %v", err)
	}

	if parsed.Version != StateVersion {
		t.Errorf("expected version to be corrected to %d, got %d", StateVersion, parsed.Version)
	}
}

func TestNewState(t *testing.T) {
	state := NewState()
	if state == nil {
		t.Fatal("NewState returned nil")
	}
	if state.Version != StateVersion {
		t.Errorf("expected version %d, got %d", StateVersion, state.Version)
	}
	if state.Lineage == "" {
		t.Error("expected non-empty lineage")
	}
}

func TestCLIState_Init(t *testing.T) {
	state := &CLIState{}
	state.Init()

	if state.Version != StateVersion {
		t.Errorf("expected version %d, got %d", StateVersion, state.Version)
	}
	if state.Lineage == "" {
		t.Error("expected non-empty lineage after init")
	}
}

func TestCLIState_DeepCopy(t *testing.T) {
	original := &CLIState{
		Version: StateVersion,
		Serial:  5,
		Lineage: "original-lineage",
	}

	copied := original.DeepCopy()
	if copied == nil {
		t.Fatal("DeepCopy returned nil")
	}

	// Verify values match
	if copied.Version != original.Version {
		t.Errorf("version mismatch: %d != %d", copied.Version, original.Version)
	}
	if copied.Serial != original.Serial {
		t.Errorf("serial mismatch: %d != %d", copied.Serial, original.Serial)
	}
	if copied.Lineage != original.Lineage {
		t.Errorf("lineage mismatch: %s != %s", copied.Lineage, original.Lineage)
	}

	copied.Serial = 999
	if original.Serial == 999 {
		t.Error("modifying copy affected original")
	}
}

func TestCLIState_DeepCopy_Nil(t *testing.T) {
	var state *CLIState
	copied := state.DeepCopy()
	if copied != nil {
		t.Errorf("expected nil for DeepCopy of nil, got %T", copied)
	}
}

func TestCLIState_MarshalEqual(t *testing.T) {
	state1 := &CLIState{
		Version: StateVersion,
		Serial:  1,
		Lineage: "test",
	}

	state2 := &CLIState{
		Version: StateVersion,
		Serial:  1,
		Lineage: "test",
	}

	if !state1.MarshalEqual(state2) {
		t.Error("expected identical states to be marshal equal")
	}

	state2.Serial = 2
	if state1.MarshalEqual(state2) {
		t.Error("expected different states to not be marshal equal")
	}
}

func TestCLIState_MarshalEqual_BothNil(t *testing.T) {
	var s1, s2 *CLIState
	if !s1.MarshalEqual(s2) {
		t.Error("expected two nil states to be equal")
	}
}

func TestCLIState_MarshalEqual_OneNil(t *testing.T) {
	state := NewState()
	var nilState *CLIState

	if state.MarshalEqual(nilState) {
		t.Error("expected non-nil and nil state to not be equal")
	}
	if nilState.MarshalEqual(state) {
		t.Error("expected nil and non-nil state to not be equal")
	}
}

func TestRemoteState_Empty(t *testing.T) {
	tests := []struct {
		name     string
		remote   *RemoteState
		expected bool
	}{
		{"nil remote", nil, true},
		{"empty type", &RemoteState{Type: ""}, true},
		{"with type", &RemoteState{Type: "s3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.remote.Empty(); got != tt.expected {
				t.Errorf("Empty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBackendState_Empty(t *testing.T) {
	tests := []struct {
		name     string
		backend  *BackendState
		expected bool
	}{
		{"nil backend", nil, true},
		{"empty type", &BackendState{Type: ""}, true},
		{"with type", &BackendState{Type: "local"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.backend.Empty(); got != tt.expected {
				t.Errorf("Empty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBackendState_ConfigSetAndGet(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"path": {
				Type:     cty.String,
				Optional: true,
			},
		},
	}

	backend := &BackendState{Type: "local"}

	val := cty.ObjectVal(map[string]cty.Value{
		"path": cty.StringVal("/tmp/state"),
	})

	err := backend.SetConfig(val, schema)
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	retrieved, err := backend.Config(schema)
	if err != nil {
		t.Fatalf("Config failed: %v", err)
	}

	if !val.RawEquals(retrieved) {
		t.Errorf("retrieved config doesn't match original:\nwant: %#v\ngot:  %#v", val, retrieved)
	}
}

func TestBackendState_ConfigNil(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"path": {Type: cty.String, Optional: true},
		},
	}

	var backend *BackendState
	val, err := backend.Config(schema)
	if err != nil {
		t.Fatalf("Config on nil backend failed: %v", err)
	}

	expectedType := schema.ImpliedType()
	if !val.Type().Equals(expectedType) {
		t.Errorf("expected type %s for nil backend, got %s", expectedType.FriendlyName(), val.Type().FriendlyName())
	}
	if !val.IsNull() {
		t.Error("expected null value for nil backend")
	}
}

func TestBackendState_ForPlan(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"path": {
				Type:     cty.String,
				Optional: true,
			},
		},
	}

	backend := &BackendState{
		Type: "local",
		Hash: 12345,
	}

	val := cty.ObjectVal(map[string]cty.Value{
		"path": cty.StringVal("/tmp/state"),
	})

	err := backend.SetConfig(val, schema)
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	plan, err := backend.ForPlan(schema, "default")
	if err != nil {
		t.Fatalf("ForPlan failed: %v", err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan backend")
	}
}

func TestBackendState_ForPlanNil(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"path": {Type: cty.String, Optional: true},
		},
	}

	var backend *BackendState
	plan, err := backend.ForPlan(schema, "default")
	if err != nil {
		t.Fatalf("ForPlan on nil backend failed: %v", err)
	}

	if plan != nil {
		t.Errorf("expected nil plan for nil backend, got %v", plan)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	original := &CLIState{
		Version: StateVersion,
		Serial:  100,
		Lineage: "test-lineage-456",
		Backend: &BackendState{
			Type: "s3",
			Hash: 12345,
		},
		Remote: &RemoteState{
			Type: "consul",
			Config: map[string]string{
				"address": "localhost:8500",
			},
		},
	}

	buf := &bytes.Buffer{}
	if err := WriteState(original, buf); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	result, err := ReadState(buf)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}

	if result.Serial != original.Serial {
		t.Errorf("serial mismatch: got %d, want %d", result.Serial, original.Serial)
	}
	if result.Lineage != original.Lineage {
		t.Errorf("lineage mismatch: got %q, want %q", result.Lineage, original.Lineage)
	}
	if result.Backend == nil || result.Backend.Type != "s3" {
		if result.Backend == nil {
			t.Errorf("backend mismatch: got nil, want type %q", "s3")
		} else {
			t.Errorf("backend mismatch: got type %q, want %q", result.Backend.Type, "s3")
		}
	}
	if result.Remote == nil || result.Remote.Type != "consul" {
		if result.Remote == nil {
			t.Errorf("remote mismatch: got nil, want type %q", "consul")
		} else {
			t.Errorf("remote mismatch: got type %q, want %q", result.Remote.Type, "consul")
		}
	}
}
