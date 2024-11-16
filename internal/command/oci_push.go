package command

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/oci"
)

type OciPushCommand struct {
	Meta
}

func (c *OciPushCommand) Run(args []string) int {
	fmt.Println(args)
	if err := validateArgs(args); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}
	ref := args[0]
	path := args[1]

	if err := oci.PushPackagedModule(ref, path); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	c.Ui.Output("Push Complete")
	return 0
}

func (c *OciPushCommand) Help() string {
	return "push module to an OCI registry"
}

func (c *OciPushCommand) Synopsis() string {
	return "push module to an OCI/Docker registry"
}

func validateArgs(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("invalid number of arguments")
	}

	return nil
}
