// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	CiphertextBlob []byte `json:"ciphertext_blob"`
}

func (m keyMeta) isPresent() bool {
	return len(m.CiphertextBlob) != 0
}

type kmsClient interface {
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

type keyProvider struct {
	Config
	svc kmsClient
	ctx context.Context
}

func (p keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: no metadata struct provided"}
	}
	inMeta, ok := rawMeta.(*keyMeta)
	if !ok {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: metadata struct is not of the correct type"}
	}

	outMeta := &keyMeta{}
	out := keyprovider.Output{}

	// as validation has happened in the config, we can safely cast here and not worry about the cast failing
	spec := types.DataKeySpec(p.KeySpec)

	generatedKeyData, err := p.svc.GenerateDataKey(p.ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(p.KMSKeyID),
		KeySpec: spec,
	})

	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to generate key",
			Cause:   err,
		}
	}

	// Set initial outputs that are always set
	out.EncryptionKey = generatedKeyData.Plaintext
	outMeta.CiphertextBlob = generatedKeyData.CiphertextBlob

	// We do not set the DecryptionKey here as we should only be setting the decryption key if we are decrypting
	// and that is handled below when we check if the inMeta has a CiphertextBlob

	if inMeta.isPresent() {
		// We have an existing decryption key to decrypt, so we should now populate the DecryptionKey
		decryptedKeyData, decryptErr := p.svc.Decrypt(p.ctx, &kms.DecryptInput{
			KeyId:          aws.String(p.KMSKeyID),
			CiphertextBlob: inMeta.CiphertextBlob,
		})

		if decryptErr != nil {
			return out, outMeta, &keyprovider.ErrKeyProviderFailure{Cause: decryptErr}
		}

		// Set decryption key on the output
		out.DecryptionKey = decryptedKeyData.Plaintext
	}

	return out, outMeta, nil
}
