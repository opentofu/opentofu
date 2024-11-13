package command

type OciPackageCommand struct {
	Meta
}

func (c *OciPackageCommand) Run(args []string) int {
	c.Ui.Output("OCI package command")
	return 0
}

func (c *OciPackageCommand) Help() string {
	return "Package an OCI image."
}
