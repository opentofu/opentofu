package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func commandHelp() func(command *cobra.Command) string {
	return func(cmd *cobra.Command) string {
		// utility functions
		newLines := func(in string, newLines int) string {
			return fmt.Sprintf("%s%s", strings.Repeat("\n", newLines), in)
		}
		indent := func(in string, i int) string {
			return fmt.Sprintf("%s%s", strings.Repeat(" ", i), in)
		}

		// group commands
		var mainCommandsStr, otherCommandsStr string
		// Group commands in main and other
		grouped := groupCommands(cmd.Commands())
		mainCommandsHelpEntries, longestMainCmd := convertCommands(grouped[commandGroupIdMain])
		mainCommandsOrder := []string{"init", "validate", "plan", "apply", "destroy"}
		mainCommands := generateHelpTextForEntries(mainCommandsHelpEntries, func(a, b helpEntry) int {
			aIdx := slices.Index(mainCommandsOrder, a.k)
			bIdx := slices.Index(mainCommandsOrder, b.k)
			return aIdx - bIdx
		}, longestMainCmd)
		if len(mainCommands) > 0 {
			mainCommandsStr = fmt.Sprintf("Main commands:\n%s", strings.Join(mainCommands, "\n"))
			mainCommandsStr = newLines(mainCommandsStr, 2)
		}
		otherCommandsHelpEntries, longestOtherCmd := convertCommands(grouped[commandGroupIdOther])
		otherCommands := generateHelpTextForEntries(otherCommandsHelpEntries, func(a, b helpEntry) int {
			return strings.Compare(a.k, b.k)
		}, longestOtherCmd)
		if len(otherCommands) > 0 {
			otherCommandsStr = fmt.Sprintf("All other commands:\n%s", strings.Join(otherCommands, "\n"))
			otherCommandsStr = newLines(otherCommandsStr, 2)
		}

		// Format flags
		globalFlagsHelpEntries, longestGlobalFlag := convertFlags(cmd.Root().Flags())
		globalFlags := generateHelpTextForEntries(globalFlagsHelpEntries, nil, longestGlobalFlag)
		var globalFlagsStr string
		if len(globalFlags) > 0 {
			globalFlagsStr = fmt.Sprintf("Global options (use these before the subcommand, if any):\n%s", strings.Join(globalFlags, "\n"))
			globalFlagsStr = newLines(globalFlagsStr, 2)
		}
		var localFlagsStr string
		if cmd.Root() != cmd {
			// Format the local flags
			localFlagsHelpEntries, longestLocalFlag := convertFlags(cmd.LocalFlags())
			localFlags := generateHelpTextForEntries(localFlagsHelpEntries, nil, longestLocalFlag)
			if len(localFlags) > 0 {
				localFlagsStr = fmt.Sprintf("Options:\n%s", strings.Join(localFlags, "\n"))
				localFlagsStr = newLines(localFlagsStr, 2)
			}
		}

		// Build final text
		helpText := fmt.Sprintf(
			`Usage: %s%s%s%s%s%s`,
			cmd.Use,
			newLines(
				indent(
					wrap(2, defaultMaxRowLen, cmd.Long),
					2,
				),
				2,
			),
			mainCommandsStr,
			otherCommandsStr,
			localFlagsStr,
			globalFlagsStr,
		)
		return helpText
	}
}

type helpEntry struct {
	k, v string
}

func generateHelpTextForEntries(entries []helpEntry, sort func(a, b helpEntry) int, maxKeyLen int) []string {
	if sort != nil {
		slices.SortFunc(entries, sort)
	}
	var res []string
	for _, e := range entries {
		key := e.k
		key = fmt.Sprintf("  %s%s  ", key, strings.Repeat(" ", maxKeyLen-len(key)))
		res = append(res, fmt.Sprintf("%s%s", key, wrap(len(key), defaultMaxRowLen, e.v)))
	}
	return res
}

func convertCommands(in []*cobra.Command) ([]helpEntry, int) {
	res := make([]helpEntry, 0, len(in))
	var maxKeySize int
	for _, i := range in {
		k := i.Use
		v := i.Short
		if l := len(k); l > maxKeySize {
			maxKeySize = l
		}
		res = append(res, helpEntry{
			k: k,
			v: v,
		})
	}
	return res, maxKeySize
}

func convertFlags(set *pflag.FlagSet) ([]helpEntry, int) {
	var res []helpEntry
	var maxFlagSize int
	set.VisitAll(func(flag *pflag.Flag) {
		key := fmt.Sprintf("-%s", flag.Name)
		if l := len(key); l > maxFlagSize {
			maxFlagSize = l
		}
		res = append(res, helpEntry{
			k: key,
			v: flag.Usage,
		})
	})
	return res, maxFlagSize
}

// NOTE: copy pasted from pflag as it is
// Splits the string `s` on whitespace into an initial substring up to
// `i` runes in length and the remainder. Will go `slop` over `i` if
// that encompasses the entire string (which allows the caller to
// avoid short orphan words on the final line).
func wrapN(i, slop int, s string) (string, string) {
	if i+slop > len(s) {
		return s, ""
	}

	w := strings.LastIndexAny(s[:i], " \t\n")
	if w <= 0 {
		return s, ""
	}
	nlPos := strings.LastIndex(s[:i], "\n")
	if nlPos > 0 && nlPos < w {
		return s[:nlPos], s[nlPos+1:]
	}
	return s[:w], s[w+1:]
}

// Wraps the string `s` to a maximum width `w` with leading indent
// `i`. The first line is not indented (this is assumed to be done by
// caller). Pass `w` == 0 to do no wrapping
func wrap(i, w int, s string) string {
	if w == 0 {
		return strings.Replace(s, "\n", "\n"+strings.Repeat(" ", i), -1)
	}

	// space between indent i and end of line width w into which
	// we should wrap the text.
	wrap := w - i

	var r, l string

	// Not enough space for sensible wrapping. Wrap as a block on
	// the next line instead.
	if wrap < 24 {
		i = 16
		wrap = w - i
		r += "\n" + strings.Repeat(" ", i)
	}
	// If still not enough space then don't even try to wrap.
	if wrap < 24 {
		return strings.Replace(s, "\n", r, -1)
	}

	// Try to avoid short orphan words on the final line, by
	// allowing wrapN to go a bit over if that would fit in the
	// remainder of the line.
	slop := 5
	wrap = wrap - slop

	// Handle first line, which is indented by the caller (or the
	// special case above)
	l, s = wrapN(wrap, slop, s)
	r = r + strings.Replace(l, "\n", "\n"+strings.Repeat(" ", i), -1)

	// Now wrap the rest
	for s != "" {
		var t string

		t, s = wrapN(wrap, slop, s)
		r = r + "\n" + strings.Repeat(" ", i) + strings.Replace(t, "\n", "\n"+strings.Repeat(" ", i), -1)
	}

	return r
}
