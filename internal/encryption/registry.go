package encryption

// Registry is a holder of KeyProvider and Method implementations. Key providers and methods can register
// themselves with this registry.
type Registry struct {
	KeyProviders map[string]KeyProviderSource
	Methods      map[string]MethodSource
}

func NewRegistry() Registry {
	return Registry{
		KeyProviders: map[string]KeyProviderSource{
			"passphrase": func() KeyProvider { return &PassphraseKeyProvider{} },
		},
		Methods: map[string]MethodSource{
			"aes_cipher": func() Method { return &AESCipherMethod{} },
		},
	}
}

type KeyProvider interface {
	KeyData() ([]byte, error)
}
type KeyProviderSource func() KeyProvider

type Method interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}
type MethodSource func() Method
