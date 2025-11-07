package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultMaxRowLen = 80
)

// NOTE: we can use groups to generate the root command help text categorized on main commands and other commands as `tofu` does right now
type commandGroupId string

const (
	commandGroupIdMain  commandGroupId = "main-commands"
	commandGroupIdOther commandGroupId = "all-other-commands"
)

func (cgi commandGroupId) title() string {
	return strings.ReplaceAll(strings.ToTitle(string(cgi)), "-", " ")
}

func (cgi commandGroupId) id() string {
	return string(cgi)
}

func (cgi commandGroupId) group() *cobra.Group {
	return &cobra.Group{
		ID:    cgi.id(),
		Title: cgi.title(),
	}
}
func parseGroupId(gid string) commandGroupId {
	switch gid {
	case commandGroupIdMain.id():
		return commandGroupIdMain
	case commandGroupIdOther.id():
		return commandGroupIdOther
	}
	panic(fmt.Errorf("unknown group id %s", gid))
}

func groupCommands(cmds []*cobra.Command) map[commandGroupId][]*cobra.Command {
	res := map[commandGroupId][]*cobra.Command{}
	for _, cmd := range cmds {
		if cmd.GroupID == "" {
			continue
		}
		gid := parseGroupId(cmd.GroupID)
		gcmds := res[gid]
		gcmds = append(gcmds, cmd)
		res[gid] = gcmds
	}
	return res
}

func listCommandsForHelp(cmds []*cobra.Command, sort func(a, b *cobra.Command) int, maxKeyLen int) []string {
	slices.SortFunc(cmds, sort)
	var res []string
	for _, subCmd := range cmds {
		key := subCmd.Use
		key = fmt.Sprintf("  %s%s  ", subCmd.Use, strings.Repeat(" ", maxKeyLen-len(key)))
		res = append(res, fmt.Sprintf("%s%s", key, wrap(len(key), defaultMaxRowLen, subCmd.Short)))
	}
	return res
}
