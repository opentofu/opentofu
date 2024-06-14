package options

import (
	"fmt"
	"strings"
)

const (
	ChDir    = "chdir"
	Help     = "help"
	Pedantic = "pedantic"
	Version  = "version"
)

func GetGlobalOptions(args []string) (map[string]string, error) {
	options := make(map[string]string)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// Global options are processed before the subcommand
			// Exit if we have found the subcommand
			break
		}

		option := strings.SplitN(arg[1:], "=", 2)
		if option[0] == ChDir {
			if len(option) != 2 {
				return nil, fmt.Errorf(
					"invalid global option -%s: must include an equals sign followed by a value: -%s=value",
					option[0],
					option[0])
			}
		} else if option[0] == "v" || option[0] == "-version" {
			// Capture -v and --version as version option
			option[0] = Version
		}

		if len(option) != 2 {
			option = append(option, "")
		}
		options[option[0]] = option[1]
	}

	return options, nil
}

func IsGlobalOptionSet(find string, args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// Global options are processed before the subcommand
			// Return if we have found the subcommand
			return false
		}

		option := strings.SplitN(arg[1:], "=", 2)
		if option[0] == find {
			return true
		}
	}
	return false
}
