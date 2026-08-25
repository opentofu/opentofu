// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package stackit_kms

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/stackitcloud/stackit-sdk-go/services/kms/v1api"
)

// keyMeta stores the version too: decrypt must target the version that encrypted.
type keyMeta struct {
	Ciphertext []byte `json:"ciphertext"`
	Version    int64  `json:"key_version"`
}

func (m keyMeta) isPresent() bool {
	return len(m.Ciphertext) != 0
}

// keyManagementClient hides v1api's request types: their fields are unexported and unmockable.
type keyManagementClient interface {
	Encrypt(ctx context.Context, version int64, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, version int64, ciphertext []byte) ([]byte, error)
	ListVersions(ctx context.Context) ([]v1api.Version, error)
}

// apiClient adapts v1api.DefaultAPI, bound to one key, to keyManagementClient.
type apiClient struct {
	svc       v1api.DefaultAPI
	projectID string
	region    string
	keyRingID string
	keyID     string
}

// Encrypt base64-encodes manually: v1api's Data field is a string, not []byte.
func (a *apiClient) Encrypt(ctx context.Context, version int64, plaintext []byte) ([]byte, error) {
	result, err := a.svc.Encrypt(ctx, a.projectID, a.region, a.keyRingID, a.keyID, version).
		EncryptPayload(v1api.EncryptPayload{
			Data: base64.StdEncoding.EncodeToString(plaintext),
		}).Execute()
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(result.GetData())
}

// Decrypt requires the version that produced this ciphertext (see keyMeta above).
func (a *apiClient) Decrypt(ctx context.Context, version int64, ciphertext []byte) ([]byte, error) {
	result, err := a.svc.Decrypt(ctx, a.projectID, a.region, a.keyRingID, a.keyID, version).
		DecryptPayload(v1api.DecryptPayload{
			Data: base64.StdEncoding.EncodeToString(ciphertext),
		}).Execute()
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(result.GetData())
}

// ListVersions returns every version: the API does not paginate this response.
func (a *apiClient) ListVersions(ctx context.Context) ([]v1api.Version, error) {
	vl, err := a.svc.ListVersions(ctx, a.projectID, a.region, a.keyRingID, a.keyID).Execute()
	if err != nil {
		return nil, err
	}
	return vl.GetVersions(), nil
}

type keyProvider struct {
	svc       keyManagementClient
	ctx       context.Context
	version   int64
	keyLength int
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

	// Generate a new data encryption key for this state snapshot.
	out.EncryptionKey = make([]byte, p.keyLength)
	if _, err := rand.Read(out.EncryptionKey); err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to generate key",
			Cause:   err,
		}
	}

	// Wrap the new key with STACKIT KMS.
	ciphertext, err := p.svc.Encrypt(p.ctx, p.version, out.EncryptionKey)
	if err != nil {
		return out, outMeta, &keyprovider.ErrKeyProviderFailure{
			Message: "failed to encrypt key",
			Cause:   err,
		}
	}
	outMeta.Ciphertext = ciphertext
	outMeta.Version = p.version

	// DecryptionKey is set only when inMeta already has a stored ciphertext.
	if inMeta.isPresent() {
		plaintext, err := p.svc.Decrypt(p.ctx, inMeta.Version, inMeta.Ciphertext)
		if err != nil {
			return out, outMeta, &keyprovider.ErrKeyProviderFailure{
				Message: "failed to decrypt key",
				Cause:   err,
			}
		}
		out.DecryptionKey = plaintext
	}

	return out, outMeta, nil
}
