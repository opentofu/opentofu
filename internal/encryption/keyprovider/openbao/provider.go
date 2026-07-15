// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package openbao

import (
	"context"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type keyMeta struct {
	Ciphertext     []byte `json:"ciphertext"`
	AssociatedData string `json:"associated_data,omitempty"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

type keyProvider struct {
	svc            service
	keyName        string
	keyLength      DataKeyLength
	associatedData string
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

	dataKey, err := p.svc.generateDataKey(ctx, p.keyName, p.keyLength.Bits(), p.associatedData)
	if err != nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to generate OpenBao data key (check if the configuration valid and OpenBao server accessible)",
			Cause:   err,
		}
	}

	outMeta := &keyMeta{
		Ciphertext:     dataKey.Ciphertext,
		AssociatedData: p.associatedData,
	}

	out := keyprovider.Output{
		EncryptionKey: dataKey.Plaintext,
	}

	if inMeta.isPresent() {
		out.DecryptionKey, err = p.svc.decryptData(ctx, p.keyName, inMeta.Ciphertext, inMeta.AssociatedData)
		if err != nil {
			return keyprovider.Output{}, nil, &keyprovider.ErrKeyProviderFailure{
				Message: "failed to decrypt ciphertext (check if the configuration valid and OpenBao server accessible)",
				Cause:   err,
			}
		}
	}

	return out, outMeta, nil
}
