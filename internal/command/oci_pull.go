package command

import "github.com/opentofu/opentofu/internal/oci"

type OciPullCommand struct {
	Meta
}

func (c *OciPullCommand) Run(args []string) int {
	c.Ui.Output("OCI Pull command")
	if err := oci.PullModule(args[0]); err != nil {
		c.Ui.Error(err.Error())
		return 1
	}
	return 0
}

func (c *OciPullCommand) Help() string {
	return "Pull an OCI image."
}

func (c *OciPullCommand) Synopsis() string {
	return "push module to an OCI/Docker registry"
}
