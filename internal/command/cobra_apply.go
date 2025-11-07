package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCobraApplyCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:                "apply",
		Short:              "Create or update infrastructure",
		Long:               "",
		DisableFlagParsing: true,
		GroupID:            commandGroupIdMain.id(),
		// ValidArgs:                  nil,
		// ValidArgsFunction:          nil,
		// Args:                       nil,
	}
	cmd.Run = func(cmd *cobra.Command, args []string) {
		fmt.Println("execute apply")
	}

	rootCmd.AddCommand(cmd)
}
