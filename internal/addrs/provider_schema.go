package addrs

type SchemaRequirements map[ResourceMode]map[string]struct{}
type ProviderSchemaRequirements map[Provider]SchemaRequirements

func (p ProviderSchemaRequirements) AddResource(provider Provider, mode ResourceMode, typ string) {
	pm, ok := p[provider]
	if !ok {
		pm = make(SchemaRequirements)
		p[provider] = pm
	}
	mm, ok := pm[mode]
	if !ok {
		mm = make(map[string]struct{})
		pm[mode] = mm
	}
	mm[typ] = struct{}{}
}

func (s SchemaRequirements) HasResource(mode ResourceMode, typ string) bool {
	_, ok := s[mode][typ]
	return ok
}

func (p ProviderSchemaRequirements) Merge(other ProviderSchemaRequirements) {
	for provider, pm := range other {
		for mode, mm := range pm {
			for typ := range mm {
				p.AddResource(provider, mode, typ)
			}
		}

	}
}
