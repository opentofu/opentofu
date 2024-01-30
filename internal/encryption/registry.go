package encryption

import (
	"github.com/hashicorp/hcl/v2"
)

// Registry is a holder of KeyProvider and Method implementations. Key providers and methods can register
// themselves with this registry. You can call the Configure function to parse an HCL block as configuration.
type Registry struct {
	KeyProviders map[string]KeyProviderDefinition
	Methods      map[string]MethodDefinition
}

func NewRegistry() Registry {
	return Registry{
		KeyProviders: map[string]KeyProviderDefinition{
			"passphrase": PassphraseKeyProvider,
		},
		Methods: map[string]MethodDefinition{
			"aes_cipher": AESCipherMethod,
		},
	}
}

type DefinitionSchema struct {
	BodySchema        *hcl.BodySchema
	KeyProviderFields []string
}

type KeyData []byte
type KeyProvider func(*hcl.BodyContent, map[string]KeyData) (KeyData, hcl.Diagnostics)
type KeyProviderDefinition func() (DefinitionSchema, KeyProvider)

type Method interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}
type MethodProvider func(*hcl.BodyContent, map[string]KeyData) (Method, hcl.Diagnostics)
type MethodDefinition func() (DefinitionSchema, MethodProvider)
