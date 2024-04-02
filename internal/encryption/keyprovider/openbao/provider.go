package openbao

import (
	"context"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Ciphertext []byte `json:"ciphertext"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

type keyProvider struct {
	svc       service
	keyName   string
	keyLength int
}

func (p keyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
			Message: "bug: no metadata struct provided",
		}
	}

	inMeta, ok := rawMeta.(*keyMeta)
	if !ok {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
			Message: "bug: invalid metadata struct type",
		}
	}

	ctx := context.Background()

	// KeyLength is specified in bytes, but OpenBao wants it in bits so it's KeyLength * 8.
	dataKey, err := p.svc.generateDataKey(ctx, p.keyName, p.keyLength*8)
	if err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to generate openbao data key",
			Cause:   err,
		}
	}

	outMeta := &keyMeta{
		Ciphertext: dataKey.Ciphertext,
	}

	out := keyprovider.Output{
		EncryptionKey: dataKey.Plaintext,
	}

	if inMeta.isPresent() {
		out.DecryptionKey, err = p.svc.decryptData(ctx, p.keyName, inMeta.Ciphertext)
		if err != nil {
			return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
				Message: "failed to decrypt ciphertext",
				Cause:   err,
			}
		}
	}

	return out, outMeta, nil
}
