package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCobraDestroyCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:                "destroy",
		Short:              "Destroy previously-created infrastructure",
		Long:               "",
		DisableFlagParsing: true,
		GroupID:            commandGroupIdMain.id(),
		// ValidArgs:                  nil,
		// ValidArgsFunction:          nil,
		// Args:                       nil,
	}
	cmd.Run = func(cmd *cobra.Command, args []string) {
		fmt.Println("execute destroy")
	}

	rootCmd.AddCommand(cmd)
}
