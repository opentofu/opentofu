package providers

import "github.com/terramate-io/opentofulib/internal/addrs"

func NewMockSchemaCache() *schemaCache {
	return &schemaCache{
		m: make(map[addrs.Provider]ProviderSchema),
	}
}
