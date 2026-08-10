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

// CommandLine represents the information need to represent the cli options
// available to a given OpenTofu command. It is used for both argument
// handling and help text construction.
type CommandLine struct {
	// Flags represents all flags available to the command line.
	Flags map[string]*Flag
	// FlagGroups defines the group headers for the Flags. These are
	// only used in a few specific commands for now and are typically
	// left empty.
	FlagGroups []FlagGroup

	// Args contains the positional arguments defined for a given command.
	// These are only allowed after all of the flag parsing is complete.
	Args []Argument
	// ArgsHelp is a field used to customize the argument error help text.
	// This is a legacy option and should not be used for any new commands.
	ArgHelp string

	// Hooks defines the pre/post command logic. Hooks.Pre should be run
	// between os.Arg handling and the actual command logic. Hooks.Post
	// should be run after the command has completed and before os.Exit is
	// called.
	Hooks Hooks
}

// PositionalArgs processes the input as a set of positional arguments. This
// should be the entries remaining after Flag parsing.
func (c CommandLine) PositionalArgs(remaining []string) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

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

	return diags
}

// Attach iterates through all of the flags and adds them
// to the given flagset.
func (c CommandLine) Attach(flags *pflag.FlagSet) {
	for _, flag := range c.Flags {
		flag.Cobra(flags)
	}
}

// StdlibArgs uses the old command line stdargs processing method. This currently exists
// as a fallback and is primarily used for testing. Once we have completely switched to a
// new CLI library, the tests can be updated and this function removed.
func (c CommandLine) StdlibArgs(args []string) tfdiags.Diagnostics {
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

	cmdFlags := defaultFlagSet("")
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

	diags = diags.Append(c.PositionalArgs(remaining))

	return diags
}

// Stdlib is a wrapper around StdlibArgs that handles hooks. This is used by the
// legacy Parse methods and should be removed when they are retired.
func (c CommandLine) Stdlib(name string, args []string) (func(), tfdiags.Diagnostics) {
	diags := c.StdlibArgs(args)

	// Process hooks
	return func() { c.Hooks.Post() }, diags.Append(c.Hooks.Pre())
}

// Hook is a helper function to add a hook to the command line processing.
func (c *CommandLine) Hook(h Hook) {
	c.Hooks = append(c.Hooks, h)
}

// Argument represents a positional argument.
type Argument struct {
	Name     string
	Optional bool
	Variadic bool
	// Process takes the current remaining os.Args entries and returns
	// the remainder after the given argument has been processed.
	Process func([]string) ([]string, error)
}

// PositionalArg registers a positional argument.
func (c *CommandLine) PositionalArg(p *string, name string, optional bool) {
	for _, arg := range c.Args {
		if arg.Variadic {
			// In practice, this will be caught in testing as it's run before we even get to executing any commands
			panic("BUG: Can not register a positional argument after a variadic argument!")
		}
	}
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

// VariadicArg registers a variadic argument.
func (c *CommandLine) VariadicArg(p *[]string, name string) {
	for _, arg := range c.Args {
		if arg.Variadic {
			// In practice, this will be caught in testing as it's run before we even get to executing any commands
			panic("BUG: Can not register multiple variadic arguments!")
		}
	}
	c.Args = append(c.Args, Argument{Name: name, Variadic: true, Process: func(args []string) ([]string, error) {
		*p = args
		return nil, nil
	}})
}

// Flag registers the given flag to the CommandLine.
func (c *CommandLine) Flag(flag *Flag) *Flag {
	if c.Flags == nil {
		c.Flags = map[string]*Flag{}
	}
	c.Flags[flag.Name] = flag
	return flag
}

// BoolVar attaches a bool flag to the CommandLine.
func (c *CommandLine) BoolVar(p *bool, name string, value bool, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.BoolVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.BoolVar(p, name, value, usage) },
	})
}

// IntVar attaches a int flag to the CommandLine.
func (c *CommandLine) IntVar(p *int, name string, value int, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.IntVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.IntVar(p, name, value, usage) },
	})
}

// StringVar attaches a string flag to the CommandLine.
func (c *CommandLine) StringVar(p *string, name string, value string, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.StringVar(p, name, value, usage) },
	})
}

// DurationVar attaches a time.Duration flag to the CommandLine.
func (c *CommandLine) DurationVar(p *time.Duration, name string, value time.Duration, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.DurationVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.DurationVar(p, name, value, usage) },
	})
}

// StringArrayVar attaches a StringArray flag to the CommandLine.
// This treats every instance of -name is a new entry and does not perform any ',' splitting.
func (c *CommandLine) StringArrayVar(p *[]string, name string, value []string, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.StringArrayVar(p, name, value, usage) },
		Stdlib: func(f *flag.FlagSet) { f.Var((*flags.FlagStringSlice)(p), name, usage) },
	})
}

// RawFlags attaches a flags.RawFlags to the CommandLine.
// This is a deprecated function and should be removed once var and var-file
// processing is improved. This will correspond with the removal of the flags package.
func (c *CommandLine) RawFlags(p flags.RawFlags, name string, usage string) *Flag {
	return c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Cobra:  func(f *pflag.FlagSet) { f.Func(name, usage, func(s string) error { return p.Set(s) }) },
		Stdlib: func(f *flag.FlagSet) { f.Var(p, name, usage) },
	})
}

// Flag is our representation of a command line flag and the implementation
// details of interfacing it with a given command line library.
type Flag struct {
	// Name is the name of the flag.
	Name string
	// Usage is the usage text that will be formatted.  It may include
	// newlines, but it is discouraged.
	Usage string
	// GroupID is the group that this flag is nested under. This is
	// for formatting in the help/usage text.
	GroupID string
	// Display is a suffix for the Name during help text formatting. For example:
	// Display: "=value", would render as
	//   --flag-name=value
	Display string
	// Hidden determines if this flag be visible in help text.
	Hidden bool
	// Global determines if this flag is allowed to intermix with positional
	// arguments. This is a view holdover and should be considered for removal
	// at some future date.
	Global bool

	// Cobra implementation of this flag
	Cobra func(*pflag.FlagSet)
	// Stdlib implementation of this flag. Will be removed once the new
	// CLI adoption is complete.
	Stdlib func(*flag.FlagSet)

	// IsSet is a workaround for -backend and -cloud. TODO consider
	// proper argument aliases as supported by the cli implementation.
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

// FlagGroup is the information needed to define a flag
// grouping for help text display.
type FlagGroup struct {
	// ID corresponds to flag's GroupID
	ID string
	// Title of the group
	Title string
	// Description present below the title
	Description string
	// Suffix text for after the group flags have
	// been rendered
	Suffix string
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
