// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

// The StateLocker view is used to display locking/unlocking status messages
// if the state lock process takes longer than expected.
type StateLocker interface {
	Locking()
	Unlocking()
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
	_, _ = v.view.streams.Println("Acquiring state lock. This may take a few moments...")
}

func (v *StateLockerHuman) Unlocking() {
	_, _ = v.view.streams.Println("Releasing state lock. This may take a few moments...")
}

// StateLockerJSON is an implementation of StateLocker which prints the state lock status
// to a terminal in machine-readable JSON form.
type StateLockerJSON struct {
	view *JSONView
}

var _ StateLocker = (*StateLockerJSON)(nil)

func (v *StateLockerJSON) Locking() {
	v.view.log.Info("Acquiring state lock. This may take a few moments...", "type", "state_lock_acquire")
}

func (v *StateLockerJSON) Unlocking() {
	v.view.log.Info("Releasing state lock. This may take a few moments...", "type", "state_lock_release")
}
