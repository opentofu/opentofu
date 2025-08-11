package parser

import "github.com/hashicorp/hcl/v2"

type Provisioner struct {
	Type      string    `hcl:"type,label"`
	TypeRange hcl.Range `hcl:"type,label_range"`
	Config    hcl.Body  `hcl:",remain"`

	Connection []*Block `hcl:"connection,block"`
	Escaped    []*Block `hcl:"_,block"`
	Lifecycle  []*Block `hcl:"lifecycle,block"`

	When      *hcl.Attribute `hcl:"when,attr"`
	OnFailure *hcl.Attribute `hcl:"on_failure,attr"`

	DefRange hcl.Range `hcl:",def_range"`
}
