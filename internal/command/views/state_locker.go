// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/opentofu/opentofu/internal/command/arguments"
)

// The StateLocker view is used to display locking/unlocking status messages
// if the state lock process takes longer than expected.
type StateLocker interface {
	Locking()
	Unlocking()
}

// NewStateLocker returns an initialized StateLocker implementation for the given ViewType.
func NewStateLocker(args arguments.ViewOptions, view *View) StateLocker {
	var state StateLocker
	switch args.ViewType {
	case arguments.ViewHuman:
		state = &StateLockerHuman{view: view}
	case arguments.ViewJSON:
		state = &StateLockerJSON{output: view.streams.Stdout.File}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		state = StateLockerMulti{state, &StateLockerJSON{output: args.JSONInto}}
	}

	return state
}

type StateLockerMulti []StateLocker

var _ StateLocker = (StateLockerMulti)(nil)

func (m StateLockerMulti) Locking() {
	for _, s := range m {
		s.Locking()
	}
}

func (m StateLockerMulti) Unlocking() {
	for _, s := range m {
		s.Unlocking()
	}
}

// StateLockerHuman is an implementation of StateLocker which prints status to
// a terminal.
type StateLockerHuman struct {
	view *View
}

var _ StateLocker = (*StateLockerHuman)(nil)

func (v *StateLockerHuman) Locking() {
	v.view.streams.Println("Acquiring state lock. This may take a few moments...")
}

func (v *StateLockerHuman) Unlocking() {
	v.view.streams.Println("Releasing state lock. This may take a few moments...")
}

// StateLockerJSON is an implementation of StateLocker which prints the state lock status
// to a terminal in machine-readable JSON form.
type StateLockerJSON struct {
	output *os.File
}

var _ StateLocker = (*StateLockerJSON)(nil)

func (v *StateLockerJSON) Locking() {
	current_timestamp := time.Now().Format(time.RFC3339)

	json_data := map[string]string{
		"@level":     "info",
		"@message":   "Acquiring state lock. This may take a few moments...",
		"@module":    "tofu.ui",
		"@timestamp": current_timestamp,
		"type":       "state_lock_acquire"}

	lock_info_message, _ := json.Marshal(json_data)
	fmt.Fprintln(v.output, string(lock_info_message))
}

func (v *StateLockerJSON) Unlocking() {
	current_timestamp := time.Now().Format(time.RFC3339)

	json_data := map[string]string{
		"@level":     "info",
		"@message":   "Releasing state lock. This may take a few moments...",
		"@module":    "tofu.ui",
		"@timestamp": current_timestamp,
		"type":       "state_lock_release"}

	lock_info_message, _ := json.Marshal(json_data)
	fmt.Fprintln(v.output, string(lock_info_message))
}
