// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/spf13/pflag"
)

type CommandLine struct {
	Flags   map[string]*Flag
	Args    []Argument
	ArgHelp string
	Hooks   Hooks
}

func (c CommandLine) Stdlib(name string, args []string) (func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Special re-ordering of arguments for "global" options
	var globalFlags []string
	var rest []string
	for _, arg := range args {
		isGlobal := false
		for _, flag := range c.Flags {
			if flag.Global {
				if strings.HasPrefix(arg, "-"+flag.Name) || strings.HasPrefix(arg, "--"+flag.Name) {
					isGlobal = true
				}
			}
		}
		if isGlobal {
			globalFlags = append(globalFlags, arg)
		} else {
			rest = append(rest, arg)
		}
	}
	if len(globalFlags) > 0 {
		args = append(globalFlags, rest...)
	}

	cmdFlags := defaultFlagSet(name)
	for _, flag := range c.Flags {
		flag.Stdlib(cmdFlags)
	}
	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line options",
			err.Error(),
		))
	}

	// Record flag set state (init hack)
	for _, flag := range c.Flags {
		flag.IsSet = flags.FlagIsSet(cmdFlags, flag.Name)
	}

	remaining := cmdFlags.Args()
	argsErrored := false
	nRequired := 0
	nOptional := 0
	for _, arg := range c.Args {
		if arg.Optional {
			nOptional += 1
		} else {
			nRequired += 1
		}

		var err error
		remaining, err = arg.Process(remaining)
		if err != nil {
			// For now, we don't care about the more specific error text
			argsErrored = true
		}
	}
	if len(remaining) > 0 || argsErrored {
		summary := "Unexpected argument"
		if argsErrored {
			summary = "Invalid arguments list"
		}

		detail := c.ArgHelp
		if detail == "" {
			numText := func(i int) string {
				switch i {
				case 0:
					return "two positional arguments."
				case 1:
					return "one positional argument."
				case 2:
					return "two positional arguments."
				case 3:
					return "three positional arguments."
				default:
					return fmt.Sprintf("%v positional arguments.", len(c.Args))
				}
			}
			if nRequired == 0 {
				if nOptional == 0 {
					detail = "Did you mean to use -chdir?"
				} else {
					detail = "Expected at most " + numText(nOptional)
				}
			} else {
				detail = "Expected exactly " + numText(nRequired)
			}
			detail = "Too many command line arguments. " + detail
		}
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			summary,
			detail,
		))
	}

	// Process hooks
	return func() { c.Hooks.Post() }, diags.Append(c.Hooks.Pre())
}

func (c *CommandLine) Hook(h Hook) {
	c.Hooks = append(c.Hooks, h)
}

type Argument struct {
	Name     string
	Optional bool
	Process  func([]string) ([]string, error)
}

func (c *CommandLine) PositionalArg(p *string, name string, optional bool) {
	c.Args = append(c.Args, Argument{Name: name, Optional: optional, Process: func(args []string) ([]string, error) {
		if len(args) == 0 {
			if !optional {
				return args, fmt.Errorf("Missing positional argument %s", name)
			}
			return args, nil
		}
		*p = args[0]
		return args[1:], nil
	}})
}
func (c *CommandLine) VariadicArg(p *[]string, name string) {
	c.Args = append(c.Args, Argument{Name: name, Process: func(args []string) ([]string, error) {
		*p = args
		return nil, nil
	}})
}

func (c *CommandLine) Flag(flag *Flag) *Flag {
	if c.Flags == nil {
		c.Flags = map[string]*Flag{}
	}
	c.Flags[flag.Name] = flag
	return flag
}

func (c *CommandLine) BoolVar(p *bool, name string, value bool, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.BoolVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.BoolVar(p, name, value, usage) },
	})
}
func (c *CommandLine) IntVar(p *int, name string, value int, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.IntVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.IntVar(p, name, value, usage) },
	})
}
func (c *CommandLine) StringVar(p *string, name string, value string, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.StringVar(p, name, value, usage) },
	})
}
func (c *CommandLine) DurationVar(p *time.Duration, name string, value time.Duration, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.DurationVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.DurationVar(p, name, value, usage) },
	})
}

func (c *CommandLine) StringArrayVar(p *[]string, name string, value []string, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringArrayVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.Var((*flags.FlagStringSlice)(p), name, usage) },
	})
}

func (c *CommandLine) RawFlags(p flags.RawFlags, name string, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.Func(name, usage, func(s string) error { return p.Set(s) }) },
		Stdlib: func(f *flag.FlagSet) { f.Var(p, name, usage) },
	})
}

type Flag struct {
	Name    string
	Usage   string
	GroupID string
	Display string
	Hidden  bool
	Global  bool

	Cobra  func(*pflag.FlagSet)
	Stdlib func(*flag.FlagSet)

	// Hack for -backend and -cloud
	IsSet bool
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
func (f *Flag) SetGlobal(mixed bool) *Flag {
	f.Global = mixed
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
