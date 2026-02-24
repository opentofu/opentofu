// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package clistate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/mitchellh/copystructure"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	tfversion "github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// StateVersion is the current supported version for CLI state files.
const StateVersion = 3

type CLIState struct {
	// Version is the state file protocol version.
	Version int `json:"version"`

	// Backend tracks the configuration for the backend in use with
	// this state. This is used to track any changes in the backend
	// configuration.
	Backend *BackendState `json:"backend,omitempty"`

	mu sync.Mutex
}

func (s *CLIState) Lock()   { s.mu.Lock() }
func (s *CLIState) Unlock() { s.mu.Unlock() }

func NewState() *CLIState {
	s := &CLIState{}
	s.init()
	return s
}

func (s *CLIState) Init() {
	s.Lock()
	defer s.Unlock()
	s.init()
}

func (s *CLIState) init() {
	s.Version = StateVersion
}

// DeepCopy performs a deep copy of the CLI state structure and returns
// a new structure.
func (s *CLIState) DeepCopy() *CLIState {
	if s == nil {
		return nil
	}

	cpy, err := copystructure.Config{Lock: true}.Copy(s)
	if err != nil {
		panic(err)
	}

	return cpy.(*CLIState)
}

// BackendState stores the configuration to connect to a backend.
type BackendState struct {
	Type      string          `json:"type"`   // Backend type
	ConfigRaw json.RawMessage `json:"config"` // Backend raw config
	Hash      uint64          `json:"hash"`   // Hash of configuration from config files
}

func (b *BackendState) Empty() bool {
	return b == nil || b.Type == ""
}

func (b *BackendState) Config(schema *configschema.Block) (cty.Value, error) {
	ty := schema.ImpliedType()
	if b == nil {
		return cty.NullVal(ty), nil
	}
	return ctyjson.Unmarshal(b.ConfigRaw, ty)
}

func (b *BackendState) SetConfig(val cty.Value, schema *configschema.Block) error {
	ty := schema.ImpliedType()
	buf, err := ctyjson.Marshal(val, ty)
	if err != nil {
		return err
	}
	b.ConfigRaw = buf
	return nil
}

func (b *BackendState) ForPlan(schema *configschema.Block, workspaceName string) (*plans.Backend, error) {
	if b == nil {
		return nil, nil
	}

	configVal, err := b.Config(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to decode backend config: %w", err)
	}
	return plans.NewBackend(b.Type, configVal, schema, workspaceName)
}

var ErrNoState = errors.New("no state")

type jsonVersionOnly struct {
	Version int `json:"version"`
}

// ReadState reads the CLI state file format written by WriteState.
func ReadState(src io.Reader) (*CLIState, error) {
	if f, ok := src.(*os.File); ok && f == nil {
		return nil, ErrNoState
	}

	buf := bufio.NewReader(src)

	if _, err := buf.Peek(1); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrNoState
		}
		return nil, err
	}

	jsonBytes, err := io.ReadAll(buf)
	if err != nil {
		return nil, fmt.Errorf("reading CLI state file failed: %w", err)
	}

	ver := &jsonVersionOnly{}
	if err := json.Unmarshal(jsonBytes, ver); err != nil {
		return nil, fmt.Errorf("decoding CLI state file version failed: %w", err)
	}

	if ver.Version != StateVersion {
		return nil, fmt.Errorf(
			"OpenTofu %s does not support state version %d, please update.",
			tfversion.SemVer.String(),
			ver.Version,
		)
	}

	st := &CLIState{}
	if err := json.Unmarshal(jsonBytes, st); err != nil {
		return nil, fmt.Errorf("decoding CLI state file failed: %w", err)
	}

	// Ensure the legacy version is set consistently
	st.Version = StateVersion
	return st, nil
}

// WriteState writes CLIState in JSON form.
func WriteState(st *CLIState, dst io.Writer) error {
	if st == nil {
		return nil
	}

	st.Version = StateVersion

	data, err := json.MarshalIndent(st, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to encode CLI state: %w", err)
	}
	data = append(data, '\n')

	if _, err := io.Copy(dst, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to write CLI state: %w", err)
	}

	return nil
}
