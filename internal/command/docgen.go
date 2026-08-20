// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type nestedCommandNames []string

func (d nestedCommandNames) spaces() string {
	return strings.Join(d, " ")
}
func (d nestedCommandNames) dashes() string {
	return strings.Join(d, "-")
}
func (d nestedCommandNames) name() string {
	return d[len(d)-1]
}
func (d nestedCommandNames) extend(name string) nestedCommandNames {
	return append(d, name)
}

type docWriter interface {
	commandInfo(names nestedCommandNames, usage, short, long string)
	writeCommandGroup(title string, commandsToPrint []Command, sort bool)
	writeFlagGroup(title string, group *arguments.FlagGroup, flags []*arguments.Flag, singleSpace bool)
}

func (c Command) docGen(writer docWriter, names nestedCommandNames) {
	usage := c.UsageOverride.Usage
	if usage == "" {
		positionalArgs := ""
		for _, arg := range c.CommandLine.Args {
			name := arg.Name
			if arg.Variadic {
				name = name + "..."
			}
			if arg.Optional {
				positionalArgs += fmt.Sprintf(" [%s]", name)
			} else {
				positionalArgs += fmt.Sprintf(" <%s>", name)
			}
		}
		usage = fmt.Sprintf("tofu [global options] %s [options]%s\n", names.spaces(), positionalArgs)
	}

	writer.commandInfo(names.extend(c.Name), usage, c.Short, c.Long)

	printSubcmds := func(title string, groupID *string, sort bool) {
		var commandsToPrint []Command
		for _, cmd := range c.Commands {
			if cmd.Hidden {
				continue
			}
			if groupID == nil || *groupID == cmd.GroupID {
				commandsToPrint = append(commandsToPrint, cmd)
			}
		}
		if len(commandsToPrint) == 0 {
			return
		}
		writer.writeCommandGroup(title, commandsToPrint, sort)
	}
	printFlags := func(title string, group *arguments.FlagGroup) {
		var flagsToPrint []*arguments.Flag
		for _, flag := range c.CommandLine.Flags {
			if group != nil && flag.GroupID != group.ID {
				continue
			}
			if flag.Hidden {
				continue
			}
			flagsToPrint = append(flagsToPrint, flag)
		}

		if len(flagsToPrint) == 0 {
			return
		}

		writer.writeFlagGroup(title, group, flagsToPrint, c.UsageOverride.SingleSpace)
	}

	if len(c.Groups) == 0 {
		printSubcmds("Subcommands:", nil, true)
	} else {
		hasDefault := false
		for _, group := range c.Groups {
			printSubcmds(group.Title, &group.ID, !group.NoSort)
			hasDefault = hasDefault || group.ID == ""
		}

		if !hasDefault {
			printSubcmds("Additional Commands:", new(""), true)
		}
	}

	if len(c.CommandLine.FlagGroups) == 0 {
		printFlags("Options:", nil)
	} else {
		for _, group := range c.CommandLine.FlagGroups {
			printFlags(group.Title, &group)
		}
	}
}

// CommandUsage writes usage/help text to the given writer.
// This function standardizes how we format usage/help text.
func CommandUsage(namespace string, cmd Command, w io.Writer) {
	cmd.docGen(&helpWriter{w}, nestedCommandNames(strings.Split(namespace, " ")))
}

func DocgenCommander() Command {
	cmd := Command{
		Name:   "docgen",
		Short:  "Generate documentation",
		Long:   `Export builtin command documentation into a directory. The only supported option for TYPE is "man" for manpages`,
		Hidden: true,
	}

	var docType string
	var outDir string
	cmd.CommandLine.PositionalArg(&docType, "TYPE", false)
	cmd.CommandLine.PositionalArg(&outDir, "DIR", false)
	arguments.BindView(&cmd.CommandLine, 0)

	cmd.Run = func(meta Meta) int {
		if docType != "man" {
			meta.View.Diagnostics(tfdiags.New(tfdiags.Sourceless(tfdiags.Error, "Invalid TYPE", fmt.Sprintf(`Expected type "man", got %q`, docType))))
			return 1
		}
		// This can be tested locally without actually installing the man pages with:
		// MANPATH=$DIR man tofu workspace-show
		pages := RootCommander(nil, nil, nil).manPages(nil)
		level := "1"
		outDir = filepath.Join(outDir, "man"+level)
		err := os.MkdirAll(outDir, 0755)
		if err != nil {
			meta.View.Diagnostics(tfdiags.New(err))
		}
		for name, page := range pages {
			outfile := filepath.Join(outDir, name) + "." + level
			fmt.Printf("Writing %s\n", outfile)
			err := os.WriteFile(outfile, []byte(page), 0644)
			if err != nil {
				meta.View.Diagnostics(tfdiags.New(err))
				return 1
			}
		}
		return 0
	}

	return cmd
}
