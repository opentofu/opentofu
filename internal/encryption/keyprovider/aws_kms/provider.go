package aws_kms

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	CiphertextBlob []byte `json:"ciphertext_blob"`
}

type keyProvider struct {
	Config
	svc *kms.Client
	ctx context.Context
}

func (p keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	inMeta := rawMeta.(*keyMeta)
	outMeta := keyMeta{}
	out := keyprovider.Output{}

	// Generate new key pair
	if p.Symetric {
		var spec types.DataKeySpec

		for _, opt := range spec.Values() {
			if string(opt) == p.KeySpec {
				spec = opt
			}
		}

		if len(spec) == 0 {
			return out, outMeta, fmt.Errorf("Invalid key_spec %s, expected one of %v", p.KeySpec, spec.Values())
		}

		generatedKeyData, err := p.svc.GenerateDataKey(p.ctx, &kms.GenerateDataKeyInput{
			KeyId:   aws.String(p.KMSKeyID),
			KeySpec: spec,
		})

		if err != nil {
			return out, outMeta, err
		}

		// Set inital outputs
		out.EncryptionKey = generatedKeyData.Plaintext
		out.DecryptionKey = generatedKeyData.Plaintext
		outMeta.CiphertextBlob = generatedKeyData.CiphertextBlob
	} else {
		var spec types.DataKeyPairSpec

		for _, opt := range spec.Values() {
			if string(opt) == p.KeySpec {
				spec = opt
			}
		}

		if len(spec) == 0 {
			return out, outMeta, fmt.Errorf("Invalid key_spec %s, expected one of %v", p.KeySpec, spec.Values())
		}
		generatedKeyData, err := p.svc.GenerateDataKeyPair(p.ctx, &kms.GenerateDataKeyPairInput{
			KeyId:       aws.String(p.KMSKeyID),
			KeyPairSpec: spec,
		})

		if err != nil {
			return out, outMeta, err
		}

		// Set inital outputs
		out.EncryptionKey = generatedKeyData.PublicKey
		out.DecryptionKey = generatedKeyData.PrivateKeyPlaintext
		outMeta.CiphertextBlob = generatedKeyData.PrivateKeyCiphertextBlob
	}

	if len(inMeta.CiphertextBlob) != 0 {
		// We have an existing decryption key to decode
		decodedKeyData, err := p.svc.Decrypt(p.ctx, &kms.DecryptInput{
			KeyId:          aws.String(p.KMSKeyID),
			CiphertextBlob: inMeta.CiphertextBlob,
		})

		if err != nil {
			return out, outMeta, err
		}

		// Override decryption key for the existing data
		out.DecryptionKey = decodedKeyData.Plaintext
	}

	return out, outMeta, nil
}
