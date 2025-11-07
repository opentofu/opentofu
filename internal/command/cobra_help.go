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
		newLines := func(in string, newLines int) string {
			return fmt.Sprintf("%s%s", strings.Repeat("\n", newLines), in)
		}
		// groupedCmds := group[string, string, *cobra.Command](cmd.Commands(), func(command *cobra.Command) (string, string) {
		// 	return command.Use, command.Short
		// })
		// Determine the longest key to have that length as a reference for alignment
		var maxKeyLen int
		for _, cmd := range cmd.Commands() {
			if cmdLen := len(cmd.Use); cmdLen > maxKeyLen {
				maxKeyLen = cmdLen
			}
		}
		cmd.Root().Flags().VisitAll(func(flag *pflag.Flag) {
			if flagNameLen := len(flag.Name) + 1; flagNameLen > maxKeyLen {
				maxKeyLen = flagNameLen
			}
		})
		cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
			if flagNameLen := len(flag.Name) + 1; flagNameLen > maxKeyLen {
				maxKeyLen = flagNameLen
			}
		})
		// Group commands in main and other
		grouped := groupCommands(cmd.Commands())
		mainCommandsOrder := []string{"init", "validate", "plan", "apply", "destroy"}
		mainCommands := listCommandsForHelp(grouped[commandGroupIdMain], func(a, b *cobra.Command) int {
			aIdx := slices.Index(mainCommandsOrder, a.Use)
			bIdx := slices.Index(mainCommandsOrder, b.Use)
			return aIdx - bIdx
		}, maxKeyLen)
		otherCommands := listCommandsForHelp(grouped[commandGroupIdOther], func(a, b *cobra.Command) int {
			return strings.Compare(a.Use, b.Use)
		}, maxKeyLen)
		// Generate string representation for sub commands
		var mainCommandsStr, otherCommandsStr string
		if len(mainCommands) > 0 {
			mainCommandsStr = fmt.Sprintf("Main commands:\n%s", strings.Join(mainCommands, "\n"))
			mainCommandsStr = newLines(mainCommandsStr, 2)
		}
		if len(otherCommands) > 0 {
			otherCommandsStr = fmt.Sprintf("All other commands:\n%s", strings.Join(otherCommands, "\n"))
			otherCommandsStr = newLines(otherCommandsStr, 2)
		}

		// Format the global flags
		globalFlags := formatFlags(cmd.Root().Flags(), maxKeyLen)
		var globalFlagsStr string
		if len(globalFlags) > 0 {
			globalFlagsStr = fmt.Sprintf("Global options (use these before the subcommand, if any):\n%s", strings.Join(globalFlags, "\n"))
			globalFlagsStr = newLines(globalFlagsStr, 2)
		}
		// Format the local flags
		localFlags := formatFlags(cmd.LocalFlags(), maxKeyLen)
		var localFlagsStr string
		if len(localFlags) > 0 {
			localFlagsStr = fmt.Sprintf("Options:\n%s", strings.Join(localFlags, "\n"))
			localFlagsStr = newLines(localFlagsStr, 2)
		}
		helpText := fmt.Sprintf(
			`Usage: %s%s%s%s%s%s`,
			cmd.Use,
			newLines(wrap(0, defaultMaxRowLen, cmd.Long), 2),
			mainCommandsStr,
			otherCommandsStr,
			localFlagsStr,
			globalFlagsStr,
		)
		return helpText
	}
}

func formatFlags(flags *pflag.FlagSet, maxKeyLen int) []string {
	var globalFlags []string
	flags.VisitAll(func(flag *pflag.Flag) {
		key := fmt.Sprintf("-%s", flag.Name)
		key = fmt.Sprintf("  %s%s  ", key, strings.Repeat(" ", maxKeyLen-len(key)))
		globalFlags = append(globalFlags, fmt.Sprintf("%s%s", key, wrap(len(key), defaultMaxRowLen, flag.Usage)))
	})
	return globalFlags
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

func group[K comparable, V any, IN any](in []IN, mapping func(IN) (K, V)) map[K]V {
	res := map[K]V{}
	for _, i := range in {
		k, v := mapping(i)
		res[k] = v
	}
	return res
}
