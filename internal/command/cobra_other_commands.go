package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCobraOtherCommands(m Meta, rootCmd *cobra.Command) {
	other := map[string]string{
		"console":      "Try OpenTofu expressions at an interactive command prompt",
		"fmt":          "Reformat your configuration in the standard style",
		"force-unlock": "Release a stuck lock on the current workspace",
		"get":          "Install or upgrade remote OpenTofu modules",
		"graph":        "Generate a Graphviz graph of the steps in an operation",
		"import":       "Associate existing infrastructure with a OpenTofu resource",
		"login":        "Obtain and save credentials for a remote host",
		"logout":       "Remove locally-stored credentials for a remote host",
		"metadata":     "Metadata related commands",
		"output":       "Show output values from your root module",
		"providers":    "Show the providers required for this configuration",
		"refresh":      "Update the state to match remote systems",
		"show":         "Show the current state or a saved plan",
		"state":        "Advanced state management",
		"taint":        "Mark a resource instance as not fully functional",
		"test":         "Execute integration tests for OpenTofu modules",
		"untaint":      "Remove the 'tainted' state from a resource instance",
		"version":      "Show the current OpenTofu version",
		"workspace":    "Workspace management",
	}
	for cmdName, desc := range other {
		cmd := &cobra.Command{
			Use:                cmdName,
			Short:              desc,
			Long:               "",
			DisableFlagParsing: true,
			GroupID:            commandGroupIdOther.id(),
			// ValidArgs:                  nil,
			// ValidArgsFunction:          nil,
			// Args:                       nil,
		}
		cmd.Run = func(cmd *cobra.Command, args []string) {
			fmt.Println("execute", cmdName)
		}
		rootCmd.AddCommand(cmd)
	}

}
