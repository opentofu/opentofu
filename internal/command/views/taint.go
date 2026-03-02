// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Taint interface {
	Diagnostics(diags tfdiags.Diagnostics)
	TaintedSuccessfully(addr addrs.AbsResourceInstance)
}

// NewTaint returns an initialized Taint implementation for the given ViewType.
func NewTaint(args arguments.ViewOptions, view *View) Taint {
	var ret Taint
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &TaintJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &TaintHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &TaintMulti{ret, &TaintJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type TaintMulti []Taint

var _ Taint = (TaintMulti)(nil)

func (m TaintMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m TaintMulti) TaintedSuccessfully(addr addrs.AbsResourceInstance) {
	for _, o := range m {
		o.TaintedSuccessfully(addr)
	}
}

type TaintHuman struct {
	view *View
}

var _ Taint = (*TaintHuman)(nil)

func (v *TaintHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *TaintHuman) TaintedSuccessfully(addr addrs.AbsResourceInstance) {
	_, _ = v.view.streams.Println(fmt.Sprintf("Resource instance %s has been marked as tainted.", addr))
}

type TaintJSON struct {
	view *JSONView
}

var _ Taint = (*TaintJSON)(nil)

func (v *TaintJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *TaintJSON) TaintedSuccessfully(addr addrs.AbsResourceInstance) {
	v.view.Info(fmt.Sprintf("Resource instance %s has been marked as tainted.", addr))
}
