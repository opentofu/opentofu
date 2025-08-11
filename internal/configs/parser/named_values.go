package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

type Variable struct {
	Name         string         `hcl:"name,label"`
	NameRange    hcl.Range      `hcl:"name,label_range"`
	Description  *string        `hcl:"description,attr"`
	Default      *cty.Value     `hcl:"default,attr"`
	DefaultRange hcl.Range      `hcl:"default,attr_range"`
	Type         *hcl.Attribute `hcl:"type,attr"`

	Validations []*CheckRule `hcl:"validation,block"`

	Sensitive       *bool     `hcl:"sensitive,attr"`
	Deprecated      *string   `hcl:"deprecated,attr"`
	DeprecatedRange hcl.Range `hcl:"deprecated,attr_range"`
	Ephemeral       *bool     `hcl:"ephemeral,attr"`
	Nullable        *bool     `hcl:"nullable,attr"`

	DefRange hcl.Range `hcl:",def_range"`
}

type Locals Block

type Output struct {
	Name            string         `hcl:"name,label"`
	NameRange       hcl.Range      `hcl:"name,label_range"`
	Description     *string        `hcl:"description,attr"`
	Value           *hcl.Attribute `hcl:"value,attr"`
	DependsOn       *hcl.Attribute `hcl:"depends_on,attr"`
	Sensitive       *bool          `hcl:"sensitive,attr"`
	Deprecated      *string        `hcl:"deprecated,attr"`
	DeprecatedRange hcl.Range      `hcl:"deprecated,attr_range"`
	Ephemeral       *bool          `hcl:"ephemeral,attr"`

	Preconditions  []*CheckRule `hcl:"precondition,block"`
	Postconditions []*CheckRule `hcl:"postcondition,block"`

	DefRange hcl.Range `hcl:",def_range"`
}
