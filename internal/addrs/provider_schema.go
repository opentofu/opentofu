package addrs

type ProviderResourceRequirments map[ResourceMode]map[string]struct{}
type ProviderSchemaRequirements map[Provider]ProviderResourceRequirments

func (p ProviderSchemaRequirements) AddResource(provider Provider, mode ResourceMode, typ string) {
	pm, ok := p[provider]
	if !ok {
		pm = make(ProviderResourceRequirments)
		p[provider] = pm
	}
	mm, ok := pm[mode]
	if !ok {
		mm = make(map[string]struct{})
		pm[mode] = mm
	}
	mm[typ] = struct{}{}
}

func (s ProviderResourceRequirments) HasResource(mode ResourceMode, typ string) bool {
	if s == nil {
		// Legacy path
		return true
	}
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
