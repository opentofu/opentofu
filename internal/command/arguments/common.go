// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"
	"time"

	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/spf13/pflag"
)

type Flags map[string]*Flag

func (f Flags) Stdlib(stdlib *flag.FlagSet) {
	for _, flag := range f {
		flag.Stdlib(stdlib)
	}
}

func (f Flags) BoolVar(p *bool, name string, value bool, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.BoolVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.BoolVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}
func (f Flags) IntVar(p *int, name string, value int, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.IntVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.IntVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}
func (f Flags) StringVar(p *string, name string, value string, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.StringVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}
func (f Flags) DurationVar(p *time.Duration, name string, value time.Duration, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.DurationVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.DurationVar(p, name, value, usage) },
	}
	f[name] = flag
	return flag
}

func (f Flags) StringArrayVar(p *[]string, name string, value []string, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringArrayVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.Var((*flags.FlagStringSlice)(p), name, usage) },
	}
	f[name] = flag
	return flag
}

func (f Flags) RawFlags(p flags.RawFlags, name string, usage string) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.Func(name, usage, func(s string) error { return p.Set(s) }) },
		Stdlib: func(f *flag.FlagSet) { f.Var(p, name, usage) },
	}
	f[name] = flag
	return flag
}

func (f Flags) Func(name string, usage string, fn func(string) error) *Flag {
	flag := &Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.Func(name, usage, fn) },
		Stdlib: func(f *flag.FlagSet) { panic("not implemented") },
	}
	f[name] = flag
	return flag
}

type Flag struct {
	Name    string
	Usage   string
	GroupID string
	Display string
	Hidden  bool

	Cobra  func(*pflag.FlagSet)
	Stdlib func(*flag.FlagSet)
}

func (f *Flag) SetGroup(id string) *Flag {
	f.GroupID = id
	return f
}
func (f *Flag) SetDisplay(display string) *Flag {
	f.Display = display
	return f
}
func (f *Flag) SetHidden(hidden bool) *Flag {
	f.Hidden = hidden
	return f
}

type FlagGroup struct {
	ID          string
	Title       string
	Description string
}

type Hook struct {
	Pre, Post func() tfdiags.Diagnostics
}
type Hooks []Hook

func (h Hooks) Pre() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, hook := range h {
		if hook.Pre != nil {
			diags = diags.Append(hook.Pre())
		}
	}
	return diags
}

func (h Hooks) Post() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, hook := range h {
		if hook.Post != nil {
			diags = diags.Append(hook.Post())
		}
	}
	return diags
}
