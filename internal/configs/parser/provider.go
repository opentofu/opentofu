package parser

import "github.com/hashicorp/hcl/v2"

type ProviderConfig struct {
	Name       string    `hcl:"name,label"`
	NameRange  hcl.Range `hcl:"name,label_range"`
	Alias      *string   `hcl:"alias,attr"`
	AliasRange hcl.Range `hcl:"alias,attr_range"`

	ForEach *hcl.Attribute `hcl:"for_each,attr"`
	Count   *hcl.Attribute `hcl:"count,attr"`

	DependsOn *hcl.Attribute `hcl:"depends_on,attr"`
	Source    *hcl.Attribute `hcl:"source,attr"`
	Version   *hcl.Attribute `hcl:"version,attr"`

	Escaped   []*Block `hcl:"_,block"`
	Lifecycle []*Block `hcl:"lifecycle,block"`
	Locals    []*Block `hcl:"locals,block"`

	Config   hcl.Body  `hcl:",remain"`
	DefRange hcl.Range `hcl:",def_range"`
}
