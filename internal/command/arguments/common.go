// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/urfave/cli/v3"
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

	// PreHooks contain the logic that should be run between os.Args
	// handling and the actual command logic.
	PreHooks Hooks
	// PostHooks contain the logic that should be run after the command
	// has completed and before os.Exit is called.
	// This is typically only used for the --json-into special case.
	PostHooks Hooks

	// This is a bit of a hack so we can correctly report diagnostics before actually executing the command
	View *View

	// These are hacks for the meta struct that should be removed as the meta struct is broken up and removed
	Backend   *Backend
	Operation *Operation
	State     *State
	Vars      *Vars
}

// PositionalError is the common error handling of remaining positional arguments
func (c *CommandLine) PositionalError(remaining []string, argsErrored bool) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	if len(remaining) == 0 && !argsErrored {
		return nil
	}

	nRequired := 0
	nOptional := 0
	for _, arg := range c.Args {
		if arg.Optional {
			nOptional += 1
		} else {
			nRequired += 1
		}
	}

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

	return diags
}

// PositionalArgs processes the input as a set of positional arguments. This
// should be the entries remaining after Flag parsing.
func (c *CommandLine) PositionalArgs(remaining []string) tfdiags.Diagnostics {
	argsErrored := false
	for _, arg := range c.Args {
		var err error
		remaining, err = arg.Process(remaining)
		if err != nil {
			// For now, we don't care about the more specific error text
			argsErrored = true
		}
	}
	return c.PositionalError(remaining, argsErrored)
}

// RemainCheck handles positional arguments and produces a corresponding
// error if nessesary.
func (c *CommandLine) RemainCheck(remaining []string) tfdiags.Diagnostics {
	argsErrored := false
	for _, arg := range c.Args {
		if arg.Optional {
			continue
		}
		if len(arg.Cli.Get().(string)) == 0 {
			argsErrored = true
			break
		}
	}
	return c.PositionalError(remaining, argsErrored)
}

// CliFlags builds the set of flags to be attached
// to a command
func (c *CommandLine) CliFlags() []cli.Flag {
	var ret []cli.Flag

	for _, flag := range c.Flags {
		fc := flag.Cli()
		flag.IsSet = fc.IsSet
		ret = append(ret, fc)
	}

	return ret
}

// CliArguments builds the set of positional arguments to
// be attached to a command
func (c *CommandLine) CliArguments() []cli.Argument {
	var ret []cli.Argument

	for _, arg := range c.Args {
		ret = append(ret, arg.Cli)
	}

	return ret
}

// ParseLegacy uses the old command line stdargs processing method. This currently exists
// as a fallback and is primarily used for testing. Once we have completely switched to a
// new CLI library, the tests can be updated and this function removed.
func (c *CommandLine) ParseLegacy(args []string) tfdiags.Diagnostics {
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
		flag.IsSet = func() bool { return flags.FlagIsSet(cmdFlags, flag.Name) }
	}

	remaining := cmdFlags.Args()

	diags = diags.Append(c.PositionalArgs(remaining))

	return diags
}

// parseWithHooks is a wrapper around Urfave that handles hooks. This is used by the
// legacy Parse methods and should be removed when they are retired (only used in test paths).
func (c *CommandLine) parseWithHooks(name string, args []string) (func(), tfdiags.Diagnostics) {
	//diags := c.StdlibArgs(args)
	diags := c.ParseDirect(context.Background(), args)

	// Process hooks
	return func() { c.PostHooks.Run() }, diags.Append(c.PreHooks.Run())
}

// ParseDirect is only used for testing when we need to simuate processing the args.
// This is primarilly used in the command package.
func (c *CommandLine) ParseDirect(ctx context.Context, args []string) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	cc := cli.Command{
		Name:      "internal-testing",
		Flags:     c.CliFlags(),
		Arguments: c.CliArguments(),
		Writer:    io.Discard,
		ErrWriter: io.Discard,
		CommandNotFound: func(context.Context, *cli.Command, string) {
			//diags = diags.Append(c.PositionalError(nil, true))
		},
		OnUsageError: func(ctx context.Context, cmd *cli.Command, err error, isSubcommand bool) error {
			return err
		},
	}

	err := cc.Run(ctx, append([]string{cc.Name}, args...))

	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line options",
			err.Error(),
		))
	}

	diags = diags.Append(c.RemainCheck(cc.Args().Slice()))

	return diags
}

// PreHook is a helper function to add a hook to the command line processing.
func (c *CommandLine) PreHook(h func() tfdiags.Diagnostics) {
	c.PreHooks = append(c.PreHooks, h)
}

// PostHook is a helper function to add a hook to the command line processing.
func (c *CommandLine) PostHook(h func() tfdiags.Diagnostics) {
	c.PostHooks = append(c.PostHooks, h)
}

// Argument represents a positional argument.
type Argument struct {
	Name     string
	Optional bool
	Variadic bool
	// Process takes the current remaining os.Args entries and returns
	// the remainder after the given argument has been processed.
	Process func([]string) ([]string, error)
	Cli     cli.Argument
}

// PositionalArg registers a positional argument.
func (c *CommandLine) PositionalArg(p *string, name string, optional bool) {
	for _, arg := range c.Args {
		if arg.Variadic {
			// In practice, this will be caught in testing as it's run before we even get to executing any commands
			panic("BUG: Can not register a positional argument after a variadic argument!")
		}
	}
	c.Args = append(c.Args, Argument{Name: name, Optional: optional, Cli: &cli.StringArg{Name: name, Value: *p, Destination: p}, Process: func(args []string) ([]string, error) {
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
	c.Args = append(c.Args, Argument{Name: name, Variadic: true, Optional: true, Cli: &cli.StringArgs{Name: name, Destination: p, Max: -1}, Process: func(args []string) ([]string, error) {
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
	f := c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Stdlib: func(f *flag.FlagSet) { f.BoolVar(p, name, value, usage) },
	})
	f.Cli = func() cli.Flag {
		return &cli.BoolFlag{Name: f.Name, Category: f.GroupID, DefaultText: f.Display, Local: true, Usage: f.Usage, Hidden: f.Hidden, Value: value, Destination: p}
	}
	return f
}

// IntVar attaches a int flag to the CommandLine.
func (c *CommandLine) IntVar(p *int, name string, value int, usage string) *Flag {
	f := c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Stdlib: func(f *flag.FlagSet) { f.IntVar(p, name, value, usage) },
	})
	f.Cli = func() cli.Flag {
		return &cli.IntFlag{Name: f.Name, Category: f.GroupID, DefaultText: f.Display, Local: true, Usage: f.Usage, Hidden: f.Hidden, Value: value, Destination: p}
	}
	return f
}

// StringVar attaches a string flag to the CommandLine.
func (c *CommandLine) StringVar(p *string, name string, value string, usage string) *Flag {
	f := c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Stdlib: func(f *flag.FlagSet) { f.StringVar(p, name, value, usage) },
	})
	f.Cli = func() cli.Flag {
		return &cli.StringFlag{Name: f.Name, Category: f.GroupID, DefaultText: f.Display, Local: true, Usage: f.Usage, Hidden: f.Hidden, Value: value, Destination: p}
	}
	return f
}

// DurationVar attaches a time.Duration flag to the CommandLine.
func (c *CommandLine) DurationVar(p *time.Duration, name string, value time.Duration, usage string) *Flag {
	f := c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Stdlib: func(f *flag.FlagSet) { f.DurationVar(p, name, value, usage) },
	})
	f.Cli = func() cli.Flag {
		return &cli.DurationFlag{Name: f.Name, Category: f.GroupID, DefaultText: f.Display, Local: true, Usage: f.Usage, Hidden: f.Hidden, Value: value, Destination: p}
	}
	return f
}

// StringArrayVar attaches a StringArray flag to the CommandLine.
// This treats every instance of -name is a new entry and does not perform any ',' splitting.
func (c *CommandLine) StringArrayVar(p *[]string, name string, value []string, usage string) *Flag {
	f := c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Stdlib: func(f *flag.FlagSet) { f.Var((*flags.FlagStringSlice)(p), name, usage) },
	})
	f.Cli = func() cli.Flag {
		return &cli.StringSliceFlag{Name: f.Name, Category: f.GroupID, DefaultText: f.Display, Local: true, Usage: f.Usage, Hidden: f.Hidden, Value: value, Destination: p}
	}
	return f
}

// RawFlags attaches a flags.RawFlags to the CommandLine.
// This is a deprecated function and should be removed once var and var-file
// processing is improved. This will correspond with the removal of the flags package.
func (c *CommandLine) RawFlags(p flags.RawFlags, name string, usage string) *Flag {
	f := c.Flag(&Flag{
		Name:   name,
		Usage:  usage,
		Stdlib: func(f *flag.FlagSet) { f.Var(p, name, usage) },
	})
	f.Cli = func() cli.Flag {
		dest := cli.Value(p)
		return &cli.GenericFlag{Name: f.Name, Category: f.GroupID, DefaultText: f.Display, Local: true, Usage: f.Usage, Hidden: f.Hidden, Destination: &dest, Value: dest}
	}
	return f
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

	// Stdlib implementation of this flag. Will be removed once the new
	// CLI adoption is complete.
	Stdlib func(*flag.FlagSet)
	// Cli implementation of this flag (urfave/cli)
	Cli func() cli.Flag

	// IsSet is a workaround for -backend and -cloud. TODO consider
	// proper argument aliases as supported by the cli implementation.
	IsSet func() bool
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

// Hook is a function that will be executed
// pre or post command execution
type Hook func() tfdiags.Diagnostics

// Hooks is a list of hooks with a helper method
type Hooks []Hook

// Run executes all hooks sequentially and
// returns any diagnostics encountered.
func (h Hooks) Run() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, hook := range h {
		diags = diags.Append(hook())
	}
	return diags
}
