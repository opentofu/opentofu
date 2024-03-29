package gcp_kms

import (
	"context"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
)

type mockKMC struct {
	encrypt func(*kmspb.EncryptRequest) (*kmspb.EncryptResponse, error)
	decrypt func(*kmspb.DecryptRequest) (*kmspb.DecryptResponse, error)
}

func (m *mockKMC) Encrypt(ctx context.Context, req *kmspb.EncryptRequest, opts ...gax.CallOption) (*kmspb.EncryptResponse, error) {
	return m.encrypt(req)
}
func (m *mockKMC) Decrypt(ctx context.Context, req *kmspb.DecryptRequest, opts ...gax.CallOption) (*kmspb.DecryptResponse, error) {
	return m.decrypt(req)
}

func injectMock(m *mockKMC) {
	newKeyManagementClient = func(ctx context.Context, opts ...option.ClientOption) (keyManagementClient, error) {
		return m, nil
	}
}
