package parser

import "github.com/hashicorp/hcl/v2"

type CheckRule struct {
	Condition    hcl.Expression `hcl:"condition,attr"`
	ErrorMessage hcl.Expression `hcl:"error_message,attr"`

	TypeRange hcl.Range `hcl:",type_range"`
	DefRange  hcl.Range `hcl:",def_range"`
}

type Check struct {
	Name      string    `hcl:"name,label"`
	NameRange hcl.Range `hcl:"name,label_range"`

	DataResource []*Resource  `hcl:"data,block"`
	Asserts      []*CheckRule `hcl:"assert,block"`

	DefRange hcl.Range `hcl:",def_range"`
}
