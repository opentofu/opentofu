// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"os"

	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
)

// ColorizeUi is a Ui implementation that colors its output according
// to the given color schemes for the given type of output.
type ColorizeUi struct {
	Colorize    *colorstring.Colorize
	OutputColor string
	InfoColor   string
	ErrorColor  string
	WarnColor   string
	Ui          cli.Ui
}

func (u *ColorizeUi) Ask(query string) (string, error) {
	return u.Ui.Ask(u.colorize(query, u.OutputColor))
}

func (u *ColorizeUi) AskSecret(query string) (string, error) {
	return u.Ui.AskSecret(u.colorize(query, u.OutputColor))
}

func (u *ColorizeUi) Output(message string) {
	u.Ui.Output(u.colorize(message, u.OutputColor))
}

func (u *ColorizeUi) Info(message string) {
	u.Ui.Info(u.colorize(message, u.InfoColor))
}

func (u *ColorizeUi) Error(message string) {
	u.Ui.Error(u.colorize(message, u.ErrorColor))
}

func (u *ColorizeUi) Warn(message string) {
	u.Ui.Warn(u.colorize(message, u.WarnColor))
}

func (u *ColorizeUi) colorize(message string, color string) string {
	if color == "" {
		return message
	}

	return u.Colorize.Color(fmt.Sprintf("%s%s[reset]", color, message))
}

// ui wraps the primary output [cli.Ui], and redirects Warn calls to Output
// calls. This ensures that warnings are sent to stdout, and are properly
// serialized within the stdout stream.
type ui struct {
	cli.Ui
}

func (u *ui) Warn(msg string) {
	u.Ui.Output(msg)
}

// NewBasicUI returns a preconfigured [cli.Ui] that is meant to be used
// as the primary Ui for OpenTofu.
// TODO meta-refactor: this will have to be removed once everything is moved to views.
func NewBasicUI() cli.Ui {
	return NewWrappedUi(&cli.BasicUi{
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
		Reader:      os.Stdin,
	})
}

func NewWrappedUi(u cli.Ui) cli.Ui {
	return &ui{u}
}
