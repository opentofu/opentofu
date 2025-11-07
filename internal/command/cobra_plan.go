package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCobraPlanCommand(m Meta, rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:                "plan",
		Short:              "Show changes required by the current configuration",
		Long:               "",
		DisableFlagParsing: true,
		GroupID:            commandGroupIdMain.id(),
		// ValidArgs:                  nil,
		// ValidArgsFunction:          nil,
		// Args:                       nil,
	}
	cmd.Run = func(cmd *cobra.Command, args []string) {
		fmt.Println("execute plan")
	}

	rootCmd.AddCommand(cmd)
}
