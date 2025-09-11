// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure_vault

import (
	"context"
	"crypto/rand"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Ciphertext                  []byte                     `json:"ciphertext"`
	IV                          []byte                     `json:"iv"`
	AuthenticationTag           []byte                     `json:"authentication_tag"`
	AdditionalAuthenticatedData []byte                     `json:"additional_authenticated_data"`
	KeyVersion                  string                     `json:"key_version"`
	DecryptAlgorithm            azkeys.EncryptionAlgorithm `json:"algo"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

type keyManagementClient interface {
	Decrypt(ctx context.Context, name string, version string, parameters azkeys.KeyOperationParameters, options *azkeys.DecryptOptions) (azkeys.DecryptResponse, error)
	Encrypt(ctx context.Context, name string, version string, parameters azkeys.KeyOperationParameters, options *azkeys.EncryptOptions) (azkeys.EncryptResponse, error)
}

type keyProvider struct {
	svc       keyManagementClient
	ctx       context.Context
	keyName   string
	keyLength int
	algo      azkeys.EncryptionAlgorithm
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

	// Encrypt new encryption key using Azure Key Vault
	encryptedKeyData, err := p.svc.Encrypt(
		p.ctx,
		p.keyName,
		"", // Version is not provided: we omit this to always use the current version
		azkeys.KeyOperationParameters{
			Algorithm: &p.algo,
			Value:     out.EncryptionKey,
		},
		nil,
	)
	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to encrypt key",
			Cause:   err,
		}
	}

	outMeta = &keyMeta{
		Ciphertext:       encryptedKeyData.Result,
		IV:               encryptedKeyData.IV,
		KeyVersion:       encryptedKeyData.KID.Version(),
		DecryptAlgorithm: p.algo,

		AuthenticationTag:           encryptedKeyData.AuthenticationTag,
		AdditionalAuthenticatedData: encryptedKeyData.AdditionalAuthenticatedData,
	}

	// We do not set the DecryptionKey here as we should only be setting the decryption key if we are decrypting
	// and that is handled below when we check if the inMeta has a CiphertextBlob

	if inMeta.isPresent() {
		// We decrypt the key using the version that previously encrypted the key,
		// which is saved in the metadata.
		algo := inMeta.DecryptAlgorithm
		if algo == "" {
			// This should never happen, but we'll add it here just in case
			algo = p.algo
		}
		decryptResp, err := p.svc.Decrypt(
			p.ctx,
			p.keyName,
			inMeta.KeyVersion,
			azkeys.KeyOperationParameters{
				Algorithm:                   &algo,
				Value:                       inMeta.Ciphertext,
				IV:                          inMeta.IV,
				AuthenticationTag:           inMeta.AuthenticationTag,
				AdditionalAuthenticatedData: inMeta.AdditionalAuthenticatedData,
			},
			nil,
		)
		if err != nil {
			// This decryption failed
			return out, outMeta, &keyprovider.ErrKeyProviderFailure{
				Message: "failed to decrypt key",
				Cause:   err,
			}
		}
		out.DecryptionKey = decryptResp.Result
	}

	return out, outMeta, nil
}
