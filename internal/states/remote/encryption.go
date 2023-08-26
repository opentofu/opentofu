package remote

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

var encryptionMagicPrefix = []byte{120, 0, 120, 0, 120, 0, 120, 0}

func (s *State) ensureEncryptionInitialized() {
	if s.encryptionInitialized {
		return
	}
	s.encryptionInitialized = true
	passphrase, ok := os.LookupEnv("TF_REMOTE_STATE_ENCRYPTION_PASSPHRASE")
	if !ok {
		return
	}

	s.Client = &EncryptingClient{
		Client:     s.Client,
		passphrase: passphrase,
	}
}

type EncryptingClient struct {
	Client
	passphrase string
}

func (c *EncryptingClient) Get() (*Payload, error) {
	payload, err := c.Client.Get()
	if err != nil {
		return nil, err
	}

	if payload == nil || payload.Data == nil {
		return payload, nil
	}

	if !bytes.Equal(payload.Data[:len(encryptionMagicPrefix)], encryptionMagicPrefix) {
		return payload, nil
	}

	saltOffset := len(encryptionMagicPrefix)
	salt := payload.Data[saltOffset : saltOffset+16]

	key := pbkdf2.Key([]byte(c.passphrase), salt, 4096, 32, sha512.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("could not create block cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("could not create GCM: %w", err)
	}

	nonceOffset := saltOffset + len(salt)
	nonce := payload.Data[nonceOffset : nonceOffset+aesgcm.NonceSize()]
	ciphertextOffset := nonceOffset + len(nonce)
	ciphertext := payload.Data[ciphertextOffset:]

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, salt)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt state: %w", err)
	}

	return &Payload{
		MD5:  payload.MD5,
		Data: plaintext,
	}, nil
}

func (c *EncryptingClient) Put(data []byte) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("could not generate salt: %w", err)
	}

	key := pbkdf2.Key([]byte(c.passphrase), salt, 4096, 32, sha512.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("could not create block cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("could not create GCM: %w", err)
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("could not generate nonce: %w", err)
	}

	// salt is added as additional data so that it's authenticated
	ciphertext := aesgcm.Seal(nil, nonce, data, salt)

	var dataToStore bytes.Buffer
	dataToStore.Write(encryptionMagicPrefix)
	dataToStore.Write(salt)
	dataToStore.Write(nonce)
	dataToStore.Write(ciphertext)

	return c.Client.Put(dataToStore.Bytes())
}
