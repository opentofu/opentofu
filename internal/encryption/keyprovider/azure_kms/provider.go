package azure_kms

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Result []byte `json:"result"`
	// keyAlgorithm azkeys.EncryptionAlgorithm `json:"key_algoritm"`
	// keyVersion   string                     `json:"key_version"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Result) != 0
}

type keyProvider struct {
	svc          *azkeys.Client
	ctx          context.Context
	keyName      string
	keyAlgorithm azkeys.EncryptionAlgorithm
	keySize      int
}

func (p keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, keyprovider.ErrInvalidMetadata{Message: "bug: no metadata struct provided"}
	}
	inMeta := rawMeta.(*keyMeta)

	outMeta := &keyMeta{}
	out := keyprovider.Output{}

	out.EncryptionKey = make([]byte, p.keySize)
	_, err := rand.Read(out.EncryptionKey)
	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to generate key",
			Cause:   err,
		}
	}

	// Find latest key version
	version := ""
	var versionTime *time.Time
	pager := p.svc.NewListKeyPropertiesVersionsPager(p.keyName, nil)
	for pager.More() {
		page, err := pager.NextPage(p.ctx)
		if err != nil {
			return out, outMeta, &keyprovider.ErrKeyProviderFailure{
				Message: "failed to identify latest version of key",
				Cause:   err,
			}
		}
		for _, v := range page.Value {
			if *v.Attributes.Enabled {
				if version == "" || v.Attributes.Created.After(*versionTime) {
					version = v.KID.Version()
					versionTime = v.Attributes.Created
				}
			}
		}
	}
	if version == "" {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to list enabled versions of key",
		}
	}

	// Encrypt new encryption key using kms
	wrappedKeyData, err := p.svc.WrapKey(p.ctx, p.keyName, version, azkeys.KeyOperationParameters{
		Algorithm: to.Ptr(azkeys.EncryptionAlgorithmA256GCM),
		Value:     out.EncryptionKey,
	}, nil)

	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to secure key",
			Cause:   err,
		}
	}

	outMeta.Result = wrappedKeyData.Result
	// outMeta.keyAlgorithm = p.keyAlgorithm
	// outMeta.keyVersion = version

	// We do not set the DecryptionKey here as we should only be setting the decryption key if we are decrypting
	// and that is handled below when we check if the inMeta has a CiphertextBlob

	if inMeta.isPresent() {
		// We have an existing decryption key to decrypt, so we should now populate the DecryptionKey
		unwrappedKeyData, decryptErr := p.svc.UnwrapKey(p.ctx, p.keyName, version, azkeys.KeyOperationParameters{
			Algorithm: to.Ptr(azkeys.EncryptionAlgorithmA256GCM),
			Value:     inMeta.Result,
		}, nil)

		if decryptErr != nil {
			return out, outMeta, decryptErr
		}

		// Set decryption key on the output
		out.DecryptionKey = unwrappedKeyData.Result
	}

	return out, outMeta, nil
}
