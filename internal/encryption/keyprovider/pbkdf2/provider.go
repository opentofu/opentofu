// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package pbkdf2 contains a key provider that takes a passphrase and emits a PBKDF2 hash of the configured length.
package pbkdf2

import (
	"fmt"
	"io"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	goPBKDF2 "golang.org/x/crypto/pbkdf2"
)

type pbkdf2KeyProvider struct {
	Config
}

func (p pbkdf2KeyProvider) generateMetadata() (*Metadata, error) {
	// Build outMeta based on current configuration
	outMeta := &Metadata{
		Iterations:   p.Iterations,
		HashFunction: p.HashFunction,
		Salt:         make([]byte, p.SaltLength),
		KeyLength:    p.KeyLength,
	}
	// Generate new salt
	if _, err := io.ReadFull(p.randomSource, outMeta.Salt); err != nil {
		return nil, &keyprovider.ErrKeyProviderFailure{
			Message: fmt.Sprintf("failed to obtain %d bytes of random data", p.SaltLength),
			Cause:   err,
		}
	}
	return outMeta, nil
}

func (p pbkdf2KeyProvider) Provide(rawMeta keyprovider.KeyMeta) (keyprovider.Output, keyprovider.KeyMeta, error) {
	if rawMeta == nil {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{Message: "bug: no metadata struct provided"}
	}
	inMeta, ok := rawMeta.(*Metadata)
	if !ok {
		return keyprovider.Output{}, nil, &keyprovider.ErrInvalidMetadata{
			Message: fmt.Sprintf("bug: incorrect metadata type of %T provided", rawMeta),
		}
	}

	outMeta, err := p.generateMetadata()
	if err != nil {
		return keyprovider.Output{}, nil, err
	}

	var decryptionKey []byte
	if inMeta.isPresent() {
		if err := inMeta.validate(); err != nil {
			return keyprovider.Output{}, nil, err
		}
		var decryptionPassphrase []byte
		if p.Chain != nil {
			decryptionPassphrase = p.Chain.DecryptionKey
		} else {
			decryptionPassphrase = []byte(p.Passphrase)
		}
		decryptionKey = goPBKDF2.Key(
			decryptionPassphrase,
			inMeta.Salt,
			inMeta.Iterations,
			inMeta.KeyLength,
			inMeta.HashFunction.Function(),
		)
	}

	var encryptionPassphrase []byte
	if p.Chain != nil {
		encryptionPassphrase = p.Chain.EncryptionKey
	} else {
		encryptionPassphrase = []byte(p.Passphrase)
	}
	return keyprovider.Output{
		EncryptionKey: goPBKDF2.Key(
			encryptionPassphrase,
			outMeta.Salt,
			outMeta.Iterations,
			outMeta.KeyLength,
			outMeta.HashFunction.Function(),
		),
		DecryptionKey: decryptionKey,
	}, outMeta, nil
}
