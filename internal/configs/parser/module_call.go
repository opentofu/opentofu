package parser

import "github.com/hashicorp/hcl/v2"

type ModuleCall struct {
	Name      string         `hcl:"name,label"`
	NameRange hcl.Range      `hcl:"name,label_range"`
	Source    *hcl.Attribute `hcl:"source,attr"`

	Version *hcl.Attribute `hcl:"version"`

	Count   *hcl.Attribute `hcl:"count,attr"`
	ForEach *hcl.Attribute `hcl:"for_each,attr"`

	DependsOn *hcl.Attribute `hcl:"depends_on,attr"`

	Providers *hcl.Attribute `hcl:"providers,attr"`

	Escaped   []*Block `hcl:"_,block"`
	Lifecycle []*Block `hcl:"lifecycle,block"`
	Locals    []*Block `hcl:"locals,block"`
	Provider  []*Block `hcl:"provider,block"`

	Config   hcl.Body  `hcl:",remain"`
	DefRange hcl.Range `hcl:",def_range"`
}
