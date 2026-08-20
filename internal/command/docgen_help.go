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

const TERM_WIDTH = 80

type helpWriter struct {
	io.Writer
}

func (w *helpWriter) printHeader(s string) {
	fmt.Fprintf(w, "%s\n", s)
}
func (w helpWriter) printDescription(s string) {
	pad := "  "
	s = wordwrap.WrapString(s, uint(TERM_WIDTH-len(pad)))
	s = pad + strings.ReplaceAll(s, "\n", "\n"+pad)
	fmt.Fprintf(w, "%s\n\n", s)
}

type helpTableRow struct {
	name string
	info string
}

func (w helpWriter) printTable(rows []helpTableRow, sort bool, singleSpace bool) {
	if sort {
		slices.SortFunc(rows, func(a, b helpTableRow) int {
			return strings.Compare(a.name, b.name)
		})
	}

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
		if !singleSpace {
			fmt.Fprint(w, "\n")
		}
	}
}

func (w helpWriter) commandInfo(_ nestedCommandNames, usage, short, long string) {
	w.printHeader(fmt.Sprintf("Usage: %s\n", usage))
	if long != "" {
		w.printDescription(long)
	} else if short != "" {
		w.printDescription(short)
	}
}

func (w *helpWriter) writeCommandGroup(title string, commandsToPrint []Command, sort bool) {
	w.printHeader(title)
	var table []helpTableRow
	for _, cmd := range commandsToPrint {
		table = append(table, helpTableRow{
			name: cmd.Name,
			info: cmd.Short,
		})
	}
	w.printTable(table, sort, true)
	fmt.Fprint(w, "\n")
}

func (w *helpWriter) writeFlagGroup(title string, group *arguments.FlagGroup, flags []*arguments.Flag, singleSpace bool) {
	var table []helpTableRow
	for _, flag := range flags {
		s := "-" + flag.Name
		if flag.Display != "" {
			s += flag.Display
		}
		table = append(table, helpTableRow{
			name: s,
			info: flag.Usage,
		})
	}

	w.printHeader(title)
	if singleSpace {
		fmt.Fprint(w, "\n")
	}
	if group != nil && group.Description != "" {
		w.printDescription(group.Description)
	}

	w.printTable(table, true, singleSpace)

	if group != nil && group.Suffix != "" {
		w.printDescription(group.Suffix)
	}
}
