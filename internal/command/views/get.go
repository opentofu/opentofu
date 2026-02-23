// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Get interface {
	Diagnostics(diags tfdiags.Diagnostics)
	Hooks(showLocalDir bool) initwd.ModuleInstallHooks
}

// NewGet returns an initialized Get implementation for the given ViewType.
func NewGet(args arguments.ViewOptions, view *View) Get {
	var ret Get
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &GetJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &GetHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &GetMulti{ret, &GetJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type GetMulti []Get

var _ Get = (GetMulti)(nil)

func (m GetMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m GetMulti) Hooks(showLocalPath bool) initwd.ModuleInstallHooks {
	hooks := make([]initwd.ModuleInstallHooks, len(m))
	for i, o := range m {
		hooks[i] = o.Hooks(showLocalPath)
	}
	return moduleInstallationHookMulti(hooks)
}

type GetHuman struct {
	view *View
}

var _ Get = (*GetHuman)(nil)

func (v *GetHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *GetHuman) Hooks(showLocalPath bool) initwd.ModuleInstallHooks {
	return &moduleInstallationHookHuman{
		v:              v.view,
		showLocalPaths: showLocalPath,
	}
}

type GetJSON struct {
	view *JSONView
}

var _ Get = (*GetJSON)(nil)

func (v *GetJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *GetJSON) Hooks(showLocalPath bool) initwd.ModuleInstallHooks {
	return &moduleInstallationHookJSON{
		v:              v.view,
		showLocalPaths: showLocalPath,
	}
}
