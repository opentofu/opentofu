package parser

import "github.com/hashicorp/hcl/v2"

type Product struct {
	Experiments          *hcl.Attribute `hcl:"experiments,attr"`
	Language             *hcl.Attribute `hcl:"language,attr"`
	RequiredVersion      *hcl.Attribute `hcl:"required_version,attr"`
	RequiredVersionRange hcl.Range      `hcl:"required_version,attr_range"`

	RequiredProviders *RequiredProviders `hcl:"required_providers,block"`

	Backend      *Backend        `hcl:"backend,block"`
	Cloud        *Block          `hcl:"cloud,block"`
	Encryption   *Block          `hcl:"encryption,block"`
	ProviderMeta []*ProviderMeta `hcl:"provider_meta,block"`
}

type Backend struct {
	Type      string    `hcl:"type,label"`
	TypeRange hcl.Range `hcl:"type,label_range"`
	DefRange  hcl.Range `hcl:",def_range"`
	Body      hcl.Body  `hcl:",remain"`
}

type RequiredProviders struct {
	Body      hcl.Body  `hcl:",remain"`
	DefRange  hcl.Range `hcl:",def_range"`
	TypeRange hcl.Range `hcl:"type,label_range"`
}

type ProviderMeta struct {
	Provider      string    `hcl:"provider,label"`
	ProviderRange hcl.Range `hcl:"provider,label_range"`
	Body          hcl.Body  `hcl:",remain"`
	DefRange      hcl.Range `hcl:",def_range"`
}
