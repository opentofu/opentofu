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

type Untaint interface {
	Diagnostics(diags tfdiags.Diagnostics)
	UntaintedSuccessfully(addr addrs.AbsResourceInstance)
}

// NewUntaint returns an initialized Untaint implementation for the given ViewType.
func NewUntaint(args arguments.ViewOptions, view *View) Untaint {
	var ret Untaint
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &UntaintJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &UntaintHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &UntaintMulti{ret, &UntaintJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type UntaintMulti []Untaint

var _ Untaint = (UntaintMulti)(nil)

func (m UntaintMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m UntaintMulti) UntaintedSuccessfully(addr addrs.AbsResourceInstance) {
	for _, o := range m {
		o.UntaintedSuccessfully(addr)
	}
}

type UntaintHuman struct {
	view *View
}

var _ Untaint = (*UntaintHuman)(nil)

func (v *UntaintHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *UntaintHuman) UntaintedSuccessfully(addr addrs.AbsResourceInstance) {
	_, _ = v.view.streams.Println(fmt.Sprintf("Resource instance %s has been successfully untainted.", addr))
}

type UntaintJSON struct {
	view *JSONView
}

var _ Untaint = (*UntaintJSON)(nil)

func (v *UntaintJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *UntaintJSON) UntaintedSuccessfully(addr addrs.AbsResourceInstance) {
	v.view.Info(fmt.Sprintf("Resource instance %s has been successfully untainted.", addr))
}
