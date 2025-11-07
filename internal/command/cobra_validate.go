package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCobraValidateCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:                "validate",
		Short:              "Check whether the configuration is valid",
		Long:               "",
		DisableFlagParsing: true,
		GroupID:            commandGroupIdMain.id(),
		// ValidArgs:                  nil,
		// ValidArgsFunction:          nil,
		// Args:                       nil,
	}
	cmd.Run = func(cmd *cobra.Command, args []string) {
		fmt.Println("execute validate")
	}

	rootCmd.AddCommand(cmd)
}
