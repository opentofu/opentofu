// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gcp_kms

import (
	"context"
	"crypto/rand"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Ciphertext []byte `json:"ciphertext"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

type keyManagementClient interface {
	Encrypt(ctx context.Context, req *kmspb.EncryptRequest, opts ...gax.CallOption) (*kmspb.EncryptResponse, error)
	Decrypt(ctx context.Context, req *kmspb.DecryptRequest, opts ...gax.CallOption) (*kmspb.DecryptResponse, error)
}

type keyProvider struct {
	svc       keyManagementClient
	ctx       context.Context
	keyName   string
	keyLength int
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
	out.EncryptionKey = make([]byte, p.keyLength)
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
