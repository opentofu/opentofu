package gcp_kms

import (
	"context"
	"crypto/rand"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Ciphertext []byte `json:"ciphertext"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

type keyProvider struct {
	svc     *kms.KeyManagementClient
	ctx     context.Context
	keyName string
	keySize int
}

func (p keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: no metadata struct provided"}
	}
	inMeta, ok := rawMeta.(*keyMeta)
	if !ok {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: invalid metadata struct type"}
	}

	outMeta := &keyMeta{}
	out := keyprovider.Output{}

	// Generate new key
	out.EncryptionKey = make([]byte, p.keySize)
	_, err := rand.Read(out.EncryptionKey)
	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to generate key",
			Cause:   err,
		}
	}

	// Encrypt new encryption key using kms
	encryptedKeyData, err := p.svc.Encrypt(p.ctx, &kmspb.EncryptRequest{
		Name:      p.keyName,
		Plaintext: out.EncryptionKey,
	})
	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to encrypt key",
			Cause:   err,
		}
	}

	outMeta.Ciphertext = encryptedKeyData.Ciphertext

	// We do not set the DecryptionKey here as we should only be setting the decryption key if we are decrypting
	// and that is handled below when we check if the inMeta has a CiphertextBlob

	if inMeta.isPresent() {
		// We have an existing decryption key to decrypt, so we should now populate the DecryptionKey
		decryptedKeyData, decryptErr := p.svc.Decrypt(p.ctx, &kmspb.DecryptRequest{
			Name:       p.keyName,
			Ciphertext: inMeta.Ciphertext,
		})

		if decryptErr != nil {
			return out, outMeta, decryptErr
		}

		// Set decryption key on the output
		out.DecryptionKey = decryptedKeyData.Plaintext
	}

	return out, outMeta, nil
}
