package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

type File struct {
	Product []*Product `hcl:"terraform,block"`

	RequiredProviders []*RequiredProviders `hcl:"required_providers,block"`

	ProviderConfigs []*ProviderConfig `hcl:"provider,block"`

	Variables []*Variable `hcl:"variable,block"`
	Locals    []*Locals   `hcl:"locals,block"`
	Outputs   []*Output   `hcl:"output,block"`

	ModuleCalls []*ModuleCall `hcl:"module,block"`

	ManagedResources   []*Resource `hcl:"resource,block"`
	DataResources      []*Resource `hcl:"data,block"`
	EphemeralResources []*Resource `hcl:"ephemeral,block"`

	Moved   []*Moved   `hcl:"moved,block"`
	Import  []*Import  `hcl:"import,block"`
	Removed []*Removed `hcl:"removed,block"`

	Checks []*Check `hcl:"check,block"`
}

func NewFile(body hcl.Body) (*File, hcl.Diagnostics) {
	var file File
	diags := gohcl.DecodeBody(body, new(hcl.EvalContext), &file)
	return &file, diags
}
