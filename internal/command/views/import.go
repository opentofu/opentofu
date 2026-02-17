// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

type Import interface {
	Diagnostics(diags tfdiags.Diagnostics)
	Hooks() []tofu.Hook
	Operation() Operation

	InvalidAddressReference()
	MissingResourceConfiguration(addr addrs.AbsResourceInstance, modulePath string, resourceType string, resourceName string)
	Success()
	UnsupportedLocalOp()
}

// NewImport returns an initialized Import implementation for the given ViewType.
func NewImport(args arguments.ViewOptions, view *View) Import {
	var ret Import
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &ImportJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &ImportHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &ImportMulti{ret, &ImportJSON{view: NewJSONView(view, args.JSONInto)}}
	}

	return ret
}

type ImportMulti []Import

var _ Import = (ImportMulti)(nil)

func (m ImportMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m ImportMulti) InvalidAddressReference() {
	for _, o := range m {
		o.InvalidAddressReference()
	}
}

func (m ImportMulti) MissingResourceConfiguration(addr addrs.AbsResourceInstance, modulePath string, resourceType string, resourceName string) {
	for _, o := range m {
		o.MissingResourceConfiguration(addr, modulePath, resourceType, resourceName)
	}
}

func (m ImportMulti) Success() {
	for _, o := range m {
		o.Success()
	}
}

func (m ImportMulti) UnsupportedLocalOp() {
	for _, o := range m {
		o.UnsupportedLocalOp()
	}
}

func (m ImportMulti) Hooks() []tofu.Hook {
	var hooks []tofu.Hook
	for _, o := range m {
		hooks = append(hooks, o.Hooks()...)
	}
	return hooks
}

func (m ImportMulti) Operation() Operation {
	var operation OperationMulti
	for _, mi := range m {
		operation = append(operation, mi.Operation())
	}
	return operation
}

// The ImportHuman implementation renders messages in a human-readable form.
type ImportHuman struct {
	view *View
}

var _ Import = (*ImportHuman)(nil)

func (v *ImportHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ImportHuman) InvalidAddressReference() {
	const msg = `For information on valid syntax, see:
https://opentofu.org/docs/cli/state/resource-addressing/`
	_, _ = v.view.streams.Println(msg)
}

func (v *ImportHuman) MissingResourceConfiguration(addr addrs.AbsResourceInstance, modulePath string, resourceType string, resourceName string) {
	// This is not a diagnostic because currently our diagnostics printer
	// doesn't support having a code example in the detail, and there's
	// a code example in this message.
	// TODO: Improve the diagnostics printer so we can use it for this
	// message.
	const tpl = `[reset][bold][red]Error:[reset][bold] resource address %q does not exist in the configuration.[reset]

Before importing this resource, please create its configuration in %s. For example:

resource %q %q {
  # (resource arguments)
}
`
	output := v.view.colorize.Color(
		fmt.Sprintf(
			tpl,
			addr, modulePath, resourceType, resourceName,
		),
	)
	_, _ = v.view.streams.Eprintln(output)
}

func (v *ImportHuman) Success() {
	const msg = `Import successful!

The resources that were imported are shown above. These resources are now in
your OpenTofu state and will henceforth be managed by OpenTofu.`

	output := v.view.colorize.Color(fmt.Sprintf("\n[reset][green]%s\n", msg))
	_, _ = v.view.streams.Println(output)
}

func (v *ImportHuman) UnsupportedLocalOp() {
	v.view.errorln(errUnsupportedLocalOp)
}

func (v *ImportHuman) Hooks() []tofu.Hook {
	return []tofu.Hook{NewUiHook(v.view)}
}

func (v *ImportHuman) Operation() Operation {
	return NewOperation(arguments.ViewHuman, v.view.runningInAutomation, v.view)
}

type ImportJSON struct {
	view *JSONView
}

var _ Import = (*ImportJSON)(nil)

func (v *ImportJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ImportJSON) InvalidAddressReference() {
	const msg = `For information on valid syntax, see: https://opentofu.org/docs/cli/state/resource-addressing/`
	v.view.Info(msg)
}

func (v *ImportJSON) MissingResourceConfiguration(addr addrs.AbsResourceInstance, modulePath string, _ string, _ string) {
	msg := fmt.Sprintf("Resource address %q does not exist in the configuration. Before importing this resource, please create its configuration in %s", addr, modulePath)
	v.view.Error(msg)
}

func (v *ImportJSON) Success() {
	msg := "Import successful! The resources that were imported are shown above. These resources are now in your OpenTofu state and will henceforth be managed by OpenTofu"
	v.view.Info(msg)
}

func (v *ImportJSON) UnsupportedLocalOp() {
	v.view.Error(
		strings.TrimSpace(
			strings.ReplaceAll(
				strings.ReplaceAll(
					errUnsupportedLocalOp,
					"\n", " "),
				"  ", " "),
		),
	)
}

func (v *ImportJSON) Hooks() []tofu.Hook {
	return []tofu.Hook{newJSONHook(v.view)}
}

func (v *ImportJSON) Operation() Operation {
	return &OperationJSON{view: v.view}
}
