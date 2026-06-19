// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

type mockKMS struct {
	genkey  func(params *kms.GenerateDataKeyInput) (*kms.GenerateDataKeyOutput, error)
	decrypt func(params *kms.DecryptInput) (*kms.DecryptOutput, error)
}

func (m *mockKMS) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return m.genkey(params)
}
func (m *mockKMS) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return m.decrypt(params)
}

func injectMock(t testing.TB, m *mockKMS) {
	original := newKMSFromConfig
	t.Cleanup(func() {
		newKMSFromConfig = original
	})

	newKMSFromConfig = func(cfg aws.Config) kmsClient {
		return m
	}
}

func injectDefaultMock(t testing.TB) {
	injectCapturingMock(t, "alias/my-mock-key")
}

func injectCapturingMock(t testing.TB, keyId string) (capturedGenKeyContext *map[string]string, capturedDecryptContext *map[string]string) {
	var genCtx, decCtx map[string]string
	injectMock(t, &mockKMS{
		genkey: func(params *kms.GenerateDataKeyInput) (*kms.GenerateDataKeyOutput, error) {
			genCtx = params.EncryptionContext
			keyData := make([]byte, 32)
			_, err := rand.Read(keyData)
			if err != nil {
				panic(err)
			}
			return &kms.GenerateDataKeyOutput{
				CiphertextBlob: append([]byte(keyId), keyData...),
				Plaintext:      keyData,
			}, nil
		},
		decrypt: func(params *kms.DecryptInput) (*kms.DecryptOutput, error) {
			decCtx = params.EncryptionContext
			return &kms.DecryptOutput{
				Plaintext: params.CiphertextBlob[len(keyId):],
			}, nil
		},
	})
	return &genCtx, &decCtx
}
