// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"regexp"

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
	cliUi            cli.Ui
	jsonView         *views.JSONView
	onlyOutputInJSON bool
}

var matchColorRe = regexp.MustCompile("\033\\[[\\d;]*m")

func stripColor(s string) string {
	// This is a workaround for supporting json-into in legacy UI code paths. Hopefully this will all be ripped out once rfc/20251105-use-cobra-instead-of-mitchellh.md
	// and related work is completed.
	//
	// NOTE: The regexp above is specifically tailored to the mitchellh colorstring.go implementation and will NOT work with the *full* set
	// of possible colorization chars.
	return matchColorRe.ReplaceAllString(s, "")
}

func (m *WrappedUi) Ask(s string) (string, error) {
	return m.cliUi.Ask(s)
}

func (m *WrappedUi) AskSecret(s string) (string, error) {
	return m.cliUi.AskSecret(s)
}

func (m *WrappedUi) Output(s string) {
	m.jsonView.Output(stripColor(s))
	if m.onlyOutputInJSON {
		return
	}
	m.cliUi.Output(s)
}

func (m *WrappedUi) Info(s string) {
	m.jsonView.Info(stripColor(s))
	if m.onlyOutputInJSON {
		return
	}
	m.cliUi.Info(s)
}

func (m *WrappedUi) Error(s string) {
	m.jsonView.Error(stripColor(s))
	if m.onlyOutputInJSON {
		return
	}
	m.cliUi.Error(s)
}

func (m *WrappedUi) Warn(s string) {
	m.jsonView.Warn(stripColor(s))
	if m.onlyOutputInJSON {
		return
	}
	m.cliUi.Warn(s)
}
