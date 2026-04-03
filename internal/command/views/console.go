// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Console interface {
	Diagnostics(diags tfdiags.Diagnostics)

	UnsupportedLocalOp()
	Output(result string)

	// Backend returns the non-command view that contains methods to provide
	// progress output for the backend operations.
	Backend() Backend
}

// NewConsole returns an initialized Console implementation for the given ViewType.
func NewConsole(args arguments.ViewOptions, view *View) Console {
	var ret Console
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &ConsoleJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &ConsoleHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &ConsoleMulti{ret, &ConsoleJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type ConsoleMulti []Console

var _ Console = (ConsoleMulti)(nil)

func (m ConsoleMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m ConsoleMulti) UnsupportedLocalOp() {
	for _, o := range m {
		o.UnsupportedLocalOp()
	}
}

func (m ConsoleMulti) Output(result string) {
	for _, o := range m {
		o.Output(result)
	}
}

func (m ConsoleMulti) Backend() Backend {
	ret := make([]Backend, len(m))
	for i, v := range m {
		ret[i] = v.Backend()
	}
	return BackendMulti(ret)
}

type ConsoleHuman struct {
	view *View
}

var _ Console = (*ConsoleHuman)(nil)

func (v *ConsoleHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ConsoleHuman) UnsupportedLocalOp() {
	v.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *ConsoleHuman) Output(result string) {
	_, _ = v.view.streams.Println(result)
}

func (v *ConsoleHuman) Backend() Backend {
	return &BackendHuman{
		view: v.view,
	}
}

// ConsoleJSON is meant to be used only for the `-json-into` situation.
// The `console` command with `-json` does not really make sense so this is not allowed.
type ConsoleJSON struct {
	view *JSONView
}

var _ Console = (*ConsoleJSON)(nil)

func (v *ConsoleJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ConsoleJSON) UnsupportedLocalOp() {
	v.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *ConsoleJSON) Output(result string) {
	v.view.Info(result)
}

func (v *ConsoleJSON) Backend() Backend {
	return &BackendJSON{
		view: v.view,
	}
}
