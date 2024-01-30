package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"github.com/hashicorp/hcl/v2"
)

// TODO: THIS IS NOT PRODUCTION CODE AND HAS NOT BEEN AUDITED.
func AESCipherMethod() (DefinitionSchema, MethodProvider) {
	schema := DefinitionSchema{
		BodySchema: &hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "cipher", Required: true},
			},
		},
		KeyProviderFields: []string{"cipher"},
	}

	return schema, func(content *hcl.BodyContent, keys map[string]KeyData) (Method, hcl.Diagnostics) {
		// Could probably init the cipher and GCM here?
		return &AESCipherMethodImpl{
			key: keys["cipher"],
		}, nil
	}
}

type AESCipherMethodImpl struct {
	key []byte
}

// Inspired by https://bruinsslot.jp/post/golang-crypto/

func (m AESCipherMethodImpl) Encrypt(data []byte) ([]byte, error) {
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
func (m AESCipherMethodImpl) Decrypt(data []byte) ([]byte, error) {
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
