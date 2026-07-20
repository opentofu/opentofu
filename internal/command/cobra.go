// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/spf13/cobra"
)

type metaFunc func() (Meta, error)

var (
	MainCommandGroup  = &cobra.Group{ID: "main", Title: "Main commands:"}
	OtherCommandGroup = &cobra.Group{ID: "other", Title: "All other commands:"}
)

type ExitCodeError int

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("%#v", e)
}

type UsageOptions struct {
	Usage      string
	Suffix     string
	FlagGroups []arguments.FlagGroup

	FlagSingleSpace bool
}

func CommandUsage(cmd *cobra.Command, flags arguments.Flags, opts UsageOptions, w io.Writer) error {
	const TERM_WIDTH = 80

	// Helpers
	printHeader := func(s string) {
		fmt.Fprintf(w, "%s\n", s)
	}
	printDescription := func(s string) {
		pad := "  "
		s = wordwrap.WrapString(s, uint(TERM_WIDTH-len(pad)))
		s = pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		fmt.Fprintf(w, "%s\n\n", s)
	}
	printSubcmds := func(title string, cmds []*cobra.Command, groupID *string) {
		printHeader(title)
		for _, cmd := range cmd.Commands() {
			if groupID == nil || *groupID == cmd.GroupID {
				name := fmt.Sprintf(fmt.Sprintf("%%-%ds", cmd.NamePadding()+1), cmd.Name())
				fmt.Fprintf(w, "  %s%s\n", name, cmd.Short)
			}
		}
		fmt.Fprint(w, "\n")
	}
	printFlags := func(title string, group *arguments.FlagGroup) {
		// This whole thing is not terribly efficient, but it runs with no particular urgency
		formatFlag := func(flag *arguments.Flag) string {
			s := "-" + flag.Name
			if flag.Display != "" {
				s += flag.Display
			}
			return s
		}

		var flagsToPrint []*arguments.Flag
		maxFlagLength := 0
		for _, flag := range flags {
			if group != nil && flag.GroupID != group.ID {
				continue
			}
			if flag.Hidden {
				continue
			}
			flagsToPrint = append(flagsToPrint, flag)
			maxFlagLength = max(maxFlagLength, len(formatFlag(flag)))
		}

		if len(flagsToPrint) == 0 {
			return
		}

		slices.SortFunc(flagsToPrint, func(a, b *arguments.Flag) int {
			return strings.Compare(a.Name, b.Name)
		})

		printHeader(title)
		if !opts.FlagSingleSpace {
			fmt.Fprint(w, "\n")
		}
		if group != nil && group.Description != "" {
			printDescription(group.Description)
		}

		padding := "  "
		flagSpace := maxFlagLength + len(padding)*2
		descPad := TERM_WIDTH - flagSpace
		for _, flag := range flagsToPrint {
			flagStr := padding + formatFlag(flag)
			fmt.Fprint(w, flagStr)
			fmt.Fprint(w, strings.Repeat(" ", flagSpace-len(flagStr)))

			usage := wordwrap.WrapString(flag.Usage, uint(descPad))
			pad := strings.Repeat(" ", flagSpace)
			usage = strings.ReplaceAll(usage, "\n", "\n"+pad) + "\n"
			fmt.Fprint(w, usage)
			if !opts.FlagSingleSpace {
				fmt.Fprint(w, "\n")
			}
		}
	}

	// Start building

	if opts.Usage != "" {
		printHeader(fmt.Sprintf("Usage: %s\n", opts.Usage))
	} else {
		printHeader(fmt.Sprintf("Usage: tofu [global options] %s [options]\n", cmd.Use))
	}

	if cmd.Long != "" {
		printDescription(cmd.Long)
	} else if cmd.Short != "" {
		printDescription(cmd.Short)
	}

	if cmd.HasAvailableSubCommands() {
		if len(cmd.Groups()) == 0 {
			printSubcmds("Subcommands:", cmd.Commands(), nil)
		} else {
			for _, group := range cmd.Groups() {
				printSubcmds(group.Title, cmd.Commands(), &group.ID)
			}

			if !cmd.AllChildCommandsHaveGroup() {
				printSubcmds("Additional Commands:", cmd.Commands(), new(""))
			}
		}
	}

	if cmd.HasAvailableLocalFlags() {
		if len(opts.FlagGroups) == 0 {
			printFlags("Options:", nil)
		} else {
			for _, group := range opts.FlagGroups {
				printFlags(group.Title, &group)
			}
		}
	}
	/*if cmd.HasAvailableInheritedFlags() {
		printFlags("Global Options:", cmd.InheritedFlags(), nil)
	}*/

	return nil
}
