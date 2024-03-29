package aws_kms

import (
	"context"
	"crypto/rand"

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

func injectMock(m *mockKMS) {
	newKMSFromConfig = func(cfg aws.Config) kmsClient {
		return m
	}
}

func injectDefaultMock() {
	injectMock(&mockKMS{
		genkey: func(params *kms.GenerateDataKeyInput) (*kms.GenerateDataKeyOutput, error) {
			keyData := make([]byte, 32)
			_, err := rand.Read(keyData)
			if err != nil {
				panic(err)
			}

			return &kms.GenerateDataKeyOutput{
				CiphertextBlob: append([]byte(*params.KeyId), keyData...),
				Plaintext:      keyData,
			}, nil

		},
		decrypt: func(params *kms.DecryptInput) (*kms.DecryptOutput, error) {
			return &kms.DecryptOutput{
				Plaintext: params.CiphertextBlob[:len(*params.KeyId)],
			}, nil
		},
	})
}
