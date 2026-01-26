package arguments

import (
	"flag"
	"os"
	"strconv"

	"github.com/opentofu/opentofu/internal/tofu"
)

const (
	// InputModeEnvVar is the environment variable that, if set to "false" or
	// "0", causes tofu commands to behave as if the `-input=false` flag was
	// specified.
	InputModeEnvVar = "TF_INPUT"
)

type Input struct {
	input bool
}

func (i *Input) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&i.input, "input", true, "input")
}

// Input returns whether or not input asking is enabled.
func (i *Input) Input(forceDisabled bool) bool {
	if forceDisabled || !i.input {
		return false
	}

	if envVar := os.Getenv(InputModeEnvVar); envVar != "" {
		if v, err := strconv.ParseBool(envVar); err == nil && !v {
			return false
		}
	}

	return true
}

func (i *Input) InputFlag() bool {
	return i.input
}

func (i *Input) InputMode(forceDisabled bool) tofu.InputMode {
	if forceDisabled || !i.input {
		return 0
	}

	if envVar := os.Getenv(InputModeEnvVar); envVar != "" {
		if v, err := strconv.ParseBool(envVar); err == nil {
			if !v {
				return 0
			}
		}
	}

	var mode tofu.InputMode
	mode |= tofu.InputModeProvider

	return mode
}
