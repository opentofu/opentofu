// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure_vault

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Ciphertext []byte `json:"ciphertext"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

type keyManagementClient interface {
	Decrypt(ctx context.Context, name string, version string, parameters azkeys.KeyOperationParameters, options *azkeys.DecryptOptions) (azkeys.DecryptResponse, error)
	Encrypt(ctx context.Context, name string, version string, parameters azkeys.KeyOperationParameters, options *azkeys.EncryptOptions) (azkeys.EncryptResponse, error)
	NewListKeyPropertiesVersionsPager(name string, options *azkeys.ListKeyPropertiesVersionsOptions) *runtime.Pager[azkeys.ListKeyPropertiesVersionsResponse]
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

	outMeta.Ciphertext = encryptedKeyData.Result

	// We do not set the DecryptionKey here as we should only be setting the decryption key if we are decrypting
	// and that is handled below when we check if the inMeta has a CiphertextBlob

	if inMeta.isPresent() {
		// Unlike, for example, GCP KMS, we must go through every enabled version of the KEK (Key-encryption key),
		// as an Azure KEK rotation might make it so that the key in the meta was encrypted by an
		// earlier KEK. GCP does this automatically. I wish this did, too...

		// We're going to page through all the KEK versions and try decrypting the existing key in the keyMeta
		// with each one. If that doesn't work, there's an error; either a missing or prematurely disabled key.
		versionPager := p.svc.NewListKeyPropertiesVersionsPager(p.keyName, nil)

		for versionPager.More() {
			result, err := versionPager.NextPage(p.ctx)
			if err != nil {
				return out, outMeta, &keyprovider.ErrKeyProviderFailure{
					Message: "failed to list key versions",
					Cause:   err,
				}
			}
			for _, properties := range result.Value {
				if properties == nil {
					continue
				}
				if *properties.Attributes.Enabled {
					decryptResp, err := p.svc.Decrypt(
						p.ctx,
						p.keyName,
						properties.KID.Version(),
						azkeys.KeyOperationParameters{
							Algorithm: &p.algo,
							Value:     inMeta.Ciphertext,
						},
						nil,
					)
					if err != nil {
						// This decryption failed
						// TODO should we have a debug log message here?
						continue
					}
					out.DecryptionKey = decryptResp.Result
					return out, outMeta, nil
				}
			}
		}
		// We've reached the end of looking through key versions, and none of them work.
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("after trying all enabled key versions of %s, none of them worked", p.keyName),
			Cause:   errors.New("the Azure key version which encrypted the current decryption key is not present, or is disabled, or is otherwise unavailable"),
		}
	}

	return out, outMeta, nil
}
