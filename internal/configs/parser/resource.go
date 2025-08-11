package parser

import (
	"github.com/hashicorp/hcl/v2"
)

type Resource struct {
	Type      string    `hcl:"type,label"`
	TypeRange hcl.Range `hcl:"type,label_range"`
	Name      string    `hcl:"name,label"`
	NameRange hcl.Range `hcl:"name,label_range"`

	Config hcl.Body `hcl:",remain"`

	Count   *hcl.Attribute `hcl:"count,attr"`
	ForEach *hcl.Attribute `hcl:"for_each,attr"`

	Provider *hcl.Attribute `hcl:"provider,attr"`

	DependsOn *hcl.Attribute `hcl:"depends_on,attr"`

	Connection   []*Block       `hcl:"connection,block"`
	Provisioners []*Provisioner `hcl:"provisioner,block"`

	Lifecycle []*ResourceLifecycle `hcl:"lifecycle,block"`
	Escaped   []*Block             `hcl:"_,block"`
	Locals    []*Block             `hcl:"locals,block"`

	DefRange hcl.Range `hcl:",def_range"`
}

type ResourceLifecycle struct {
	CreateBeforeDestroy *bool          `hcl:"create_before_destroy,attr"`
	PreventDestroy      *bool          `hcl:"prevent_destroy,attr"`
	IgnoreChanges       *hcl.Attribute `hcl:"ignore_changes,attr"`
	TriggersReplacement *hcl.Attribute `hcl:"replace_triggered_by,attr"`
	Preconditions       []*CheckRule   `hcl:"precondition,block"`
	Postconditions      []*CheckRule   `hcl:"postcondition,block"`
	DefRange            hcl.Range      `hcl:",def_range"`
}

type Block struct {
	Body      hcl.Body  `hcl:",remain"`
	DefRange  hcl.Range `hcl:",def_range"`
	TypeRange hcl.Range `hcl:",type_range"`
}
