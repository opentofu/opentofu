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

	// Set initial outputs
	out.EncryptionKey = generatedKeyData.Plaintext
	outMeta.CiphertextBlob = generatedKeyData.CiphertextBlob

	// We do not set the DecryptionKey here as we should only be setting the decryption key if we are decrypting
	// and that is handled below when we check if the inMeta has a CiphertextBlob
	//out.DecryptionKey = generatedKeyData.Plaintext

	if len(inMeta.CiphertextBlob) != 0 {
		// We have an existing decryption key to decrypt
		decryptedKeyData, err := p.svc.Decrypt(p.ctx, &kms.DecryptInput{
			KeyId:          aws.String(p.KMSKeyID),
			CiphertextBlob: inMeta.CiphertextBlob,
		})

		if err != nil {
			return out, outMeta, err
		}

		// Override decryption key for the existing data
		out.DecryptionKey = decryptedKeyData.Plaintext
	}

	return out, outMeta, nil
}
