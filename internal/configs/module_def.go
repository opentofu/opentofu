package configs

import "github.com/hashicorp/hcl/v2"

type ModuleDef struct {
	Name     string
	Contents *Module
}

func decodeModuleDefBlock(block *hcl.Block) (*ModuleDef, hcl.Diagnostics) {
	return nil, nil
}
