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
	HelpPrompt()

	UnsupportedLocalOp()
	Output(result string)
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

func (m ConsoleMulti) HelpPrompt() {
	for _, o := range m {
		o.HelpPrompt()
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

type ConsoleHuman struct {
	view *View
}

var _ Console = (*ConsoleHuman)(nil)

func (v *ConsoleHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ConsoleHuman) HelpPrompt() {
	helpText := `
Usage: tofu [global options] console [options]

  Starts an interactive console for experimenting with OpenTofu
  interpolations.

  This will open an interactive console that you can use to type
  interpolations into and inspect their values. This command loads the
  current state. This lets you explore and test interpolations before
  using them in future configurations.

  This command will never modify your state.

Options:

  -compact-warnings      If OpenTofu produces any warnings that are not
                         accompanied by errors, show them in a more compact
                         form that includes only the summary messages.

  -consolidate-warnings  If OpenTofu produces any warnings, no consolidation
                         will be performed. All locations, for all warnings
                         will be listed. Enabled by default.

  -consolidate-errors    If OpenTofu produces any errors, no consolidation
                         will be performed. All locations, for all errors
                         will be listed. Disabled by default

  -state=path            Legacy option for the local backend only. See the local
                         backend's documentation for more information.

  -var 'foo=bar'         Set a variable in the OpenTofu configuration. This
                         flag can be set multiple times.

  -var-file=foo          Set variables in the OpenTofu configuration from
                         a file. If "terraform.tfvars" or any ".auto.tfvars"
                         files are present, they will be automatically loaded.

  -json-into=out.json    All the evaluation results returned back to the user
                         are streamed in json format in the given file.

`
	v.view.errorln(helpText)
}

func (v *ConsoleHuman) UnsupportedLocalOp() {
	v.view.errorln(errUnsupportedLocalOp)
}

func (v *ConsoleHuman) Output(result string) {
	_, _ = v.view.streams.Println(result)
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

func (v *ConsoleJSON) HelpPrompt() {}

func (v *ConsoleJSON) UnsupportedLocalOp() {
	v.view.Error("The configured backend doesn't support this operation. The 'backend' in OpenTofu defines how OpenTofu operates. The default backend performs all operations locally on your machine. Your configuration is configured to use a non-local backend. This backend doesn't support this operation")
}

func (v *ConsoleJSON) Output(result string) {
	v.view.Info(result)
}
