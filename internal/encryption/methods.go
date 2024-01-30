package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

type AESCipherMethod struct {
	Key []byte `hcl:"cipher"`
}

// Inspired by https://bruinsslot.jp/post/golang-crypto/

func (m AESCipherMethod) Encrypt(data []byte) ([]byte, error) {
	blockCipher, err := aes.NewCipher(m.Key)
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
	blockCipher, err := aes.NewCipher(m.Key)
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
