// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"regexp"

	"github.com/opentofu/opentofu/internal/command/arguments"
)

var ErrorInputDisabled = fmt.Errorf("in this view cannot ask user input")

var _ Ui = (*ViewUiHuman)(nil)
var _ Ui = (*ViewUiJSON)(nil)
var _ Ui = (*ViewUiMulti)(nil)

type Ui interface {
	Output(string)
}

func NewViewUI(args *arguments.View, view *View, oldUi Ui) Ui {
	var ret Ui
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &ViewUiJSON{
			view: NewJSONView(view, nil),
		}
	case arguments.ViewHuman:
		ret = &ViewUiHuman{
			ui:   oldUi,
			view: view,
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
	ui          Ui
	view        *View
	outputColor string
}

func (u *ViewUiHuman) Output(message string) {
	_, _ = u.view.streams.Println(u.colorize(message, u.outputColor))
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

func (u *ViewUiJSON) Output(message string) {
	u.view.Info(stripColor(message))
}

// ViewUiMulti is a Ui implementation that colors its output according
// to the given color schemes for the given type of output.
type ViewUiMulti []Ui

func (u ViewUiMulti) Output(message string) {
	for _, ui := range u {
		ui.Output(message)
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
