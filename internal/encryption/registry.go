package encryption

import (
	"github.com/hashicorp/hcl/v2"
)

// Registry is a holder of KeyProvider and Method implementations. Key providers and methods can register
// themselves with this registry. You can call the Configure function to parse an HCL block as configuration.
type Registry interface {
	RegisterKeyProvider(string, KeyProviderDefinition) error
	RegisterMethod(string, MethodDefinition) error

	GetKeyProvider(string) KeyProviderDefinition
	GetMethod(string) MethodDefinition
}

type DefinitionSchema struct {
	Schema            hcl.BodySchema
	KeyProviderFields []string
}

type KeyProviderDefinition interface {
	Schema() DefinitionSchema
	Configure(hcl.Block, map[string]KeyProvider) (KeyProvider, error)
}

type KeyProvider func() ([]byte, error)

type MethodDefinition interface {
	Schema() DefinitionSchema
	Configure(hcl.Block, map[string]KeyProvider) (Method, error)
}

type Method func([]byte) ([]byte, error)
