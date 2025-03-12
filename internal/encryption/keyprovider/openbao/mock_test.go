package openbao

import (
	"context"

	openbao "github.com/openbao/openbao/api/v2"
)

type mockClientFunc func(ctx context.Context, path string, data map[string]interface{}) (*openbao.Secret, error)

func (f mockClientFunc) WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*openbao.Secret, error) {
	return f(ctx, path, data)
}

func injectMock(m mockClientFunc) {
	newClient = func(_ *openbao.Config, _ string) (client, error) {
		return m, nil
	}
}

func injectDefaultClient() {
	newClient = newOpenBaoClient
}
