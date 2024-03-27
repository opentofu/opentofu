package openbao

import (
	"context"

	openbao "github.com/openbao/openbao/api"
)

type mockClientFunc func(ctx context.Context, path string, data []byte) (*openbao.Secret, error)

func (f mockClientFunc) WriteBytesWithContext(ctx context.Context, path string, data []byte) (*openbao.Secret, error) {
	return f(ctx, path, data)
}

func injectMock(m mockClientFunc) {
	newClient = func(_ *openbao.Config, _ string) (client, error) {
		return m, nil
	}
}
