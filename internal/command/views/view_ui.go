// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"regexp"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
)

var ErrorInputDisabled = fmt.Errorf("in this view cannot ask user input")

var _ cli.Ui = (*ViewUiHuman)(nil)
var _ cli.Ui = (*ViewUiJSON)(nil)
var _ cli.Ui = (*ViewUiMulti)(nil)

func NewViewUI(args arguments.ViewOptions, view *View, oldUi cli.Ui) cli.Ui {
	var ret cli.Ui
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &ViewUiJSON{
			view: NewJSONView(view, nil),
		}
	case arguments.ViewHuman:
		ret = &ViewUiHuman{
			errorColor: "[red]",
			warnColor:  "[yellow]",
			ui:         oldUi,
			view:       view,
		}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &ViewUiMulti{ret, &ViewUiJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

// ViewUiHuman is a Ui implementation that colors its output according
// to the given color schemes for the given type of output.
type ViewUiHuman struct {
	ui          cli.Ui
	view        *View
	errorColor  string
	warnColor   string
	outputColor string
	infoColor   string
}

func (u *ViewUiHuman) Ask(query string) (string, error) {
	return u.ui.Ask(u.colorize(query, u.outputColor))
}

func (u *ViewUiHuman) AskSecret(query string) (string, error) {
	return u.ui.AskSecret(u.colorize(query, u.outputColor))
}

func (u *ViewUiHuman) Output(message string) {
	_, _ = u.view.streams.Println(u.colorize(message, u.outputColor))
}

func (u *ViewUiHuman) Info(message string) {
	_, _ = u.view.streams.Println(u.colorize(message, u.infoColor))
}

func (u *ViewUiHuman) Error(message string) {
	_, _ = u.view.streams.Eprintln(u.colorize(message, u.errorColor))
}

func (u *ViewUiHuman) Warn(message string) {
	// Warning messages are meant to go to stdout as pointed out here: https://github.com/opentofu/opentofu/commit/0c3bb316ea56aacf5108883d1a269a53744fdd43
	_, _ = u.view.streams.Println(u.colorize(message, u.warnColor))
}

func (u *ViewUiHuman) colorize(message string, color string) string {
	if color == "" {
		return message
	}

	return u.view.colorize.Color(fmt.Sprintf("%s%s[reset]", color, message))
}

// ViewUiJSON is a Ui implementation that colors its output according
// to the given color schemes for the given type of output.
type ViewUiJSON struct {
	view *JSONView
}

func (u *ViewUiJSON) Ask(_ string) (string, error) {
	return "", ErrorInputDisabled
}

func (u *ViewUiJSON) AskSecret(_ string) (string, error) {
	return "", ErrorInputDisabled
}

func (u *ViewUiJSON) Output(message string) {
	u.view.Info(stripColor(message))
}

func (u *ViewUiJSON) Info(message string) {
	u.view.Info(stripColor(message))
}

func (u *ViewUiJSON) Error(message string) {
	u.view.Error(stripColor(message))
}

func (u *ViewUiJSON) Warn(message string) {
	u.view.Warn(stripColor(message))
}

// ViewUiMulti is a Ui implementation that colors its output according
// to the given color schemes for the given type of output.
type ViewUiMulti []cli.Ui

func (u ViewUiMulti) Ask(query string) (string, error) {
	var err error

	for _, ui := range u {
		out, innerErr := ui.Ask(query)
		if innerErr == nil {
			return out, innerErr // Return first response
		}
		err = innerErr // Othwerise, store the error to be returned later in case it's needed
	}
	return "", err
}

func (u ViewUiMulti) AskSecret(query string) (string, error) {
	var err error

	for _, ui := range u {
		out, innerErr := ui.AskSecret(query)
		if innerErr == nil {
			return out, innerErr // Return first response
		}
		err = innerErr // Othwerise, store the error to be returned later in case it's needed
	}
	return "", err
}

func (u ViewUiMulti) Output(message string) {
	for _, ui := range u {
		ui.Output(message)
	}
}

func (u ViewUiMulti) Info(message string) {
	for _, ui := range u {
		ui.Info(message)
	}
}

func (u ViewUiMulti) Error(message string) {
	for _, ui := range u {
		ui.Error(message)
	}
}

func (u ViewUiMulti) Warn(message string) {
	for _, ui := range u {
		ui.Warn(message)
	}
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
