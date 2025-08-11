package parser

import "github.com/hashicorp/hcl/v2"

type Moved struct {
	From *hcl.Attribute `hcl:"from,attr"`
	To   *hcl.Attribute `hcl:"to,attr"`

	DefRange hcl.Range `hcl:",def_range"`
}

type Import struct {
	ID *hcl.Attribute `hcl:"id,attr"`
	To *hcl.Attribute `hcl:"to,attr"`

	ForEach  *hcl.Attribute `hcl:"for_each,attr"`
	Provider *hcl.Attribute `hcl:"provider,attr"`

	DefRange hcl.Range `hcl:",def_range"`
}

type Removed struct {
	From *hcl.Attribute `hcl:"from,attr"`

	Provisioners []*Provisioner      `hcl:"provisioner,block"`
	Lifecycle    []*RemovedLifecycle `hcl:"lifecycle,block"`

	DefRange hcl.Range `hcl:",def_range"`
}

type RemovedLifecycle struct {
	Destroy *bool `hcl:"destroy,attr"`

	DefRange hcl.Range `hcl:",def_range"`
}
