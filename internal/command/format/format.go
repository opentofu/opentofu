// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package format contains helpers for formatting various OpenTofu
// structures for human-readable output.
//
// This package is used by the official OpenTofu CLI in formatting any
// output and is exported to encourage non-official frontends to mimic the
// output formatting as much as possible so that text formats of OpenTofu
// structures have a consistent look and feel.
package format

import "github.com/opentofu/opentofu/internal/plans"

// DiffActionSymbol returns a string that, once passed through a
// colorstring.Colorize, will produce a result that can be written
// to a terminal to produce a symbol made of three printable
// characters, possibly interspersed with VT100 color codes.
func DiffActionSymbol(action plans.Action) string {
	switch action {
	case plans.DeleteThenCreate:
		return "[red]-[reset]/[green]+[reset]"
	case plans.CreateThenDelete:
		return "[green]+[reset]/[red]-[reset]"
	case plans.Create:
		return "  [green]+[reset]"
	case plans.Delete:
		return "  [red]-[reset]"
	case plans.Read:
		return " [cyan]<=[reset]"
	case plans.Update:
		return "  [yellow]~[reset]"
	case plans.NoOp:
		return "   "
	case plans.Forget:
		return "  [red].[reset]"
	default:
		return "  ?"
	}
}
