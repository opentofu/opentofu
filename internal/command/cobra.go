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
)

type metaFunc func() (Meta, error)

type Group struct {
	ID    string
	Title string
}

var (
	MainCommandGroup  = Group{ID: "main", Title: "Main commands:"}
	OtherCommandGroup = Group{ID: "other", Title: "All other commands:"}
)

type Command struct {
	Name    string
	Short   string
	Long    string
	GroupID string

	Commands []Command
	Groups   []Group

	Flags        arguments.Flags
	UsageOptions UsageOptions
	Run          func(Meta) int
}

type UsageOptions struct {
	Usage      string
	Suffix     string
	FlagGroups []arguments.FlagGroup

	SingleSpace bool
}

func CommandUsage(cmd Command, w io.Writer) {
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
	type row struct {
		name string
		info string
	}
	printTable := func(rows []row) {
		slices.SortFunc(rows, func(a, b row) int {
			return strings.Compare(a.name, b.name)
		})

		maxNameLength := 0
		for _, row := range rows {
			maxNameLength = max(maxNameLength, len(row.name))
		}

		padding := "  "
		nameSpace := maxNameLength + len(padding)*2
		infoPad := TERM_WIDTH - nameSpace
		for _, row := range rows {
			nameStr := padding + row.name
			fmt.Fprint(w, nameStr)
			fmt.Fprint(w, strings.Repeat(" ", nameSpace-len(nameStr)))

			usage := wordwrap.WrapString(row.info, uint(infoPad))
			pad := strings.Repeat(" ", nameSpace)
			usage = strings.ReplaceAll(usage, "\n", "\n"+pad) + "\n"
			fmt.Fprint(w, usage)
			if !cmd.UsageOptions.SingleSpace {
				fmt.Fprint(w, "\n")
			}
		}
	}
	printSubcmds := func(title string, cmds []Command, groupID *string) {
		var commandsToPrint []row
		for _, cmd := range cmd.Commands {
			if groupID == nil || *groupID == cmd.GroupID {
				commandsToPrint = append(commandsToPrint, row{
					name: cmd.Name,
					info: cmd.Short,
				})
			}
		}
		if len(commandsToPrint) == 0 {
			return
		}
		printHeader(title)
		printTable(commandsToPrint)
		fmt.Fprint(w, "\n")
	}
	printFlags := func(title string, group *arguments.FlagGroup) {
		var flagsToPrint []row
		for _, flag := range cmd.Flags {
			if group != nil && flag.GroupID != group.ID {
				continue
			}
			if flag.Hidden {
				continue
			}
			s := "-" + flag.Name
			if flag.Display != "" {
				s += flag.Display
			}
			flagsToPrint = append(flagsToPrint, row{
				name: s,
				info: flag.Usage,
			})
		}

		if len(flagsToPrint) == 0 {
			return
		}

		printHeader(title)
		if !cmd.UsageOptions.SingleSpace {
			fmt.Fprint(w, "\n")
		}
		if group != nil && group.Description != "" {
			printDescription(group.Description)
		}

		printTable(flagsToPrint)
	}

	// Start building

	if cmd.UsageOptions.Usage != "" {
		printHeader(fmt.Sprintf("Usage: %s\n", cmd.UsageOptions.Usage))
	} else {
		printHeader(fmt.Sprintf("Usage: tofu [global options] %s [options]\n", cmd.Name))
	}

	if cmd.Long != "" {
		printDescription(cmd.Long)
	} else if cmd.Short != "" {
		printDescription(cmd.Short)
	}

	if len(cmd.Groups) == 0 {
		printSubcmds("Subcommands:", cmd.Commands, nil)
	} else {
		for _, group := range cmd.Groups {
			printSubcmds(group.Title, cmd.Commands, &group.ID)
		}

		printSubcmds("Additional Commands:", cmd.Commands, new(""))
	}

	if len(cmd.UsageOptions.FlagGroups) == 0 {
		printFlags("Options:", nil)
	} else {
		for _, group := range cmd.UsageOptions.FlagGroups {
			printFlags(group.Title, &group)
		}
	}
	/*if cmd.HasAvailableInheritedFlags() {
		printFlags("Global Options:", cmd.InheritedFlags(), nil)
	}*/
}
