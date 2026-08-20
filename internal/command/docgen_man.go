// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/muesli/roff"
	"github.com/opentofu/opentofu/internal/command/arguments"
)

type manWriter struct {
	*roff.Document
}

func (w *manWriter) commandInfo(names nestedCommandNames, usage, short, long string) {
	dashNs := names.dashes()
	spaceNs := names.spaces()

	w.Heading(1, dashNs, spaceNs, time.Now())

	w.Section("name")
	w.Text(fmt.Sprintf("%s - %s", spaceNs, short))

	w.Section("synopsis")
	w.Text(usage)

	if long != "" {
		w.Section("description")
		w.Text(long)
	}
}

func (w *manWriter) writeCommandGroup(title string, cmds []Command, sort bool) {
	w.Section(strings.TrimSuffix(title, ":"))
	if sort {
		slices.SortFunc(cmds, func(a, b Command) int {
			return strings.Compare(a.Name, b.Name)
		})
	}
	for _, cmd := range cmds {
		w.List(cmd.Name + " - " + cmd.Short)
	}
}

func (w *manWriter) writeFlagGroup(title string, group *arguments.FlagGroup, flags []*arguments.Flag, singleSpace bool) {
	w.Section(strings.TrimSuffix(title, ":"))
	if group != nil && group.Description != "" {
		w.Text(group.Description)
	}

	slices.SortFunc(flags, func(a, b *arguments.Flag) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, flag := range flags {
		w.TaggedParagraph(-1)
		s := "-" + flag.Name
		if flag.Display != "" {
			s += flag.Display
		}
		w.TextBold(s)
		w.EndSection()
		w.Text(flag.Usage)
	}

	if group != nil && group.Suffix != "" {
		w.Text(group.Suffix)
	}
}

func (c Command) manPages(names nestedCommandNames) map[string]string {
	if c.Name == "" {
		c.Name = "tofu"
		c.Short = "OpenTofu"
	}

	writer := &manWriter{roff.NewDocument()}
	c.docGen(writer, names)

	fullName := names.extend(c.Name)

	var seeAlso []string
	for i := range fullName {
		if i != 0 {
			seeAlso = append(seeAlso, fullName[:i].dashes())
		}
	}
	for _, cmd := range c.Commands {
		if !cmd.Hidden {
			seeAlso = append(seeAlso, fullName.extend(cmd.Name).dashes())
		}
	}
	writer.Section("see also")
	for i, also := range seeAlso {
		if i != 0 {
			writer.Text(", ")
		}
		writer.TextBold(also)
		writer.Text("(1)")
	}

	// TODO
	// * Environment Variables
	// * Exit Codes

	result := map[string]string{
		fullName.dashes(): writer.String(),
	}

	for _, cmd := range c.Commands {
		maps.Copy(result, cmd.manPages(fullName))
	}

	return result
}
