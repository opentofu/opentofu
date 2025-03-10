// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"encoding/json"
	"fmt"
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
func NewStateLocker(vt arguments.ViewType, view *View) StateLocker {
	switch vt {
	case arguments.ViewHuman:
		return &StateLockerHuman{view: view}
	case arguments.ViewJSON:
		return &StateLockerJSON{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", vt))
	}
}

// StateLockerHuman is an implementation of StateLocker which prints status to
// a terminal.
type StateLockerHuman struct {
	view *View
}

var _ StateLocker = (*StateLockerHuman)(nil)
var _ StateLocker = (*StateLockerJSON)(nil)

func (v *StateLockerHuman) Locking() {
	v.view.streams.Println("Acquiring state lock. This may take a few moments...") //nolint:errcheck // ui output
}

func (v *StateLockerHuman) Unlocking() {
	v.view.streams.Println("Releasing state lock. This may take a few moments...") //nolint:errcheck // ui output
}

// StateLockerJSON is an implementation of StateLocker which prints the state lock status
// to a terminal in machine-readable JSON form.
type StateLockerJSON struct {
	view *View
}

func (v *StateLockerJSON) Locking() {
	current_timestamp := time.Now().Format(time.RFC3339)

	json_data := map[string]string{
		"@level":     "info",
		"@message":   "Acquiring state lock. This may take a few moments...",
		"@module":    "tofu.ui",
		"@timestamp": current_timestamp,
		"type":       "state_lock_acquire"}

	lock_info_message, _ := json.Marshal(json_data)
	v.view.streams.Println(string(lock_info_message)) //nolint:errcheck // ui output
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
	v.view.streams.Println(string(lock_info_message)) //nolint:errcheck // ui output
}
