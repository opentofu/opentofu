// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/command/views"
)

// WrappedUi is a shim which adds json compatibility to those commands which
// have not yet been refactored to support output by views.View.
//
// For those not support json output command, all output is printed by cli.Ui.
// So we create WrappedUi, contains the old cli.Ui and views.JSONView,
// implement cli.Ui interface, so that we can make all command support json
// output in a short time.
type WrappedUi struct {
	cliUi        cli.Ui
	jsonView     *views.JSONView
	outputInJSON bool
}

func (m *WrappedUi) Ask(s string) (string, error) {
	return m.cliUi.Ask(s)
}

func (m *WrappedUi) AskSecret(s string) (string, error) {
	return m.cliUi.AskSecret(s)
}

func (m *WrappedUi) Output(s string) {
	if m.outputInJSON {
		m.jsonView.Output(s)
		return
	}
	m.cliUi.Output(s)
}

func (m *WrappedUi) Info(s string) {
	if m.outputInJSON {
		m.jsonView.Info(s)
		return
	}
	m.cliUi.Info(s)
}

func (m *WrappedUi) Error(s string) {
	if m.outputInJSON {
		m.jsonView.Error(s)
		return
	}
	m.cliUi.Error(s)
}

func (m *WrappedUi) Warn(s string) {
	if m.outputInJSON {
		m.jsonView.Warn(s)
		return
	}
	m.cliUi.Warn(s)
}
