package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"github.com/hashicorp/hcl/v2"
)

// TODO: THIS IS NOT PRODUCTION CODE AND HAS NOT BEEN AUDITED.
type AESCipherMethodDef struct{}

func (m AESCipherMethodDef) Schema() DefinitionSchema {
	return DefinitionSchema{
		BodySchema: &hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "cipher", Required: true},
			},
		},
		KeyProviderFields: []string{"cipher"},
	}
}

func (m AESCipherMethodDef) Configure(content *hcl.BodyContent, keys map[string]KeyProvider) (Method, hcl.Diagnostics) {
	key, _ := keys["cipher"]()

	// Could probably init the cipher and GCM here?

	return &AESCipherMethod{
		key: key,
	}, nil
}

type AESCipherMethod struct {
	key []byte
}

// Inspired by https://bruinsslot.jp/post/golang-crypto/

func (m AESCipherMethod) Encrypt(data []byte) ([]byte, error) {
	blockCipher, err := aes.NewCipher(m.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, nil
}
func (m AESCipherMethod) Decrypt(data []byte) ([]byte, error) {
	blockCipher, err := aes.NewCipher(m.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
