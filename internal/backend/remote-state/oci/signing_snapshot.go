// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"crypto/rsa"
	"errors"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// A snapshot provider is only for a service request signer. The SDK signer
// asks for the private key and then the key ID in separate calls. A credential
// refresh between those calls can otherwise pair an old key with a new token.
// Each SDK service operation gets its own snapshot provider. In particular,
// a canceled SDK retry goroutine cannot overwrite another operation's pair
// when it finishes signing after its caller has already returned.
// Validation and other standalone credential reads use the original provider.
type snapshotSigningProvider struct {
	common.ConfigurationProvider
	mu          sync.Mutex
	pairedKeyID string
}

func newSnapshotSigningProvider(provider common.ConfigurationProvider) common.ConfigurationProvider {
	return &snapshotSigningProvider{ConfigurationProvider: provider}
}

func signingCredentialError() error {
	return errors.New("OCI request signing could not obtain a stable credential pair; check credential renewal and retry the operation")
}

func (p *snapshotSigningProvider) PrivateRSAKey() (key *rsa.PrivateKey, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pairedKeyID = ""
	defer func() {
		if recover() != nil {
			key = nil
			err = signingCredentialError()
			p.pairedKeyID = ""
		}
	}()
	// Refreshing providers generally become stable after one renewal. Bound
	// attempts so a broken or constantly rotating credential source fails
	// promptly without signing with mismatched credentials.
	for range 3 {
		before, readErr := p.ConfigurationProvider.KeyID()
		if readErr != nil || before == "" {
			return nil, signingCredentialError()
		}
		candidate, readErr := p.ConfigurationProvider.PrivateRSAKey()
		if readErr != nil || candidate == nil {
			return nil, signingCredentialError()
		}
		after, readErr := p.ConfigurationProvider.KeyID()
		if readErr != nil || after == "" {
			return nil, signingCredentialError()
		}
		if before == after {
			p.pairedKeyID = after
			return candidate, nil
		}
	}
	return nil, signingCredentialError()
}

func (p *snapshotSigningProvider) KeyID() (keyID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() {
		if recover() != nil {
			keyID = ""
			err = signingCredentialError()
		}
	}()
	if p.pairedKeyID != "" {
		return p.pairedKeyID, nil
	}
	keyID, err = p.ConfigurationProvider.KeyID()
	if err != nil {
		return "", signingCredentialError()
	}
	return keyID, nil
}

func (p *snapshotSigningProvider) Refreshable() bool {
	provider, ok := p.ConfigurationProvider.(common.RefreshableConfigurationProvider)
	return ok && provider.Refreshable()
}
